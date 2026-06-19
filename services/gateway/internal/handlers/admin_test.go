package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aimed/gateway/internal/ragclient"
	"github.com/aimed/gateway/internal/store"
)

// ─── admin fakes ─────────────────────────────────────────────────────────────

// fakeAdminRepo is a minimal in-memory fake for store.AdminRepository.
type fakeAdminRepo struct {
	saveErr   error
	updateErr error
	getErr    error
	listErr   error

	savedID string
	docs    []store.AdminDocument
	detail  *store.AdminDocument
	total   int
}

func (f *fakeAdminRepo) SaveAdminDocument(_ context.Context, rec store.AdminDocument) (string, error) {
	if f.saveErr != nil {
		return "", f.saveErr
	}
	id := f.savedID
	if id == "" {
		id = "doc-admin-1"
	}
	return id, nil
}

func (f *fakeAdminRepo) UpdateAdminDocumentResult(_ context.Context, _ string, _ string, _ int, _ map[string]int, _ int, _ []string, _ string) error {
	return f.updateErr
}

func (f *fakeAdminRepo) ListAdminDocuments(_ context.Context, _, _ int) ([]store.AdminDocument, int, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	return f.docs, f.total, nil
}

func (f *fakeAdminRepo) GetAdminDocument(_ context.Context, _ string) (*store.AdminDocument, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.detail, nil
}

// fakeAdminRAG is a minimal in-memory fake for ragclient.AdminIngestClient.
type fakeAdminRAG struct {
	res         *ragclient.IngestResult
	err         error
	calls       int
	gotFilename string
}

func (f *fakeAdminRAG) IngestFile(_ context.Context, filename string, _ []byte, _ string) (*ragclient.IngestResult, error) {
	f.calls++
	f.gotFilename = filename
	if f.err != nil {
		return nil, f.err
	}
	return f.res, nil
}

// withAdmin returns a copy of req with role="admin" injected into context.
func withAdmin(req *http.Request, doctorID string) *http.Request {
	ctx := context.WithValue(req.Context(), ctxKeyDoctorID, doctorID)
	ctx = context.WithValue(ctx, ctxKeyRole, "admin")
	return req.WithContext(ctx)
}

// buildMultipartBody creates a multipart/form-data body with a single "file" field.
func buildMultipartBody(filename string, content []byte) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", filename)
	_, _ = part.Write(content)
	_ = writer.Close()
	return &buf, writer.FormDataContentType()
}

// ─── upload handler tests ────────────────────────────────────────────────────

