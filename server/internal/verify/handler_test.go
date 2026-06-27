package verify_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestHandlerReturns500OnLoaderError(t *testing.T) {
	s := fakeLoader{err: errors.New("disk gone")}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/verify", nil)
	verify.NewHandler(s).ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", rec.Code, rec.Body.String())
	}
}

func getVerify(t *testing.T, s fakeLoader) verify.Result {
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

func validRows() []storage.VerifyEvent {
	rawA := `{"id":"evt_a","message":"first"}`
	hashA := storage.HashForVerify(rawA, "")
	rawB := `{"id":"evt_b","message":"second"}`
	hashB := storage.HashForVerify(rawB, hashA)
	rawC := `{"id":"evt_c","message":"third"}`
	hashC := storage.HashForVerify(rawC, hashB)
	return []storage.VerifyEvent{
		{ID: "evt_a", RawJSON: rawA, Hash: hashA},
		{ID: "evt_b", RawJSON: rawB, Hash: hashB},
		{ID: "evt_c", RawJSON: rawC, Hash: hashC},
	}
}
