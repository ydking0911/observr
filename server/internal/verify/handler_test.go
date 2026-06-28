package verify_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ydking0911/observr/server/internal/storage"
	"github.com/ydking0911/observr/server/internal/verify"
)

func TestHandlerReportsOKForValidChain(t *testing.T) {
	s := fakeLoader{events: validRows()}

	result := getVerify(t, s)
	if !result.OK {
		t.Fatalf("expected ok response, got %+v", result)
	}
	if result.Checked != 3 {
		t.Fatalf("Checked = %d, want 3", result.Checked)
	}
	if result.Skipped != 0 {
		t.Fatalf("Skipped = %d, want 0", result.Skipped)
	}
	if result.BrokenAt != nil {
		t.Fatalf("BrokenAt = %v, want nil", *result.BrokenAt)
	}
	if result.Detail != nil {
		t.Fatalf("Detail = %v, want nil", *result.Detail)
	}
}

func TestHandlerReportsFirstTamperedRow(t *testing.T) {
	rows := validRows()
	rows[1].RawJSON = `{"id":"evt_b","message":"changed"}`
	s := fakeLoader{events: rows}

	result := getVerify(t, s)
	if result.OK {
		t.Fatalf("expected broken response, got %+v", result)
	}
	// evt_a verified before the break; the broken row is not counted as checked.
	if result.Checked != 1 {
		t.Fatalf("Checked = %d, want 1", result.Checked)
	}
	if result.BrokenAt == nil || *result.BrokenAt != "evt_b" {
		t.Fatalf("BrokenAt = %v, want evt_b", result.BrokenAt)
	}
	if result.Detail == nil || *result.Detail != verify.DetailBrokenLink {
		t.Fatalf("Detail = %v, want broken_link", result.Detail)
	}
}

func TestHandlerReportsDeletedMiddleRow(t *testing.T) {
	rows := validRows()
	rows = []storage.VerifyEvent{rows[0], rows[2]}
	s := fakeLoader{events: rows}

	result := getVerify(t, s)
	if result.OK {
		t.Fatalf("expected broken response, got %+v", result)
	}
	if result.Checked != 1 {
		t.Fatalf("Checked = %d, want 1", result.Checked)
	}
	if result.BrokenAt == nil || *result.BrokenAt != "evt_c" {
		t.Fatalf("BrokenAt = %v, want evt_c", result.BrokenAt)
	}
	if result.Detail == nil || *result.Detail != verify.DetailBrokenLink {
		t.Fatalf("Detail = %v, want broken_link", result.Detail)
	}
}

// C1: a DB upgraded in place has legacy rows (empty raw_json/hash) preceding
// the first hashed event. Those rows are unverifiable, not tampered, so the
// chain must report OK with a Skipped count — never a false "broken".
func TestHandlerSkipsLeadingLegacyRowsAndVerifiesSuffix(t *testing.T) {
	rows := append([]storage.VerifyEvent{
		{ID: "evt_legacy1", RawJSON: "", Hash: ""},
		{ID: "evt_legacy2", RawJSON: "", Hash: ""},
	}, validRows()...)
	s := fakeLoader{events: rows}

	result := getVerify(t, s)
	if !result.OK {
		t.Fatalf("expected ok response for legacy prefix, got %+v", result)
	}
	if result.Skipped != 2 {
		t.Fatalf("Skipped = %d, want 2", result.Skipped)
	}
	if result.Checked != 3 {
		t.Fatalf("Checked = %d, want 3", result.Checked)
	}
	if result.BrokenAt != nil {
		t.Fatalf("BrokenAt = %v, want nil", *result.BrokenAt)
	}
	if result.Detail != nil {
		t.Fatalf("Detail = %v, want nil", *result.Detail)
	}
}

// A blank row appearing *after* hashed rows means a previously-hashed event was
// deleted or blanked — that is real tampering and must be reported.
func TestHandlerReportsBlankRowAfterHashedRows(t *testing.T) {
	rows := validRows()
	rows[1].RawJSON = ""
	rows[1].Hash = ""
	s := fakeLoader{events: rows}

	result := getVerify(t, s)
	if result.OK {
		t.Fatalf("expected broken response, got %+v", result)
	}
	if result.BrokenAt == nil || *result.BrokenAt != "evt_b" {
		t.Fatalf("BrokenAt = %v, want evt_b", result.BrokenAt)
	}
	if result.Detail == nil || *result.Detail != verify.DetailBrokenLink {
		t.Fatalf("Detail = %v, want broken_link", result.Detail)
	}
}

func TestHandlerReportsOKForEmptyDB(t *testing.T) {
	s := fakeLoader{events: nil}

	result := getVerify(t, s)
	if !result.OK {
		t.Fatalf("expected ok response for empty db, got %+v", result)
	}
	if result.Checked != 0 || result.Skipped != 0 {
		t.Fatalf("Checked=%d Skipped=%d, want 0/0", result.Checked, result.Skipped)
	}
}

