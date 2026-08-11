package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aimed/gateway/internal/anonymizer"
	"github.com/aimed/gateway/internal/config"
	"github.com/aimed/gateway/internal/ragclient"
	"github.com/aimed/gateway/internal/store"
)

// ─── test helpers ─────────────────────────────────────────────────────────────

// withDoctor returns a copy of req carrying an authenticated doctor_id in
// context (as requireAuth would inject). Scoped handlers read it via
// doctorIDFromContext (Этап 9 изоляция).
func withDoctor(req *http.Request, doctorID string) *http.Request {
	ctx := context.WithValue(req.Context(), ctxKeyDoctorID, doctorID)
	ctx = context.WithValue(ctx, ctxKeyRole, "doctor")
	return req.WithContext(ctx)
}

// ─── fakes ───────────────────────────────────────────────────────────────────

// cleanAnon anonymizes by echoing input as clean (no residual PII). It records
// whether it was called so we can assert the gateway gate runs BEFORE RAG.
// Each non-empty string yields one removed [ФИО] (person) for summary tests.
type cleanAnon struct{ calls int }

func (a *cleanAnon) Anonymize(_ context.Context, raw string) (anonymizer.Result, error) {
	a.calls++
	return anonymizer.Result{
		Text:          raw + " [OK]",
		RemovedCount:  1,
		RemovedByType: map[anonymizer.EntityType]int{anonymizer.EntityPerson: 1},
		Clean:         true,
	}, nil
}

// noPIIAnon echoes input unchanged with zero removals (free text without PII).
type noPIIAnon struct{}

func (noPIIAnon) Anonymize(_ context.Context, raw string) (anonymizer.Result, error) {
	return anonymizer.Result{Text: raw, RemovedCount: 0, Clean: true}, nil
}

// blockingAnon always reports residual PII (gate fails → fail-closed).
type blockingAnon struct{}

func (blockingAnon) Anonymize(_ context.Context, _ string) (anonymizer.Result, error) {
	return anonymizer.Result{Text: "", Clean: false,
		Suspicions: []anonymizer.Suspicion{{Type: anonymizer.EntityPerson}}}, nil
}

type fakeRAG struct {
	res        *ragclient.GenerateResult
	err        error
	gotAnswers map[string]any
	calls      int
}

func (f *fakeRAG) Generate(_ context.Context, req ragclient.GenerateRequest) (*ragclient.GenerateResult, error) {
	f.calls++
	f.gotAnswers = req.Answers
	if f.err != nil {
		return nil, f.err
	}
	return f.res, nil
}
func (f *fakeRAG) Health(context.Context) error { return nil }

type fakeRepo struct {
	saved   store.GenerationRecord
	saveErr error
	id      string
	calls   int

	list    []store.HistoryItem
	total   int
	detail  *store.HistoryDetail
	getErr  error
	listErr error

	// Doctor scoping capture (Этап 9): what doctor_id reached the store layer.
	gotListDoctorID *string
	gotGetDoctorID  *string

	// Auth fixtures (Этап 9). Maps let auth-handler tests drive register/login/
	// refresh/logout against an in-memory doctor/session store.
	doctorsByID     map[string]*store.Doctor
	doctorsByEmail  map[string]*store.Doctor
	sessionsByHash  map[string]*store.Session
	createDoctorErr error
	nextDoctorID    string
	nextSessionID   string
}

func (r *fakeRepo) SaveGeneration(_ context.Context, rec store.GenerationRecord) (string, error) {
	r.calls++
	r.saved = rec
	if r.saveErr != nil {
		return "", r.saveErr
	}
	return r.id, nil
}
func (r *fakeRepo) ListGenerations(_ context.Context, f store.ListFilter) ([]store.HistoryItem, int, error) {
	r.gotListDoctorID = f.DoctorID
	if r.listErr != nil {
		return nil, 0, r.listErr
	}
	return r.list, r.total, nil
}
func (r *fakeRepo) GetGeneration(_ context.Context, _ string, doctorID *string) (*store.HistoryDetail, error) {
	r.gotGetDoctorID = doctorID
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.detail, nil
}

// ─── auth Repository methods (in-memory) ──────────────────────────────────────

