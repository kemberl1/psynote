"""FastAPI application entrypoint for the RAG service.

Этап 3 (ingestion): /health + /admin/audit-sample (выгрузка выборки обезличенных
чанков для ручного аудита приватности, docs/04 §7). Сам ingestion запускается
CLI-командой `python -m app.ingest ingest` (one-shot, docs/03 §5).

Этап 4 (генерация): POST /generate — главный эндпоинт генерации дневника
(анонимизация-гейт → RAG-retrieval → промпт → LLM X5 с фолбэком), docs/03,
docs/07 §5.

ОТКЛОНЕНИЕ ОТ docs/07: в контракте /api/v1/generate висит на Go-Gateway, который
проксирует на RAG. По заданию Этапа 4 конвейер генерации (LLM+fallback+промпты)
реализован в RAG-сервисе и публикуется как POST /generate. Gateway на следующих
этапах будет проксировать /api/v1/generate → rag:8000/generate. Конверт ответа
`{meta, data}` / `{meta, error}` и коды (422 PII, 503 LLM) сохранены по docs/07 §1.
"""

from __future__ import annotations

import logging
import uuid
from datetime import datetime, timezone

from fastapi import FastAPI, HTTPException, Query
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
    version="0.4.0",
    description=(
        "RAG-сервис: ingestion корпуса (CLI), retrieval из Qdrant, генерация "
        "психиатрических дневников (LLM X5 с фолбэком). См. docs/03_rag_design.md."
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
    """Тело запроса генерации (docs/07 §5)."""

    document_type: str = Field(..., description="daily | exam_10d")
    answers: dict = Field(default_factory=dict,
                          description="Ответы опросника (docs/06 §3)")
    options: dict | None = Field(
        default=None, description="опции (stream и пр.)")


@app.get("/health", tags=["health"])
def health() -> dict:
    """Liveness probe + доступность LLM-конфига (docs/07 §8)."""
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
    """Сгенерировать дневник: гейт → retrieval → промпт → LLM (docs/03, docs/07 §5).

    Коды (docs/07 §1):
      • 200 — успех, data.content — обезличенный дневник;
      • 400 — неизвестный document_type;
      • 422 — PII-гейт заблокировал свободный ввод (PII_DETECTED);
      • 503 — все модели LLM недоступны / LLM не сконфигурирован.
    """
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
        # Auth (401/403) — конфиг-проблема ключа; все модели недоступны — 503.
        if isinstance(exc, LLMAuthError):
            logger.error("generate: ошибка авторизации LLM")
            return _error_response(
                503, "LLM_AUTH_ERROR",
                "Ошибка авторизации LLM (проверьте X5_API_KEY)")
        logger.error("generate: LLM недоступен (%s)", type(exc).__name__)
        return _error_response(503, "LLM_UNAVAILABLE",
                               "Сервис генерации временно недоступен")
    except Exception as exc:  # noqa: BLE001
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
