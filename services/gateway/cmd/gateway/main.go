// Command gateway is the entrypoint for the AI MED API-Gateway service.
//
// Role of this service (see docs/02_system_architecture.md §4):
//   - API-Gateway: "front door" for all frontend requests.
//   - Anonymization gate: strips PII before anything is stored / sent to the LLM.
//   - Export service: builds Word/PDF documents.
//   - Orchestration: coordinates RAG (Python) and the X5 CoPilot LLM with fallback.
//
// Этап 1 (каркас): здесь поднимается только HTTP-сервер с health-эндпоинтом.
// Бизнес-логика (auth, анонимизация, экспорт, оркестрация) — заглушки в internal/.
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
	"github.com/aimed/gateway/internal/config"
	"github.com/aimed/gateway/internal/handlers"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := config.Load()

	// PII-гейт (docs/04). Словари — встроенные (go:embed) или из ANONYMIZER_DICT_DIR.
	// NER-сайдкар на Этапе 2 не подключаем — работает Go-only MVP (docs/04 §6).
	anon, err := anonymizer.New(anonymizer.Options{
		DictionaryDir: cfg.AnonymizerDictDir,
		FailClosed:    cfg.AnonymizerFailClosed,
	})
	if err != nil {
		slog.Error("anonymizer init failed", "error", err)
		os.Exit(1)
	}

	mux := handlers.NewRouter(cfg, anon)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
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