func (r *fakeRepo) CreateDoctor(_ context.Context, email, passwordHash, displayName, role string) (string, error) {
	if r.createDoctorErr != nil {
		return "", r.createDoctorErr
	}
	if r.doctorsByEmail == nil {
		r.doctorsByEmail = map[string]*store.Doctor{}
		r.doctorsByID = map[string]*store.Doctor{}
	}
	if _, ok := r.doctorsByEmail[email]; ok {
		return "", store.ErrEmailTaken
	}
	id := r.nextDoctorID
	if id == "" {
		id = "doc-" + email
	}
	d := &store.Doctor{
		ID: id, Email: email, PasswordHash: passwordHash,
		DisplayName: displayName, Role: role, IsActive: true,
	}
	r.doctorsByEmail[email] = d
	r.doctorsByID[id] = d
	return id, nil
}
func (r *fakeRepo) GetDoctorByEmail(_ context.Context, email string) (*store.Doctor, error) {
	if d, ok := r.doctorsByEmail[email]; ok {
		return d, nil
	}
	return nil, store.ErrNotFound
}
func (r *fakeRepo) GetDoctorByID(_ context.Context, id string) (*store.Doctor, error) {
	if d, ok := r.doctorsByID[id]; ok {
		return d, nil
	}
	return nil, store.ErrNotFound
}
func (r *fakeRepo) TouchLastLogin(_ context.Context, _ string) error { return nil }

func (r *fakeRepo) CreateSession(_ context.Context, doctorID, refreshTokenHash string, expiresAt time.Time) (string, error) {
	if r.sessionsByHash == nil {
		r.sessionsByHash = map[string]*store.Session{}
	}
	id := r.nextSessionID
	if id == "" {
		id = "sess-" + refreshTokenHash[:8]
	}
	r.sessionsByHash[refreshTokenHash] = &store.Session{
		ID: id, DoctorID: doctorID, RefreshTokenHash: refreshTokenHash,
		ExpiresAt: expiresAt, Revoked: false,
	}
	return id, nil
}
func (r *fakeRepo) GetSessionByHash(_ context.Context, refreshTokenHash string) (*store.Session, error) {
	if s, ok := r.sessionsByHash[refreshTokenHash]; ok {
		return s, nil
	}
	return nil, store.ErrNotFound
}
func (r *fakeRepo) RevokeSession(_ context.Context, id string) error {
	for _, s := range r.sessionsByHash {
		if s.ID == id {
			s.Revoked = true
		}
	}
	return nil
}

// ─── generate ─────────────────────────────────────────────────────────────────

func TestGenerateHandler_Success(t *testing.T) {
	anon := &cleanAnon{}
	rag := &fakeRAG{res: &ragclient.GenerateResult{
		RequestID: "rag-rid", LLMModelUsed: "deepseek-v4-flash", TokensUsed: 100,
		Content: "дневник [ДАТА]", Status: "done", TitleSafe: "Ежедневный дневник",
		AnswersAnonymized: map[string]any{"mood": "lowered"}, AnonymizerRemovedCount: 3,
	}}
	repo := &fakeRepo{id: "db-rid"}

	h := newGenerateHandler(config.Config{}, anon, rag, repo)
	body := `{"document_type":"daily","answers":{"complaints_detail":"жалобы","mood":"lowered"}}`
	req := withDoctor(httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(body)), "doc-42")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Gate must have run before RAG.
	if anon.calls == 0 {
		t.Error("gateway anonymizer was not invoked before RAG")
	}
	if rag.calls != 1 {
		t.Errorf("rag called %d times, want 1", rag.calls)
	}
	// Forwarded answers must be the anonymized variants (string went through gate).
	if got, _ := rag.gotAnswers["complaints_detail"].(string); !strings.Contains(got, "[OK]") {
		t.Errorf("forwarded answers not anonymized: %v", rag.gotAnswers)
	}
	if repo.calls != 1 {
		t.Errorf("repo SaveGeneration called %d times, want 1", repo.calls)
	}
	// Persisted record must carry anonymized fields only.
	if repo.saved.ContentAnonymized != "дневник [ДАТА]" || repo.saved.TitleSafe != "Ежедневный дневник" {
		t.Errorf("persisted wrong fields: %+v", repo.saved)
	}
	// Изоляция (Этап 9): doctor_id из context должен попасть в запись.
	if repo.saved.DoctorID == nil || *repo.saved.DoctorID != "doc-42" {
		t.Errorf("doctor_id must be scoped to authenticated doctor, got %v", repo.saved.DoctorID)
	}

	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	data, _ := env.Data.(map[string]any)
	if data["request_id"] != "db-rid" || data["status"] != "done" {
		t.Errorf("response data wrong: %+v", data)
	}

	// Anonymization summary: two free-text strings (complaints_detail, mood),
	// each yields one [ФИО] removal via cleanAnon → removed_count=2, {ФИО:2}.
	summary, ok := data["anonymization"].(map[string]any)
	if !ok {
		t.Fatalf("anonymization summary missing in data: %+v", data)
	}
	if rc, _ := summary["removed_count"].(float64); rc != 2 {
		t.Errorf("removed_count=%v, want 2", summary["removed_count"])
	}
	byType, _ := summary["removed_by_type"].(map[string]any)
	if c, _ := byType["ФИО"].(float64); c != 2 {
		t.Errorf("removed_by_type[ФИО]=%v, want 2 (%+v)", byType["ФИО"], byType)
	}
	// Persisted audit total = gateway input (2) + RAG downstream (3) = 5.
	if repo.saved.AnonymizerRemovedCount != 5 {
		t.Errorf("persisted anonymizer_removed_count=%d, want 5 (gateway 2 + rag 3)",
			repo.saved.AnonymizerRemovedCount)
	}
}

