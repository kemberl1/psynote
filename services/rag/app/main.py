"""FastAPI application entrypoint for the RAG service.

Этап 3 (ingestion): /health + /admin/audit-sample (выгрузка выборки обезличенных
чанков для ручного аудита приватности, docs/04 §7). Сам ingestion запускается
CLI-командой `python -m app.ingest ingest` (one-shot, docs/03 §5).

Будущие роуты (retrieve/build-context для генерации) — Этап 4.
"""

from __future__ import annotations

import logging

from fastapi import FastAPI, HTTPException, Query

from app.config import get_settings
from app.qdrant_store import QdrantStore

logger = logging.getLogger(__name__)
settings = get_settings()

app = FastAPI(
    title="AI MED — RAG service",
    version="0.3.0",
    description=(
        "RAG-сервис: ingestion корпуса (CLI), чанкинг, локальные эмбеддинги e5, "
        "retrieval из Qdrant. См. docs/03_rag_design.md."
    ),
)


@app.get("/health", tags=["health"])
def health() -> dict[str, str]:
    """Liveness probe. Проверка из docs/10 Этап 0."""
    return {"status": "ok"}


@app.get("/admin/audit-sample", tags=["audit"])
def audit_sample(sample: int = Query(default=20, ge=1, le=200)) -> dict:
    """Выгрузить выборку ОБЕЗЛИЧЕННЫХ чанков для ручного аудита приватности.

    docs/04 §7: позволяет на приёмке проверить отсутствие ПДн в проиндексированных
    данных. Возвращает payload (text + метаданные) — все данные уже обезличены
    гейтом на этапе ingestion.
    """
    store = QdrantStore(settings)
    try:
        payloads = store.sample_payloads(limit=sample)
    except Exception as exc:  # noqa: BLE001
        logger.error("audit-sample: ошибка чтения Qdrant: %s",
                     type(exc).__name__)
        raise HTTPException(status_code=503,
                            detail="коллекция недоступна или пуста") from exc
    return {
        "collection": settings.qdrant_collection,
        "count": len(payloads),
        "chunks": payloads,
    }
