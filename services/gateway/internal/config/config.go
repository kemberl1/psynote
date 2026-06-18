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
	"strconv"
	"time"
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
	// RAGGenerateTimeout — щедрый таймаут на оркестрацию генерации (LLM долгая,
	// docs/03 §10). Из ENV RAG_GENERATE_TIMEOUT_S (по умолчанию 120s).
	RAGGenerateTimeout time.Duration
	// RAGHealthTimeout — короткий таймаут для health-проверки RAG.
	RAGHealthTimeout time.Duration

	// JWT auth (см. docs/09_security_privacy.md §1).
	JWTSecret string
	// AccessTokenTTL — время жизни access-JWT (короткое, ~15 мин, docs/09 §1.3).
	AccessTokenTTL time.Duration
	// RefreshTokenTTL — время жизни refresh-токена (длинное, ~7–30 дней).
	RefreshTokenTTL time.Duration

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

	// Anonymizer (PII-гейт, см. docs/04_anonymization.md).
	// AnonymizerDictDir — путь к словарям на диске; пусто = встроенные (go:embed).
	AnonymizerDictDir string
	// AnonymizerNERURL — адрес ЛОКАЛЬНОГО Python NER-сайдкара (Natasha).
	// Пусто = NER отключён, работает Go-only MVP (docs/04 §6). PII не покидает периметр.
	AnonymizerNERURL string
	// AnonymizerFailClosed — при сомнении блокировать (по умолчанию true, docs/04 §1).
	AnonymizerFailClosed bool
}

// Load reads configuration from the environment, applying sensible defaults
// suitable for the docker-compose topology (см. docs/02 §7).
func Load() Config {
	return Config{
		HTTPAddr:    getEnv("GATEWAY_HTTP_ADDR", ":8080"),
		PostgresDSN: getEnv("POSTGRES_DSN", ""),
		QdrantURL:   getEnv("QDRANT_URL", "http://qdrant:6333"),
		// RAG_URL — предпочтительное имя (задание Этапа 5); RAG_BASE_URL —
		// исторический алиас (каркас). Поддерживаем оба, RAG_URL выигрывает.
		RAGBaseURL:         getEnv("RAG_URL", getEnv("RAG_BASE_URL", "http://rag:8000")),
		RAGGenerateTimeout: getEnvDuration("RAG_GENERATE_TIMEOUT_S", 120*time.Second),
		RAGHealthTimeout:   getEnvDuration("RAG_HEALTH_TIMEOUT_S", 5*time.Second),
		JWTSecret:          getEnv("JWT_SECRET", ""),
		AccessTokenTTL:     getEnvDuration("ACCESS_TOKEN_TTL_S", 15*time.Minute),
		RefreshTokenTTL:    getEnvDuration("REFRESH_TOKEN_TTL_S", 30*24*time.Hour),

		LLMBaseURL:     getEnv("X5_BASE_URL", "https://api-copilot.x5.ru/aigw/v1/"),
		LLMAPIKey:      getEnv("X5_API_KEY", ""),
		LLMCABundle:    getEnv("LLM_CA_BUNDLE", "/app/certs/x5_root_ca.pem"),
		LLMModelLarge:  getEnv("LLM_MODEL_LARGE", "x5-airun-large"),
		LLMModelMedium: getEnv("LLM_MODEL_MEDIUM", "x5-airun-medium"),
		LLMModelSmall:  getEnv("LLM_MODEL_SMALL", "x5-airun-small"),

		CORSAllowedOrigin: getEnv("CORS_ALLOWED_ORIGIN", "http://localhost:5173"),

		AnonymizerDictDir:    getEnv("ANONYMIZER_DICT_DIR", ""),
		AnonymizerNERURL:     getEnv("ANONYMIZER_NER_URL", ""),
		AnonymizerFailClosed: getEnvBool("ANONYMIZER_FAIL_CLOSED", true),
	}
}

// getEnvDuration reads an integer "seconds" env var into a time.Duration.
// Missing/empty/invalid falls back to the default.
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs <= 0 {
		return fallback
	}
	return time.Duration(secs) * time.Second
}

// getEnvBool reads a boolean env var, treating "0"/"false"/"no" as false and
// any other non-empty value as true; missing/empty falls back to the default.
func getEnvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	switch v {
	case "0", "false", "FALSE", "False", "no", "NO":
		return false
	default:
		return true
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