func TestGenerateHandler_NoPIISummary(t *testing.T) {
	rag := &fakeRAG{res: &ragclient.GenerateResult{
		RequestID: "rag-rid", Content: "ok", Status: "done",
	}}
	repo := &fakeRepo{id: "db-rid"}
	h := newGenerateHandler(config.Config{}, noPIIAnon{}, rag, repo)
	body := `{"document_type":"daily","answers":{"complaints_detail":"без особенностей"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	data, _ := env.Data.(map[string]any)
	summary, ok := data["anonymization"].(map[string]any)
	if !ok {
		t.Fatalf("anonymization summary missing: %+v", data)
	}
	if rc, _ := summary["removed_count"].(float64); rc != 0 {
		t.Errorf("removed_count=%v, want 0", summary["removed_count"])
	}
	if byType, _ := summary["removed_by_type"].(map[string]any); len(byType) != 0 {
		t.Errorf("removed_by_type must be empty when no PII, got %+v", byType)
	}
}

func TestGenerateHandler_PIIBlocked(t *testing.T) {
	rag := &fakeRAG{}
	repo := &fakeRepo{}
	h := newGenerateHandler(config.Config{}, blockingAnon{}, rag, repo)
	body := `{"document_type":"daily","answers":{"complaints_detail":"Иванов Иван"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d, want 422", rec.Code)
	}
	if rag.calls != 0 {
		t.Error("RAG must NOT be called when gateway gate blocks (fail-closed)")
	}
	if repo.calls != 0 {
		t.Error("store must NOT be called when gateway gate blocks")
	}
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error == nil || env.Error.Code != "PII_DETECTED" {
		t.Errorf("want PII_DETECTED, got %+v", env.Error)
	}
}

func TestGenerateHandler_RAGUnavailable(t *testing.T) {
	rag := &fakeRAG{err: ragclient.ErrUnavailable}
	repo := &fakeRepo{}
	h := newGenerateHandler(config.Config{}, &cleanAnon{}, rag, repo)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/generate",
		strings.NewReader(`{"document_type":"daily","answers":{}}`))
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", rec.Code)
	}
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error == nil || env.Error.Code != "LLM_UNAVAILABLE" {
		t.Errorf("want LLM_UNAVAILABLE, got %+v", env.Error)
	}
}

func TestGenerateHandler_RAGPIIPassthrough(t *testing.T) {
	rag := &fakeRAG{err: &ragclient.Error{HTTPStatus: 422, Code: "PII_DETECTED", Message: "ПДн"}}
	repo := &fakeRepo{}
	h := newGenerateHandler(config.Config{}, &cleanAnon{}, rag, repo)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/generate",
		strings.NewReader(`{"document_type":"daily","answers":{}}`))
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d, want 422", rec.Code)
	}
}

func TestGenerateHandler_InvalidDocType(t *testing.T) {
	h := newGenerateHandler(config.Config{}, &cleanAnon{}, &fakeRAG{}, &fakeRepo{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/generate",
		strings.NewReader(`{"document_type":"unknown","answers":{}}`))
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestGenerateHandler_PersistFailureStillReturns(t *testing.T) {
	rag := &fakeRAG{res: &ragclient.GenerateResult{
		RequestID: "rag-rid", Content: "ok", Status: "done",
	}}
	repo := &fakeRepo{saveErr: context.DeadlineExceeded}
	h := newGenerateHandler(config.Config{}, &cleanAnon{}, rag, repo)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/generate",
		strings.NewReader(`{"document_type":"daily","answers":{}}`))
	rec := httptest.NewRecorder()
	h(rec, req)
	// Generation succeeded; persistence failure must not lose the result.
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200 despite persist failure", rec.Code)
	}
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	data, _ := env.Data.(map[string]any)
	if data["request_id"] != "rag-rid" {
		t.Errorf("expected fallback to rag request_id, got %+v", data)
	}
}
