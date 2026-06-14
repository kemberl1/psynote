// Package ragclient is the gateway's HTTP client for the Python RAG service.
//
// Role (docs/02 §3, docs/07 §5): the Go gateway is the orchestrator / data
// gatekeeper. It forwards an ALREADY-anonymized generation request to the RAG
// service (POST {RAG_URL}/generate) and maps the RAG envelope/error codes back
// to typed Go errors so the public /api/v1/generate handler can return the
// contract codes (422 PII_DETECTED, 503 LLM_UNAVAILABLE, …).
//
// PRIVACY (docs/04, docs/09 §4): only anonymized text leaves the gateway, and
// this client NEVER logs request/response bodies (they may transit free-text).
// Logs carry status codes and error categories only.
//
// Этап 5: реальный клиент к rag:8000 заменяет каркасную заглушку.
package ragclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ─── Контракт запроса/ответа (docs/07 §5, services/rag/app/main.py) ──────────

// GenerateRequest is the payload sent to RAG POST /generate.
//
// Answers must already be anonymized by the gateway gate (fail-closed first
// line of defence, docs/04 §1) before they are placed here.
type GenerateRequest struct {
	DocumentType string         `json:"document_type"`
	Answers      map[string]any `json:"answers"`
	Options      map[string]any `json:"options,omitempty"`
}

// Retrieval mirrors data.retrieval from the RAG response (docs/05 §3.2).
type Retrieval struct {
	ChunksUsed     int    `json:"chunks_used"`
	Syndrome       string `json:"syndrome"`
	DiagnosisClass string `json:"diagnosis_class"`
	Dynamics       string `json:"dynamics"`
}

// GenerateResult is the flattened, gateway-facing result of a RAG generation.
// It joins the RAG meta + data into one struct the orchestrator persists and
// returns (docs/07 §5, docs/05 §2.2).
type GenerateResult struct {
	// From meta.
	RequestID    string `json:"request_id"`
	LLMModelUsed string `json:"llm_model_used"`
	TokensUsed   int    `json:"tokens_used"`
	ChunksUsed   int    `json:"chunks_used"`
	// From data.
	DocumentType           string         `json:"document_type"`
	Content                string         `json:"content"`
	Status                 string         `json:"status"`
	TitleSafe              string         `json:"title_safe"`
	AnswersAnonymized      map[string]any `json:"answers_anonymized"`
	AnonymizerRemovedCount int            `json:"anonymizer_removed_count"`
	Retrieval              Retrieval      `json:"retrieval"`
}

// ─── Ошибки клиента (мапятся в коды docs/07 §1) ──────────────────────────────

// Error is a typed RAG error carrying the upstream contract code so the
// handler can translate it to the right HTTP status (docs/07 §1).
type Error struct {
	// HTTPStatus is the status code returned by the RAG service.
	HTTPStatus int
	// Code is the contract error code (PII_DETECTED, LLM_UNAVAILABLE, …).
	Code string
	// Message is a human-readable, PII-free message.
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("ragclient: rag returned %d %s: %s", e.HTTPStatus, e.Code, e.Message)
}

// ErrUnavailable is returned for transport-level failures (timeout, connection
// refused, DNS) — the RAG service itself is unreachable. Maps to 503.
var ErrUnavailable = errors.New("ragclient: rag service unavailable")

// ─── Интерфейс и реализация ──────────────────────────────────────────────────

// Client talks to the RAG service.
type Client interface {
	// Generate orchestrates a diary generation through RAG (docs/07 §5).
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error)
	// Health probes RAG liveness (docs/07 §8). Returns nil when reachable.
	Health(ctx context.Context) error
}

// HTTPClient is the production RAG client over HTTP/JSON.
type HTTPClient struct {
	baseURL string
	http    *http.Client
	// healthTimeout bounds the Health probe independently of generation.
	healthTimeout time.Duration
}

