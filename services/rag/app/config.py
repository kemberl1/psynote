"""Configuration for the RAG service, loaded from environment variables.

See docs/05_data_model.md §4 (секреты в .env) и docs/03_rag_design.md.

Эмбеддинги считаются ЛОКАЛЬНО (docs/03 §3) — облачный embed-API не используется
для медкорпуса ради приватности (NFR-P3).

КРИТИЧНО (docs/04 §1): анонимизация выполняется ЕДИНСТВЕННЫМ источником истины —
Go-анонимайзером в gateway. Python-ingestion вызывает gateway /api/v1/anonymize
для каждого фрагмента ДО эмбеддинга и записи в Qdrant. URL берётся из ENV.
"""

from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Settings:
    """Runtime settings for the RAG service. Всё конфигурируется через ENV."""

    # ─── Qdrant vector DB (docs/02 §7 — :6333, docs/05 §3) ───────────────────
    qdrant_url: str = os.getenv("QDRANT_URL", "http://qdrant:6333")
    qdrant_collection: str = os.getenv("QDRANT_COLLECTION", "corpus_diaries")

    # ─── Локальная модель эмбеддингов (docs/03 §3 — multilingual-e5-large) ───
    embedding_model: str = os.getenv(
        "EMBEDDING_MODEL", "intfloat/multilingual-e5-large")
    # Размерность вектора e5-large = 1024 (docs/05 §3.2). Для -base = 768.
    # Если поменяли модель — поправьте размерность (или 0 = определить по модели).
    embedding_dim: int = int(os.getenv("EMBEDDING_DIM", "1024"))
    # Кэш HF-моделей (монтируется volume, чтобы не качать модель при каждом ране).
    hf_home: str = os.getenv("HF_HOME", "/app/.cache/huggingface")
    # Устройство инференса: cpu | cuda. В docker по умолчанию cpu.
    embedding_device: str = os.getenv("EMBEDDING_DEVICE", "cpu")
    embedding_batch_size: int = int(os.getenv("EMBEDDING_BATCH_SIZE", "16"))

    # ─── Анонимайзер-гейт (Go gateway, docs/04 §1) ───────────────────────────
    # URL gateway внутри docker-сети. НЕ хардкодим (docs/04, критичный принцип).
    gateway_url: str = os.getenv("GATEWAY_URL", "http://gateway:8080")
    anonymize_path: str = os.getenv("ANONYMIZE_PATH", "/api/v1/anonymize")
    # Таймауты/ретраи HTTP-клиента к анонимайзеру.
    anonymize_timeout_s: float = float(os.getenv("ANONYMIZE_TIMEOUT_S", "30"))
    anonymize_retries: int = int(os.getenv("ANONYMIZE_RETRIES", "3"))
    anonymize_backoff_s: float = float(os.getenv("ANONYMIZE_BACKOFF_S", "1.0"))

    # ─── Корпус (docs/03 §5). Монтируется volume read-only, НЕ копируется в образ. ─
    corpus_dir: str = os.getenv("CORPUS_DIR", "/data/corpus")
    # Подпапка с дневниками внутри корпуса (фокус Этапа 3, docs/03 §4).
    corpus_diaries_subdir: str = os.getenv(
        "CORPUS_DIARIES_SUBDIR", "02_корпус/сборник_дневников_ИБ")

    # ─── Чанкинг (docs/03 §4) ────────────────────────────────────────────────
    # Максимальный размер чанка в символах (мягкая граница; режем по секциям/записям).
    chunk_max_chars: int = int(os.getenv("CHUNK_MAX_CHARS", "1200"))
    chunk_overlap_chars: int = int(os.getenv("CHUNK_OVERLAP_CHARS", "150"))
    # Минимальный размер чанка — короче отбрасываем как шум (подписи и т.п.).
    chunk_min_chars: int = int(os.getenv("CHUNK_MIN_CHARS", "40"))

    # ─── HTTP server ─────────────────────────────────────────────────────────
    host: str = os.getenv("RAG_HOST", "0.0.0.0")
    port: int = int(os.getenv("RAG_PORT", "8000"))


def get_settings() -> Settings:
    """Return the process settings (single instance is fine here)."""
    return Settings()
