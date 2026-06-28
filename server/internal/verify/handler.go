// Package verify handles GET /verify for audit hash-chain integrity checks.
package verify

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/ydking0911/observr/server/internal/storage"
)

type loader interface {
	ForEachVerifyEvent(fn func(storage.VerifyEvent) error) error
}

type chainMetaLoader interface {
	ChainMeta() (*storage.ChainMeta, error)
}

// Detail identifies the class of integrity failure, when one is known.
type Detail string

const (
	DetailBrokenLink    Detail = "broken_link"
	DetailTailTruncated Detail = "tail_truncated"
)

// Result is the JSON response returned by GET /verify.
//
// Checked counts hashed events that were successfully verified. Skipped counts
// leading legacy rows (written before the hash chain existed) that carry no hash
// and are therefore unverifiable rather than broken. BrokenAt, when non-nil, is
// the first event whose stored hash no longer matches the recomputed chain.
type Result struct {
	OK       bool    `json:"ok"`
	Checked  int     `json:"checked"`
	Skipped  int     `json:"skipped"`
	BrokenAt *string `json:"broken_at"`
	Detail   *Detail `json:"detail"`
}

// NewHandler returns an HTTP handler that verifies the stored audit hash chain.
func NewHandler(s loader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := Check(s)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

// errBroken stops streaming early once a tampered row is found.
var errBroken = errors.New("chain broken")

// Check streams the stored events in insertion order and recomputes the hash
// chain. A run of legacy rows (empty raw_json/hash) before the first hashed
// event is treated as unverifiable and skipped — those rows predate the chain,
// so reporting them as broken would raise a false alarm on every upgraded DB.
// Once the hashed segment begins, an empty row means a previously-hashed event
// was deleted or blanked, which is reported as tampering.
func Check(s loader) (Result, error) {
	var (
		prevHash string
		checked  int
		skipped  int
		started  bool
		broken   *string
		meta     *storage.ChainMeta
	)

	if metaLoader, ok := s.(chainMetaLoader); ok {
		var err error
		meta, err = metaLoader.ChainMeta()
		if err != nil {
			return Result{}, fmt.Errorf("load chain meta: %w", err)
		}
		if meta != nil {
			prevHash = meta.BasePrevHash
			started = meta.BasePrevHash != ""
		}
	}

	err := s.ForEachVerifyEvent(func(event storage.VerifyEvent) error {
		if meta != nil && event.RowID <= meta.BaseRowID {
			return nil
		}
		isLegacy := event.RawJSON == "" || event.Hash == ""

		if !started {
			if isLegacy {
				skipped++
				return nil
			}
			started = true
		} else if isLegacy {
			id := event.ID
			broken = &id
			return errBroken
		}

		if event.Hash != storage.HashForVerify(event.RawJSON, prevHash) {
			id := event.ID
			broken = &id
			return errBroken
		}
		prevHash = event.Hash
		checked++
		return nil
	})
	if err != nil && !errors.Is(err, errBroken) {
		return Result{}, fmt.Errorf("verify events: %w", err)
	}

	if broken != nil {
		detail := DetailBrokenLink
		return Result{OK: false, Checked: checked, Skipped: skipped, BrokenAt: broken, Detail: &detail}, nil
	}
	if meta != nil {
		if checked < meta.Count {
			detail := DetailTailTruncated
			return Result{OK: false, Checked: checked, Skipped: skipped, BrokenAt: nil, Detail: &detail}, nil
		}
		if checked != meta.Count || (checked > 0 && prevHash != meta.HeadHash) {
			detail := DetailBrokenLink
			return Result{OK: false, Checked: checked, Skipped: skipped, BrokenAt: nil, Detail: &detail}, nil
		}
	}
	return Result{OK: true, Checked: checked, Skipped: skipped, BrokenAt: nil, Detail: nil}, nil
}