func TestAdminUpload_Success(t *testing.T) {
	repo := &fakeAdminRepo{savedID: "admin-doc-1"}
	rag := &fakeAdminRAG{res: &ragclient.IngestResult{
		Status:                 "ingested",
		ChunksCount:            5,
		QdrantIDs:              []string{"q1", "q2", "q3", "q4", "q5"},
		AnonymizerRemovedCount: 12,
		RemovedByType:          map[string]int{"ФИО": 8, "адрес": 4},
	}}
	d := adminDeps{repo: repo, rag: rag}
	h := newAdminUploadHandler(d)

	body, ct := buildMultipartBody("diary.docx", []byte("PK\x03\x04fake-docx-content"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/documents", body)
	req.Header.Set("Content-Type", ct)
	req = withAdmin(req, "admin-user-1")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rag.calls != 1 {
		t.Errorf("RAG IngestFile called %d times, want 1", rag.calls)
	}
	if rag.gotFilename != "diary.docx" {
		t.Errorf("RAG got filename=%q, want diary.docx", rag.gotFilename)
	}

	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if env.Data["status"] != "ingested" {
		t.Errorf("status=%v, want ingested", env.Data["status"])
	}
	if env.Data["doc_id"] != "admin-doc-1" {
		t.Errorf("doc_id=%v, want admin-doc-1", env.Data["doc_id"])
	}
	if cc, _ := env.Data["chunks_count"].(float64); cc != 5 {
		t.Errorf("chunks_count=%v, want 5", env.Data["chunks_count"])
	}
}

func TestAdminUpload_AllFormats(t *testing.T) {
	// Each of .docx, .odt, .doc should be accepted.
	for _, ext := range []string{".docx", ".odt", ".doc"} {
		t.Run(ext, func(t *testing.T) {
			repo := &fakeAdminRepo{}
			rag := &fakeAdminRAG{res: &ragclient.IngestResult{Status: "ingested"}}
			d := adminDeps{repo: repo, rag: rag}
			h := newAdminUploadHandler(d)

			body, ct := buildMultipartBody("file"+ext, []byte("content"))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/documents", body)
			req.Header.Set("Content-Type", ct)
			req = withAdmin(req, "admin-1")
			rec := httptest.NewRecorder()
			h(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("ext=%s status=%d body=%s", ext, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAdminUpload_UnsupportedFormat(t *testing.T) {
	d := adminDeps{repo: &fakeAdminRepo{}, rag: &fakeAdminRAG{}}
	h := newAdminUploadHandler(d)

	body, ct := buildMultipartBody("data.xlsx", []byte("PK\x03\x04content"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/documents", body)
	req.Header.Set("Content-Type", ct)
	req = withAdmin(req, "admin-1")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 for unsupported format", rec.Code)
	}
}

func TestAdminUpload_MissingFile(t *testing.T) {
	d := adminDeps{repo: &fakeAdminRepo{}, rag: &fakeAdminRAG{}}
	h := newAdminUploadHandler(d)

	// Send empty multipart (no "file" field).
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/documents", nil)
	req = withAdmin(req, "admin-1")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 for missing file", rec.Code)
	}
}

func TestAdminUpload_EmptyFile(t *testing.T) {
	d := adminDeps{repo: &fakeAdminRepo{}, rag: &fakeAdminRAG{}}
	h := newAdminUploadHandler(d)

	body, ct := buildMultipartBody("empty.docx", []byte{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/documents", body)
	req.Header.Set("Content-Type", ct)
	req = withAdmin(req, "admin-1")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 for empty file", rec.Code)
	}
}

func TestAdminUpload_NoAuth(t *testing.T) {
	d := adminDeps{repo: &fakeAdminRepo{}, rag: &fakeAdminRAG{}}
	h := newAdminUploadHandler(d)

	body, ct := buildMultipartBody("diary.docx", []byte("content"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/documents", body)
	req.Header.Set("Content-Type", ct)
	// No withAdmin/withDoctor — no auth.
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rec.Code)
	}
}

func TestAdminUpload_RAGError(t *testing.T) {
	repo := &fakeAdminRepo{}
	rag := &fakeAdminRAG{err: fmt.Errorf("rag unreachable")}
	d := adminDeps{repo: repo, rag: rag}
	h := newAdminUploadHandler(d)

	body, ct := buildMultipartBody("diary.docx", []byte("PK\x03\x04content"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/documents", body)
	req.Header.Set("Content-Type", ct)
	req = withAdmin(req, "admin-1")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status=%d, want 502 for RAG error", rec.Code)
	}
}

func TestAdminUpload_RAGPIIBlocked(t *testing.T) {
	repo := &fakeAdminRepo{}
	rag := &fakeAdminRAG{res: &ragclient.IngestResult{
		Status:       "pii_blocked",
		ErrorMessage: "обнаружены неанонимизированные ПДн",
	}}
	d := adminDeps{repo: repo, rag: rag}
	h := newAdminUploadHandler(d)

	body, ct := buildMultipartBody("diary.docx", []byte("PK\x03\x04content"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/documents", body)
	req.Header.Set("Content-Type", ct)
	req = withAdmin(req, "admin-1")
	rec := httptest.NewRecorder()
	h(rec, req)

	// Handler still returns 200 (the ingest itself succeeded, but status=status from RAG).
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data["status"] != "pii_blocked" {
		t.Errorf("status=%v, want pii_blocked", env.Data["status"])
	}
}

// ─── requireAdmin middleware tests ───────────────────────────────────────────

func TestRequireAdmin_AdminAllowed(t *testing.T) {
	inner := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
	h := requireAdmin(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = withAdmin(req, "admin-1")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200 for admin", rec.Code)
	}
}

func TestRequireAdmin_DoctorForbidden(t *testing.T) {
	inner := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
	h := requireAdmin(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = withDoctor(req, "doc-1") // role="doctor" not "admin"
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403 for non-admin", rec.Code)
	}
}

func TestRequireAdmin_NoRoleForbidden(t *testing.T) {
	inner := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
	h := requireAdmin(inner)

	// Authenticated but no role in context (edge case).
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), ctxKeyDoctorID, "doc-1")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403 for missing role", rec.Code)
	}
}

// ─── list handler tests ──────────────────────────────────────────────────────

func TestAdminDocumentList_Success(t *testing.T) {
	now := time.Now()
	docs := []store.AdminDocument{
		{ID: "d1", OriginalFilename: "one.docx", Status: "ingested", ChunksCount: 3, CreatedAt: now},
		{ID: "d2", OriginalFilename: "two.odt", Status: "failed", ErrorMessage: "ошибка", CreatedAt: now},
	}
	repo := &fakeAdminRepo{docs: docs, total: 2}
	d := adminDeps{repo: repo, rag: &fakeAdminRAG{}}
	h := newAdminDocumentListHandler(d)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/documents", nil)
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if env.Meta.Total != 2 {
		t.Errorf("total=%d, want 2", env.Meta.Total)
	}
	if len(env.Data) != 2 {
		t.Fatalf("len(data)=%d, want 2", len(env.Data))
	}
	if env.Data[0]["original_filename"] != "one.docx" {
		t.Errorf("data[0].original_filename=%v", env.Data[0]["original_filename"])
	}
}

func TestAdminDocumentList_StoreError(t *testing.T) {
	repo := &fakeAdminRepo{listErr: fmt.Errorf("db down")}
	d := adminDeps{repo: repo, rag: &fakeAdminRAG{}}
	h := newAdminDocumentListHandler(d)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/documents", nil)
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", rec.Code)
	}
}

// ─── detail handler tests ────────────────────────────────────────────────────

func TestAdminDocumentDetail_Success(t *testing.T) {
	doc := &store.AdminDocument{
		ID: "d1", OriginalFilename: "diary.docx", Status: "ingested",
		AnonymizerRemovedCount: 7, ChunksCount: 4,
		QdrantIDs: []string{"q1", "q2", "q3", "q4"},
	}
	repo := &fakeAdminRepo{detail: doc}
	d := adminDeps{repo: repo, rag: &fakeAdminRAG{}}
	h := newAdminDocumentDetailHandler(d)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/documents/d1", nil)
	req.SetPathValue("id", "d1")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if env.Data["original_filename"] != "diary.docx" {
		t.Errorf("filename=%v", env.Data["original_filename"])
	}
}

func TestAdminDocumentDetail_NotFound(t *testing.T) {
	repo := &fakeAdminRepo{getErr: store.ErrNotFound}
	d := adminDeps{repo: repo, rag: &fakeAdminRAG{}}
	h := newAdminDocumentDetailHandler(d)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/documents/missing", nil)
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rec.Code)
	}
}

func TestAdminDocumentDetail_MissingID(t *testing.T) {
	d := adminDeps{repo: &fakeAdminRepo{}, rag: &fakeAdminRAG{}}
	h := newAdminDocumentDetailHandler(d)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/documents/", nil)
	// No SetPathValue — id will be empty.
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 for missing id", rec.Code)
	}
}

// ─── storeSafeFilename unit tests ────────────────────────────────────────────

func TestStoreSafeFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"дневник 2024.docx", "дневник 2024.docx"},
		{"/tmp/path/file.odt", "file.odt"},
		{".", "unknown.docx"},
		{"/", "unknown.docx"},
	}
	for _, tt := range tests {
		if got := storeSafeFilename(tt.input); got != tt.want {
			t.Errorf("storeSafeFilename(%q)=%q, want %q", tt.input, got, tt.want)
		}
	}
}
