"""FastAPI application entrypoint for the RAG service.

Этап 3 (ingestion): /health + /admin/audit-sample.
Этап 4 (генерация): POST /generate — главный эндпоинт генерации дневника.
Этап 10: POST /ingest — загрузка файла через admin UI (multipart/form-data).
"""

from __future__ import annotations

import logging
import tempfile
import uuid
from datetime import datetime, timezone
from pathlib import Path

from fastapi import FastAPI, HTTPException, Query, File, UploadFile
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field

from app.config import get_settings
from app.llm_client import (
    AllModelsUnavailableError,
    LLMAuthError,
    LLMError,
    LLMNotConfiguredError,
)
from app.pipeline import (
    DiaryGenerator,
    PiiBlockedError,
    UnsupportedDocTypeError,
)
from app.qdrant_store import QdrantStore
from app.templates import SUPPORTED_DOC_TYPES

logger = logging.getLogger(__name__)
settings = get_settings()

app = FastAPI(
    title="AI MED — RAG service",
    version="0.5.0",
    description=(
        "RAG-сервис: ingestion корпуса (CLI), retrieval из Qdrant, генерация "
        "психиатрических дневников (LLM X5 с фолбэком), загрузка документов "
        "через admin UI (Этап 10). См. docs/03_rag_design.md."
    ),
)


# ─── Конверт ответа (docs/07 §1) ─────────────────────────────────────────────
def _meta(extra: dict | None = None) -> dict:
    meta = {
        "request_id": str(uuid.uuid4()),
        "ts": datetime.now(timezone.utc).isoformat(),
    }
    if extra:
        meta.update(extra)
    return meta


def _error_response(status: int, code: str, message: str) -> JSONResponse:
    return JSONResponse(
        status_code=status,
        content={"meta": _meta(), "error": {"code": code, "message": message}},
    )


# ─── Схемы запроса/ответа (docs/07 §5) ───────────────────────────────────────
class GenerateRequest(BaseModel):
    document_type: str = Field(..., description="daily | exam_10d")
    answers: dict = Field(default_factory=dict)
    options: dict | None = Field(default=None)


@app.get("/health", tags=["health"])
def health() -> dict:
    models = settings.llm_models()
    return {
        "status": "ok",
        "llm": {
            "configured": bool(settings.x5_api_key),
            "models": models,
        },
        "supported_document_types": list(SUPPORTED_DOC_TYPES),
    }


@app.post("/generate", tags=["generation"])
def generate(req: GenerateRequest) -> JSONResponse:
    generator = DiaryGenerator(settings)
    try:
        result = generator.generate(req.document_type, req.answers)
    except UnsupportedDocTypeError as exc:
        return _error_response(400, "INVALID_DOCUMENT_TYPE", str(exc))
    except PiiBlockedError as exc:
        return _error_response(422, "PII_DETECTED", str(exc))
    except LLMNotConfiguredError as exc:
        logger.error("generate: LLM не сконфигурирован")
        return _error_response(503, "LLM_NOT_CONFIGURED", str(exc))
    except (AllModelsUnavailableError, LLMAuthError, LLMError) as exc:
        if isinstance(exc, LLMAuthError):
            logger.error("generate: ошибка авторизации LLM")
            return _error_response(
                503, "LLM_AUTH_ERROR",
                "Ошибка авторизации LLM (проверьте X5_API_KEY)")
        logger.error("generate: LLM недоступен (%s)", type(exc).__name__)
        return _error_response(503, "LLM_UNAVAILABLE",
                               "Сервис генерации временно недоступен")
    except Exception as exc:
        logger.error("generate: внутренняя ошибка: %s", type(exc).__name__)
        return _error_response(500, "INTERNAL_ERROR", "Внутренняя ошибка сервиса")
    finally:
        generator.close()

    return JSONResponse(
        status_code=200,
        content={
            "meta": _meta({
                "llm_model_used": result.model_used,
                "tokens_used": result.tokens_used,
                "chunks_used": result.chunks_used,
            }),
            "data": {
                "document_type": req.document_type,
                "content": result.content,
                "status": "done",
                "title_safe": result.title_safe,
                "answers_anonymized": result.answers_anonymized,
                "anonymizer_removed_count": result.anonymizer_removed_count,
                "retrieval": {
                    "chunks_used": result.chunks_used,
                    "syndrome": result.syndrome,
                    "diagnosis_class": result.diagnosis_class,
                    "dynamics": result.dynamics,
                },
            },
        },
    )


