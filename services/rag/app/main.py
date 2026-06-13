"""FastAPI application entrypoint for the RAG service.

Этап 1 (каркас): только /health. Будущие роуты (build_context, ingest)
добавятся на Этапах 2–3 роадмапа (docs/10_roadmap_stepbystep.md).
"""

from __future__ import annotations

from fastapi import FastAPI

from app.config import get_settings

settings = get_settings()

app = FastAPI(
    title="AI MED — RAG service",
    version="0.1.0",
    description=(
        "RAG-сервис: чанкинг, локальные эмбеддинги, retrieval из Qdrant, "
        "построение промпта, ingestion корпуса. См. docs/03_rag_design.md."
    ),
)


@app.get("/health", tags=["health"])
def health() -> dict[str, str]:
    """Liveness probe. Проверка из docs/10 Этап 0."""
    return {"status": "ok"}


# TODO(этап 2): POST /ingest — приём ОБЕЗЛИЧЕННОГО документа → чанкинг →
#               эмбеддинги → upsert в Qdrant (docs/03 §5).
# TODO(этап 2): POST /retrieve — поиск top-k обезличенных образцов с фильтрами
#               по метаданным (docs/03 §6).
# TODO(этап 3): POST /build-context — сборка few-shot контекста для генерации.
