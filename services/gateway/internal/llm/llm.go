// Package llm is the OpenAI-compatible client for the X5 CoPilot LLM,
// with automatic model fallback (large → medium → small).
//
// See docs/03_rag_design.md §9–10:
//   - base_url, Bearer key, corporate CA bundle (LLM_CA_BUNDLE), timeouts/retries;
//   - fallback large→medium→small; 401/403 — без ретраев.
//
// Этап 1 (каркас): только интерфейс и заглушка. Реальный http.Client с
// tls.Config{RootCAs: pool} и go-openai-совместимым клиентом — Этап 3 роадмапа.
package llm

import "context"

// ChatRequest is a minimal chat-completion request.
type ChatRequest struct {
	Model       string
	Prompt      string
	Temperature float32
}

// Client performs chat completions against the LLM provider.
type Client interface {
	// Chat sends a prompt and returns the generated text.
	Chat(ctx context.Context, req ChatRequest) (string, error)
}

// Stub is a no-op placeholder.
type Stub struct{}

// Chat is a placeholder. TODO(этап 3): реальный вызов + fallback + ретраи.
func (Stub) Chat(_ context.Context, _ ChatRequest) (string, error) { return "", nil }