@app.get("/admin/audit-sample", tags=["audit"])
def audit_sample(sample: int = Query(default=20, ge=1, le=200)) -> dict:
    store = QdrantStore(settings)
    try:
        payloads = store.sample_payloads(limit=sample)
    except Exception as exc:
        logger.error("audit-sample: ошибка чтения Qdrant: %s",
                     type(exc).__name__)
        raise HTTPException(status_code=503,
                            detail="коллекция недоступна или пуста") from exc
    return {
        "collection": settings.qdrant_collection,
        "count": len(payloads),
        "chunks": payloads,
    }


# ─── POST /ingest — загрузка документа через admin (Этап 10) ────────────────

@app.post("/ingest", tags=["ingest"])
async def ingest(file: UploadFile = File(...)) -> JSONResponse:
    """Принять файл (.docx/.odt/.doc) и загрузить в RAG.

    Пайплайн: extract → anonymize(gateway) → chunk → embed → upsert.
    Оригинал НЕ хранится (приватность, docs/09).
    """
    from app.anonymizer_client import AnonymizerClient
    from app.chunking import chunk_document
    from app.embeddings import Embedder
    from app.extractors import SUPPORTED_TEXT_SUFFIXES, ExtractionError, extract_text
    from app.qdrant_store import PointRecord, build_payload, make_point_id

    # Валидация расширения.
    if not file.filename:
        return _error_response(400, "BAD_REQUEST", "имя файла отсутствует")

    suffix = Path(file.filename).suffix.lower()
    if suffix not in SUPPORTED_TEXT_SUFFIXES:
        return _error_response(400, "BAD_REQUEST",
                               f"неподдерживаемый формат {suffix} "
                               f"(допустимы: {', '.join(sorted(SUPPORTED_TEXT_SUFFIXES))})")

    # Прочитать содержимое (максимум ~20 МБ).
    contents = await file.read()
    if len(contents) > 20 * 1024 * 1024:
        return _error_response(400, "BAD_REQUEST", "файл слишком большой (макс. 20 МБ)")
    if len(contents) == 0:
        return _error_response(400, "BAD_REQUEST", "пустой файл")

    # Сохранить во временный файл для extractors.
    with tempfile.NamedTemporaryFile(suffix=suffix, delete=True) as tmp:
        tmp.write(contents)
        tmp.flush()
        tmp_path = Path(tmp.name)

        # 1. Извлечение текста.
        try:
            raw_text = extract_text(tmp_path)
        except ExtractionError as exc:
            logger.warning("ingest: extraction failed: %s", exc)
            return _error_response(422, "EXTRACTION_ERROR",
                                   f"не удалось извлечь текст: {exc}")
        except Exception as exc:
            logger.error("ingest: unexpected extraction error: %s",
                         type(exc).__name__)
            return _error_response(500, "INTERNAL_ERROR", "ошибка извлечения текста")

    if not raw_text or not raw_text.strip():
        return _error_response(422, "EMPTY_DOCUMENT", "документ пуст после извлечения текста")

    # 2. Анонимизация через gateway (fail-closed).
    anon = AnonymizerClient(settings)
    try:
        result = anon.anonymize(raw_text)
    finally:
        anon.close()

    del raw_text  # сырой текст больше не нужен

    if not result.passed:
        return _error_response(422, "PII_DETECTED",
                               "документ не удалось безопасно обезличить")

    # 3. Чанкинг обезличенного текста.
    chunks = chunk_document(result.content, settings)
    if not chunks:
        return _error_response(422, "NO_CHUNKS",
                               "нет валидных чанков после обезличивания")

    # 4. Эмбеддинги.
    embedder = Embedder(settings)
    store = QdrantStore(settings)
    dim = embedder.dimension
    store.ensure_collection(dim)
    vectors = embedder.embed_passages([c.text for c in chunks])

    # 5. Upsert в Qdrant (source="user_upload" для отличия от корпуса).
    points: list[PointRecord] = []
    qdrant_ids: list[str] = []
    for chunk, vector in zip(chunks, vectors):
        payload = build_payload(
            chunk.text,
            doc_type=chunk.doc_type,
            section=chunk.section,
            syndrome=chunk.syndrome,
            diagnosis_class=chunk.diagnosis_class,
            dynamics=chunk.dynamics,
            source="user_upload",
        )
        pid = make_point_id(chunk.text, payload)
        points.append(PointRecord(
            point_id=pid, vector=vector, payload=payload))
        qdrant_ids.append(pid)

    written = store.upsert(points)

    removed_by_type = {}
    for k, v in result.removed_by_type.items():
        removed_by_type[k] = v

    return JSONResponse(
        status_code=200,
        content={
            "meta": _meta(),
            "data": {
                "status": "ingested",
                "chunks_count": written,
                "qdrant_ids": qdrant_ids,
                "anonymizer_removed_count": result.removed_count,
                "removed_by_type": removed_by_type,
            },
        },
    )
