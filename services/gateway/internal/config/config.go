// Package config loads gateway configuration from environment variables.
//
// See docs/05_data_model.md §4 (Конфигурация и секреты) and
// docs/09_security_privacy.md §6 — secrets live in .env, never in code/repo.
//
// Этап 1: загрузка значений из окружения. Реальное использование секретов
// (LLM-вызовы, JWT-подпись, подключение к БД) появится на следующих этапах.
package config

import (
	"os"
)

// APIPrefix is the common REST prefix (see docs/07_api_contract.md §1).
const APIPrefix = "/api/v1"

// Config holds all runtime configuration for the gateway.
//
// NOTE: значения секретов здесь НЕ логируются (см. docs/09 §4).
type Config struct {
	// HTTP server.
	HTTPAddr string

	// PostgreSQL (без ПДн — см. docs/05_data_model.md).
	PostgresDSN string

	// Qdrant vector DB.
	QdrantURL string

	// RAG service (Python FastAPI).
	RAGBaseURL string

	// JWT auth (см. docs/09_security_privacy.md §1).
	JWTSecret string

	// LLM (X5 CoPilot, OpenAI-совместимый) — см. docs/03_rag_design.md §9.
	// TODO(этап 3): использовать при реализации LLM-клиента с fallback.
	LLMBaseURL     string
	LLMAPIKey      string
	LLMCABundle    string
	LLMModelLarge  string
	LLMModelMedium string
	LLMModelSmall  string

	// CORS: разрешённый origin фронтенда (см. docs/09 §6).
	CORSAllowedOrigin string
}

// Load reads configuration from the environment, applying sensible defaults
// suitable for the docker-compose topology (см. docs/02 §7).
func Load() Config {
	return Config{
		HTTPAddr:    getEnv("GATEWAY_HTTP_ADDR", ":8080"),
		PostgresDSN: getEnv("POSTGRES_DSN", ""),
		QdrantURL:   getEnv("QDRANT_URL", "http://qdrant:6333"),
		RAGBaseURL:  getEnv("RAG_BASE_URL", "http://rag:8000"),
		JWTSecret:   getEnv("JWT_SECRET", ""),

		LLMBaseURL:     getEnv("X5_BASE_URL", "https://api-copilot.x5.ru/aigw/v1/"),
		LLMAPIKey:      getEnv("X5_API_KEY", ""),
		LLMCABundle:    getEnv("LLM_CA_BUNDLE", "/app/certs/x5_root_ca.pem"),
		LLMModelLarge:  getEnv("LLM_MODEL_LARGE", "x5-airun-large"),
		LLMModelMedium: getEnv("LLM_MODEL_MEDIUM", "x5-airun-medium"),
		LLMModelSmall:  getEnv("LLM_MODEL_SMALL", "x5-airun-small"),

		CORSAllowedOrigin: getEnv("CORS_ALLOWED_ORIGIN", "http://localhost:5173"),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