func TestHandlerReportsOKForLegacyOnlyDB(t *testing.T) {
	s := fakeLoader{events: []storage.VerifyEvent{
		{ID: "evt_legacy1"},
		{ID: "evt_legacy2"},
	}}

	result := getVerify(t, s)
	if !result.OK {
		t.Fatalf("expected ok for legacy-only db, got %+v", result)
	}
	if result.Checked != 0 || result.Skipped != 2 {
		t.Fatalf("Checked=%d Skipped=%d, want 0/2", result.Checked, result.Skipped)
	}
}

func TestHandlerReportsTailTruncatedFromPersistedMeta(t *testing.T) {
	rows := validRows()
	s := fakeLoader{
		events: rows[:2],
		meta: &storage.ChainMeta{
			HeadHash: rows[2].Hash,
			Count:    3,
			LastID:   "evt_c",
		},
	}

	result := getVerify(t, s)
	if result.OK {
		t.Fatalf("expected tail-truncated response, got %+v", result)
	}
	if result.Checked != 2 {
		t.Fatalf("Checked = %d, want 2", result.Checked)
	}
	if result.BrokenAt != nil {
		t.Fatalf("BrokenAt = %v, want nil", *result.BrokenAt)
	}
	if result.Detail == nil || *result.Detail != verify.DetailTailTruncated {
		t.Fatalf("Detail = %v, want tail_truncated", result.Detail)
	}
}

func TestHandlerReportsTailTruncatedAfterDirectLatestRowDelete(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "observr-*.db")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := storage.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if err := s.Insert([]storage.Event{
		{ID: "evt_a", Service: "svc", Timestamp: time.Unix(10, 0).UTC(), Type: "log", Level: "info", Message: "first"},
		{ID: "evt_b", Service: "svc", Timestamp: time.Unix(20, 0).UTC(), Type: "log", Level: "info", Message: "second"},
		{ID: "evt_c", Service: "svc", Timestamp: time.Unix(30, 0).UTC(), Type: "log", Level: "info", Message: "third"},
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	db, err := sql.Open("sqlite3", f.Name()+"?_journal=WAL&_timeout=5000")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`DELETE FROM events WHERE id = ?`, "evt_c"); err != nil {
		t.Fatalf("delete latest row: %v", err)
	}

	result := getVerify(t, s)
	if result.OK {
		t.Fatalf("expected tail-truncated response, got %+v", result)
	}
	if result.Checked != 2 {
		t.Fatalf("Checked = %d, want 2", result.Checked)
	}
	if result.BrokenAt != nil {
		t.Fatalf("BrokenAt = %v, want nil", *result.BrokenAt)
	}
	if result.Detail == nil || *result.Detail != verify.DetailTailTruncated {
		t.Fatalf("Detail = %v, want tail_truncated", result.Detail)
	}
}

func TestHandlerVerifiesRetainedSuffixFromBaseAnchor(t *testing.T) {
	rows := validRows()
	s := fakeLoader{
		events: rows[1:],
		meta: &storage.ChainMeta{
			HeadHash:     rows[2].Hash,
			Count:        2,
			LastID:       "evt_c",
			BaseRowID:    rows[0].RowID,
			BasePrevHash: rows[0].Hash,
		},
	}

	result := getVerify(t, s)
	if !result.OK {
		t.Fatalf("expected retained suffix to verify, got %+v", result)
	}
	if result.Checked != 2 {
		t.Fatalf("Checked = %d, want 2", result.Checked)
	}
	if result.Detail != nil {
		t.Fatalf("Detail = %v, want nil", *result.Detail)
	}
}

func TestHandlerReturns500OnLoaderError(t *testing.T) {
	s := fakeLoader{err: errors.New("disk gone")}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/verify", nil)
	verify.NewHandler(s).ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", rec.Code, rec.Body.String())
	}
}

type testLoader interface {
	ForEachVerifyEvent(fn func(storage.VerifyEvent) error) error
	ChainMeta() (*storage.ChainMeta, error)
}

func getVerify(t *testing.T, s testLoader) verify.Result {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/verify", nil)
	verify.NewHandler(s).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result verify.Result
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}

type fakeLoader struct {
	events []storage.VerifyEvent
	meta   *storage.ChainMeta
	err    error
}

func (f fakeLoader) ForEachVerifyEvent(fn func(storage.VerifyEvent) error) error {
	if f.err != nil {
		return f.err
	}
	for _, e := range f.events {
		if err := fn(e); err != nil {
			return err
		}
	}
	return nil
}

func (f fakeLoader) ChainMeta() (*storage.ChainMeta, error) {
	return f.meta, nil
}

func validRows() []storage.VerifyEvent {
	rawA := `{"id":"evt_a","message":"first"}`
	hashA := storage.HashForVerify(rawA, "")
	rawB := `{"id":"evt_b","message":"second"}`
	hashB := storage.HashForVerify(rawB, hashA)
	rawC := `{"id":"evt_c","message":"third"}`
	hashC := storage.HashForVerify(rawC, hashB)
	return []storage.VerifyEvent{
		{RowID: 1, ID: "evt_a", RawJSON: rawA, Hash: hashA},
		{RowID: 2, ID: "evt_b", RawJSON: rawB, Hash: hashB},
		{RowID: 3, ID: "evt_c", RawJSON: rawC, Hash: hashC},
	}
}
