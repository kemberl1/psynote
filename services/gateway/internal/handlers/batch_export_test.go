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

func TestBatchExportHandler_DOCX(t *testing.T) {
	repo := &fakeRepo{
		detail: &store.HistoryDetail{
			RequestID:    "r1",
			DocumentType: "daily",
			TitleSafe:    "Ежедневный дневник",
			Content:      "Жалобы: нет\nНазначения: продолжить.",
			Status:       "done",
			CreatedAt:    time.Date(2025, 9, 19, 8, 0, 0, 0, time.UTC),
		},
	}
	h := newBatchExportHandler(repo, export.New())

	body := `{"format":"docx","request_ids":["r1","r1"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/export/batch", strings.NewReader(body))
	req = withDoctor(req, "doc-1")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	cd := rec.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "diaries_batch_") {
		t.Errorf("Content-Disposition=%q", cd)
	}
	if _, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len())); err != nil {
		t.Errorf("batch docx is not a valid zip: %v", err)
	}
}

func TestBatchExportHandler_EmptyIDs(t *testing.T) {
	h := newBatchExportHandler(&fakeRepo{}, export.New())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/export/batch",
		strings.NewReader(`{"format":"docx","request_ids":[]}`))
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}
