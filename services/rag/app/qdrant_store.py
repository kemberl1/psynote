"""Клиент Qdrant: создание коллекции, идемпотентный upsert, выборка для аудита.

docs/05 §3: коллекция `corpus_diaries`, вектор e5 (cosine). В payload — ТОЛЬКО
обезличенный текст и метаданные (doc_type/section/syndrome/diagnosis_class/
dynamics/source/ingested_at). НИКАКИХ ФИО, дат, имён файлов/путей (docs/03 §4).

Идемпотентность (docs/05 §5): id точки детерминирован — UUIDv5 от
(обезличенный_текст + метаданные). Повторный запуск ingestion не плодит дубликаты:
тот же контент → тот же id → upsert перезаписывает, а не добавляет.

Этот модуль переиспользуется Этапом 4 (retrieval/генерация).
"""

from __future__ import annotations

import hashlib
import logging
import uuid
from dataclasses import dataclass
from datetime import datetime, timezone

from qdrant_client import QdrantClient
from qdrant_client.http import models as qmodels

from app.config import Settings

logger = logging.getLogger(__name__)

# Пространство имён для детерминированных UUIDv5 (стабильно между запусками).
_ID_NAMESPACE = uuid.UUID("6f1c2d3e-4a5b-6c7d-8e9f-001122334455")


@dataclass
class PointRecord:
    """Готовая к upsert точка: id + вектор + обезличенный payload."""

    point_id: str
    vector: list[float]
    payload: dict


def make_point_id(text: str, payload: dict) -> str:
    """Детерминированный id чанка (docs/05 §5): UUIDv5 от текста + ключевых метаданных."""
    key_parts = [
        text,
        str(payload.get("doc_type", "")),
        str(payload.get("section", "")),
        str(payload.get("source", "")),
    ]
    digest = hashlib.sha256("\u0001".join(
        key_parts).encode("utf-8")).hexdigest()
    return str(uuid.uuid5(_ID_NAMESPACE, digest))


class QdrantStore:
    """Обёртка над qdrant-client для ingestion и (будущего) retrieval."""

    def __init__(self, settings: Settings, client: QdrantClient | None = None) -> None:
        self._settings = settings
        self._collection = settings.qdrant_collection
        self._client = client or QdrantClient(url=settings.qdrant_url)

    @property
    def client(self) -> QdrantClient:
        return self._client

    @property
    def collection(self) -> str:
        return self._collection

    def ensure_collection(self, vector_size: int) -> None:
        """Идемпотентно создать коллекцию с нужной размерностью и cosine (docs/05 §3)."""
        if self._client.collection_exists(self._collection):
            logger.info("Коллекция '%s' уже существует — переиспользуем.",
                        self._collection)
            return
        logger.info("Создаю коллекцию '%s' (dim=%d, distance=Cosine).",
                    self._collection, vector_size)
        self._client.create_collection(
            collection_name=self._collection,
            vectors_config=qmodels.VectorParams(
                size=vector_size,
                distance=qmodels.Distance.COSINE,
            ),
        )
        # Индексы по метаданным для фильтрованного retrieval (docs/03 §6).
        # doc_kind — задел под будущие типы документов (Этап 4.1, docs/03 §11).
        for field_name in ("doc_type", "doc_kind", "section", "syndrome",
                           "diagnosis_class", "dynamics", "source"):
            try:
                self._client.create_payload_index(
                    collection_name=self._collection,
                    field_name=field_name,
                    field_schema=qmodels.PayloadSchemaType.KEYWORD,
                )
            except Exception as exc:  # индекс не критичен для записи
                logger.debug("payload-индекс %s не создан: %s", field_name,
                             type(exc).__name__)

    def upsert(self, points: list[PointRecord]) -> int:
        """Идемпотентный upsert батча точек. Возвращает число записанных точек."""
        if not points:
            return 0
        self._client.upsert(
            collection_name=self._collection,
            points=[
                qmodels.PointStruct(
                    id=p.point_id, vector=p.vector, payload=p.payload)
                for p in points
            ],
        )
        return len(points)

    def count(self) -> int:
        """Количество точек в коллекции (для итогового лога/аудита)."""
        return self._client.count(self._collection, exact=True).count

    def sample_payloads(self, limit: int = 20) -> list[dict]:
        """Выгрузить выборку payload для аудита приватности (docs/04 §7, приёмка).

        Возвращает обезличенные payload (text + метаданные). Используется командой
        `python -m app.ingest audit` для ручной проверки отсутствия ПДн.
        """
        points, _ = self._client.scroll(
            collection_name=self._collection,
            limit=max(1, limit),
            with_payload=True,
            with_vectors=False,
        )
        return [dict(p.payload or {}) for p in points]


def build_payload(text: str, *, doc_type: str, section: str,
                  syndrome: str | None, diagnosis_class: str | None,
                  dynamics: str | None, source: str = "corpus",
                  doc_kind: str = "diary") -> dict:
    """Сформировать payload точки (docs/05 §3.2). Только обезличенные данные.

    `doc_kind` — КАТЕГОРИЯ источника (Этап 4.1, точка расширения под docs/03 §11):
    сейчас индексируем только дневники → "diary". В будущем сюда добавятся
    "primary_exam"/"epicrisis"/"clinical_guideline" (вероятно в ОТДЕЛЬНЫХ
    коллекциях, см. docs/03 §11) — поле уже в payload, чтобы фильтровать/мигрировать
    без переиндексации схемы. doc_type остаётся клиническим подтипом (daily|exam_10d).
    """
    payload: dict = {
        "text": text,
        "doc_type": doc_type,
        "doc_kind": doc_kind,
        "section": section,
        "source": source,
        "ingested_at": datetime.now(timezone.utc).isoformat(),
    }
    if syndrome:
        payload["syndrome"] = syndrome
    if diagnosis_class:
        payload["diagnosis_class"] = diagnosis_class
    if dynamics:
        payload["dynamics"] = dynamics
    return payload
