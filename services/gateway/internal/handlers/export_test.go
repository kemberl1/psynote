package handlers

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aimed/gateway/internal/export"
	"github.com/aimed/gateway/internal/store"
)

func exportDetail() *store.HistoryDetail {
	return &store.HistoryDetail{
		RequestID:    "r1",
		DocumentType: "daily",
		TitleSafe:    "Ежедневный дневник · сниженное настроение",
		Content:      "Настроение сниженное.\n\nНазначения на [ДАТА].",
		Status:       "done",
		CreatedAt:    time.Date(2025, 9, 19, 8, 0, 0, 0, time.UTC),
	}
}

func TestExportHandler_DOCX(t *testing.T) {
	repo := &fakeRepo{detail: exportDetail()}
	h := newExportHandler(repo, export.New())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests/r1/export",
		strings.NewReader(`{"format":"docx"}`))
	req.SetPathValue("id", "r1")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Errorf("Content-Type=%q", ct)
	}
	cd := rec.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, "diary_daily_2025-09-19.docx") {
		t.Errorf("Content-Disposition=%q", cd)
	}
	// Body must be a valid zip (docx).
	body := rec.Body.Bytes()
	if _, err := zip.NewReader(bytes.NewReader(body), int64(len(body))); err != nil {
		t.Errorf("docx body is not a valid zip: %v", err)
	}
}

func TestExportHandler_PDF_Substitutions(t *testing.T) {
	repo := &fakeRepo{detail: exportDetail()}
	h := newExportHandler(repo, export.New())

	// substitutions are applied in memory; the resulting file must be a valid PDF.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests/r1/export",
		strings.NewReader(`{"format":"pdf","substitutions":{"[ДАТА]":"19.09.2025"}}`))
	req.SetPathValue("id", "r1")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type=%q", ct)
	}
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF")) {
		t.Error("pdf body does not start with %PDF")
	}
}

func TestExportHandler_NotFound(t *testing.T) {
	repo := &fakeRepo{getErr: store.ErrNotFound}
	h := newExportHandler(repo, export.New())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests/missing/export",
		strings.NewReader(`{"format":"docx"}`))
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rec.Code)
	}
}

func TestExportHandler_BadFormat(t *testing.T) {
	repo := &fakeRepo{detail: exportDetail()}
	h := newExportHandler(repo, export.New())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests/r1/export",
		strings.NewReader(`{"format":"xlsx"}`))
	req.SetPathValue("id", "r1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestApplySubstitutions(t *testing.T) {
	out := applySubstitutions("Дата: [ДАТА], Врач: [ФИО_ВРАЧА]",
		map[string]string{"[ДАТА]": "19.09.2025", "[ФИО_ВРАЧА]": "Врач Т."})
	if !strings.Contains(out, "19.09.2025") || !strings.Contains(out, "Врач Т.") {
		t.Errorf("substitutions not applied: %q", out)
	}
	// nil map is a no-op.
	if got := applySubstitutions("текст", nil); got != "текст" {
		t.Errorf("nil subs mutated text: %q", got)
	}
}
