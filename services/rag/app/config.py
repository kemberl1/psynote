"""Configuration for the RAG service, loaded from environment variables.

See docs/05_data_model.md §4 (секреты в .env) и docs/03_rag_design.md.
Эмбеддинги считаются ЛОКАЛЬНО (docs/03 §3) — облачный embed-API не используется
для медкорпуса ради приватности.
"""

from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Settings:
    """Runtime settings for the RAG service."""

    # Qdrant vector DB (docs/02 §7 — :6333).
    qdrant_url: str = os.getenv("QDRANT_URL", "http://qdrant:6333")
    qdrant_collection: str = os.getenv("QDRANT_COLLECTION", "corpus_diaries")

    # Локальная модель эмбеддингов (docs/03 §3 — multilingual-e5-large).
    # TODO(этап 2): загрузка модели через sentence-transformers.
    embedding_model: str = os.getenv(
        "EMBEDDING_MODEL", "intfloat/multilingual-e5-large")

    # HTTP server.
    host: str = os.getenv("RAG_HOST", "0.0.0.0")
    port: int = int(os.getenv("RAG_PORT", "8000"))


def get_settings() -> Settings:
    """Return the process settings (single instance is fine for the skeleton)."""
    return Settings()
