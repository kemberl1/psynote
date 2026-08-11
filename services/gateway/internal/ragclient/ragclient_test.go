package ragclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(srv *httptest.Server) *HTTPClient {
	return New(Options{
		BaseURL:         srv.URL,
		GenerateTimeout: 2 * time.Second,
		HealthTimeout:   1 * time.Second,
	})
}

func TestGenerate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/generate" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"meta": {"request_id":"rid-1","ts":"2026-01-01T00:00:00Z",
			         "llm_model_used":"deepseek-v4-flash","tokens_used":812,"chunks_used":5},
			"data": {"document_type":"daily","content":"дневник [ДАТА]","status":"done",
			         "title_safe":"Ежедневный дневник · тревожный","answers_anonymized":{"mood":"lowered"},
			         "anonymizer_removed_count":2,
			         "retrieval":{"chunks_used":5,"syndrome":"тревожный","diagnosis_class":"F4x","dynamics":"без_динамики"}}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	res, err := c.Generate(context.Background(), GenerateRequest{DocumentType: "daily", Answers: map[string]any{"mood": "lowered"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RequestID != "rid-1" || res.LLMModelUsed != "deepseek-v4-flash" || res.TokensUsed != 812 {
		t.Errorf("meta mapping wrong: %+v", res)
	}
	if res.Content != "дневник [ДАТА]" || res.Status != "done" || res.TitleSafe == "" {
		t.Errorf("data mapping wrong: %+v", res)
	}
	if res.AnonymizerRemovedCount != 2 || res.Retrieval.ChunksUsed != 5 || res.Retrieval.Syndrome != "тревожный" {
		t.Errorf("retrieval/count mapping wrong: %+v", res)
	}
}

func TestGenerate_PII422(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"meta":{"request_id":"x"},"error":{"code":"PII_DETECTED","message":"ПДн"}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Generate(context.Background(), GenerateRequest{DocumentType: "daily"})
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("expected *Error, got %v", err)
	}
	if rerr.HTTPStatus != http.StatusUnprocessableEntity || rerr.Code != "PII_DETECTED" {
		t.Errorf("wrong mapping: %+v", rerr)
	}
}

func TestGenerate_LLM503(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"meta":{},"error":{"code":"LLM_UNAVAILABLE","message":"недоступен"}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Generate(context.Background(), GenerateRequest{DocumentType: "daily"})
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("expected *Error, got %v", err)
	}
	if rerr.HTTPStatus != http.StatusServiceUnavailable || rerr.Code != "LLM_UNAVAILABLE" {
		t.Errorf("wrong mapping: %+v", rerr)
	}
}

func TestGenerate_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(Options{BaseURL: srv.URL, GenerateTimeout: 2 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.Generate(ctx, GenerateRequest{DocumentType: "daily"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable on timeout, got %v", err)
	}
}

func TestGenerate_ConnectionRefused(t *testing.T) {
	// Point at a closed server.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	c := New(Options{BaseURL: url, GenerateTimeout: time.Second})
	_, err := c.Generate(context.Background(), GenerateRequest{DocumentType: "daily"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable on refused, got %v", err)
	}
}

func TestGenerate_BadResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json at all`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Generate(context.Background(), GenerateRequest{DocumentType: "daily"})
	var rerr *Error
	if !errors.As(err, &rerr) || rerr.Code != "RAG_BAD_RESPONSE" {
		t.Fatalf("expected RAG_BAD_RESPONSE, got %v", err)
	}
}

func TestHealth(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer okSrv.Close()

	if err := newTestClient(okSrv).Health(context.Background()); err != nil {
		t.Errorf("expected healthy, got %v", err)
	}

	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badSrv.Close()
	if err := newTestClient(badSrv).Health(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
}
