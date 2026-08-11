// Command gateway is the entrypoint for the AI MED API-Gateway service.
//
// Role of this service (see docs/02_system_architecture.md §4):
//   - API-Gateway: "front door" for all frontend requests.
//   - Anonymization gate: strips PII before anything is stored / sent to the LLM.
//   - Export service: builds Word/PDF documents.
//   - Orchestration: coordinates RAG (Python) and the OpenAI-compatible LLM with fallback.
//
// Этап 10: wiring admin deps (AdminRepository, AdminRAG) for document upload.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aimed/gateway/internal/anonymizer"
	"github.com/aimed/gateway/internal/auth"
	"github.com/aimed/gateway/internal/config"
	"github.com/aimed/gateway/internal/handlers"
	"github.com/aimed/gateway/internal/ragclient"
	"github.com/aimed/gateway/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := config.Load()

	// PII-гейт (docs/04). Словари — встроенные (go:embed) или из ANONYMIZER_DICT_DIR.
	anon, err := anonymizer.New(anonymizer.Options{
		DictionaryDir: cfg.AnonymizerDictDir,
		FailClosed:    cfg.AnonymizerFailClosed,
	})
	if err != nil {
		slog.Error("anonymizer init failed", "error", err)
		os.Exit(1)
	}

	// RAG client (оркестрация генерации, docs/02 §3). URL из ENV RAG_URL.
	rag := ragclient.New(ragclient.Options{
		BaseURL:         cfg.RAGBaseURL,
		GenerateTimeout: cfg.RAGGenerateTimeout,
		HealthTimeout:   cfg.RAGHealthTimeout,
	})

	// PostgreSQL persistence (ОБЕЗЛИЧЕННАЯ история, docs/05).
	var repo store.Repository
	var pgRepo *store.PgxRepository
	if cfg.PostgresDSN != "" {
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
		pr, derr := store.NewPgxRepository(dbCtx, cfg.PostgresDSN)
		dbCancel()
		if derr != nil {
			slog.Error("postgres connect failed; history/generate disabled", "error_type", "store")
		} else {
			pgRepo = pr
			repo = pr
			defer pr.Close()
		}
	} else {
		slog.Warn("POSTGRES_DSN empty; history/generate disabled")
	}

	// Auth (Этап 9, docs/09): сервис JWT/refresh.
	var tokens *auth.TokenService
	if cfg.JWTSecret != "" {
		ts, terr := auth.NewTokenService(cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
		if terr != nil {
			slog.Error("auth disabled: token service init failed", "error_type", "auth")
		} else {
			tokens = ts
		}
	} else {
		slog.Warn("JWT_SECRET empty; auth disabled (protected routes return 503)")
	}

	// Admin deps (Этап 10): reuse PgxRepository for admin_document table,
	// reuse the same RAG HTTP client (satisfies AdminIngestClient via IngestFile).
	var adminRepo store.AdminRepository
	var adminRAG ragclient.AdminIngestClient
	if pgRepo != nil {
		adminRepo = pgRepo
	}
	adminRAG = rag

	mux := handlers.NewRouter(cfg, handlers.Deps{
		Anonymizer: anon,
		RAG:        rag,
		AdminRAG:   adminRAG,
		Repo:       repo,
		AdminRepo:  adminRepo,
		Tokens:     tokens,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	go func() {
		slog.Info("gateway starting", "addr", cfg.HTTPAddr, "api_prefix", config.APIPrefix)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("gateway shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
}
