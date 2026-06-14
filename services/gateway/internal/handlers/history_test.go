package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aimed/gateway/internal/config"
	"github.com/aimed/gateway/internal/ragclient"
	"github.com/aimed/gateway/internal/store"
)

func TestHistoryListHandler(t *testing.T) {
	repo := &fakeRepo{
		total: 2,
		list: []store.HistoryItem{
			{RequestID: "r1", DocumentType: "daily", TitleSafe: "Ежедневный дневник", LLMModelUsed: "x5-airun-large", Status: "done", CreatedAt: time.Now()},
			{RequestID: "r2", DocumentType: "exam_10d", TitleSafe: "Осмотр", LLMModelUsed: "x5-airun-medium", Status: "failed", CreatedAt: time.Now()},
		},
	}
	h := newHistoryListHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/requests?limit=20&offset=0", nil)
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Decode into a raw shape so we assert the JSON key is actually present and
	// non-null (the приёмка bug: status came back null in /requests).
	var env struct {
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if env.Meta.Total != 2 || len(env.Data) != 2 {
		t.Errorf("unexpected list/meta: total=%d len=%d", env.Meta.Total, len(env.Data))
	}
	if env.Data[0]["title_safe"] == "" {
		t.Error("title_safe missing in list item")
	}
	// status must be mapped through from the DB row (regression: was null).
	if got := env.Data[0]["status"]; got != "done" {
		t.Errorf("list item status=%v, want \"done\" (regression: status null)", got)
	}
	if got := env.Data[1]["status"]; got != "failed" {
		t.Errorf("list item[1] status=%v, want \"failed\"", got)
	}
}

func TestHistoryDetailHandler_OK(t *testing.T) {
	repo := &fakeRepo{detail: &store.HistoryDetail{
		RequestID: "r1", DocumentType: "daily", Content: "текст [ДАТА]",
		TitleSafe: "Ежедневный дневник", Status: "done",
		AnonymizerRemovedCount: 4,
		AnswersAnonymized:      map[string]any{"mood": "lowered"},
	}}
	h := newHistoryDetailHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/requests/r1", nil)
	req.SetPathValue("id", "r1")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Decode into a raw shape to assert keys are present and non-null
	// (the приёмка bug: anonymizer_removed_count came back null in /requests/{id}).
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if got := env.Data["status"]; got != "done" {
		t.Errorf("detail status=%v, want \"done\"", got)
	}
	if got, _ := env.Data["anonymizer_removed_count"].(float64); got != 4 {
		t.Errorf("detail anonymizer_removed_count=%v, want 4 (regression: was null)",
			env.Data["anonymizer_removed_count"])
	}
}

func TestHistoryDetailHandler_NotFound(t *testing.T) {
	repo := &fakeRepo{getErr: store.ErrNotFound}
	h := newHistoryDetailHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/requests/missing", nil)
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rec.Code)
	}
}

func TestDocumentTypesHandler(t *testing.T) {
	h := newDocumentTypesHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/document-types", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var env struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if len(env.Data) != 2 {
		t.Errorf("want 2 document types, got %d", len(env.Data))
	}
}

func TestQuestionnaireHandler(t *testing.T) {
	h := newQuestionnaireHandler()

	// missing param → 400
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/v1/questionnaire", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing document_type: status=%d, want 400", rec.Code)
	}

	// unknown type → 404
	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/v1/questionnaire?document_type=nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown type: status=%d, want 404", rec.Code)
	}

	// daily → 200 with questions
	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/v1/questionnaire?document_type=daily", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("daily: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data struct {
			DocumentType string           `json:"document_type"`
			Questions    []map[string]any `json:"questions"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data.DocumentType != "daily" || len(env.Data.Questions) == 0 {
		t.Errorf("daily schema malformed: %+v", env.Data)
	}
}

// TestRouter_Wiring checks routes are registered and CORS preflight works.
func TestRouter_Wiring(t *testing.T) {
	cfg := config.Config{CORSAllowedOrigin: "http://localhost:5174"}
	mux := NewRouter(cfg, Deps{
		Anonymizer: &cleanAnon{},
		RAG:        &fakeRAG{res: &ragclient.GenerateResult{Content: "ok", Status: "done"}},
		Repo:       &fakeRepo{},
	})

	// health
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("health status=%d", rec.Code)
	}

	// document-types
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/document-types", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("document-types status=%d", rec.Code)
	}

	// CORS preflight
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/api/v1/generate", nil))
	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status=%d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5174" {
		t.Errorf("CORS origin header missing: %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}