// Options configures the HTTPClient.
type Options struct {
	// BaseURL of the RAG service, e.g. http://rag:8000 (ENV RAG_URL).
	BaseURL string
	// GenerateTimeout is the (generous) timeout for one generation call.
	GenerateTimeout time.Duration
	// HealthTimeout is the short timeout for the health probe.
	HealthTimeout time.Duration
}

// New builds a production HTTP RAG client.
func New(opts Options) *HTTPClient {
	gt := opts.GenerateTimeout
	if gt <= 0 {
		gt = 120 * time.Second
	}
	ht := opts.HealthTimeout
	if ht <= 0 {
		ht = 5 * time.Second
	}
	return &HTTPClient{
		baseURL: strings.TrimRight(opts.BaseURL, "/"),
		// Per-request deadlines come from context; the client timeout is a
		// generous backstop covering the whole generation pipeline.
		http:          &http.Client{Timeout: gt},
		healthTimeout: ht,
	}
}

// ragEnvelope is the {meta, data|error} envelope returned by RAG (docs/07 §1).
type ragEnvelope struct {
	Meta struct {
		RequestID    string `json:"request_id"`
		TS           string `json:"ts"`
		LLMModelUsed string `json:"llm_model_used"`
		TokensUsed   int    `json:"tokens_used"`
		ChunksUsed   int    `json:"chunks_used"`
	} `json:"meta"`
	Data *struct {
		DocumentType           string         `json:"document_type"`
		Content                string         `json:"content"`
		Status                 string         `json:"status"`
		TitleSafe              string         `json:"title_safe"`
		AnswersAnonymized      map[string]any `json:"answers_anonymized"`
		AnonymizerRemovedCount int            `json:"anonymizer_removed_count"`
		Retrieval              Retrieval      `json:"retrieval"`
	} `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Generate posts the anonymized request to RAG and maps the response.
func (c *HTTPClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ragclient: marshal request: %w", err)
	}

	url := c.baseURL + "/generate"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ragclient: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		// Transport-level failure (timeout / refused) — RAG unreachable.
		// NB: err may wrap the URL but never the body, so it carries no PII.
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Cap the read to protect the gateway from a misbehaving upstream.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}

	var env ragEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		// Non-JSON / unexpected upstream payload — do NOT echo the body (PII).
		return nil, &Error{
			HTTPStatus: resp.StatusCode,
			Code:       "RAG_BAD_RESPONSE",
			Message:    "некорректный ответ RAG-сервиса",
		}
	}

	if resp.StatusCode != http.StatusOK {
		code := "RAG_ERROR"
		msg := "ошибка RAG-сервиса"
		if env.Error != nil {
			if env.Error.Code != "" {
				code = env.Error.Code
			}
			if env.Error.Message != "" {
				msg = env.Error.Message
			}
		}
		return nil, &Error{HTTPStatus: resp.StatusCode, Code: code, Message: msg}
	}

	if env.Data == nil {
		return nil, &Error{
			HTTPStatus: resp.StatusCode,
			Code:       "RAG_BAD_RESPONSE",
			Message:    "пустой data в ответе RAG",
		}
	}

	return &GenerateResult{
		RequestID:              env.Meta.RequestID,
		LLMModelUsed:           env.Meta.LLMModelUsed,
		TokensUsed:             env.Meta.TokensUsed,
		ChunksUsed:             env.Meta.ChunksUsed,
		DocumentType:           env.Data.DocumentType,
		Content:                env.Data.Content,
		Status:                 env.Data.Status,
		TitleSafe:              env.Data.TitleSafe,
		AnswersAnonymized:      env.Data.AnswersAnonymized,
		AnonymizerRemovedCount: env.Data.AnonymizerRemovedCount,
		Retrieval:              env.Data.Retrieval,
	}, nil
}

// Health probes GET {baseURL}/health (docs/07 §8).
func (c *HTTPClient) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.healthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("ragclient: build health request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: health status %d", ErrUnavailable, resp.StatusCode)
	}
	return nil
}

// Ensure HTTPClient satisfies the interface.
var _ Client = (*HTTPClient)(nil)
