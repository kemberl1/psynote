"""Vector retrieval from Qdrant (docs/03 §6).

Retrieval-стратегия (docs/03 §6): эмбеддинг запроса (query:) → поиск top-k в
Qdrant С ФИЛЬТРОМ по метаданным (doc_type / syndrome / diagnosis_class / section)
→ обезличенные few-shot образцы.

ЭТАП 4.1 — ступенчатый фолбэк фильтров (graceful degradation).
Узкая выборка корпуса приводила к тому, что строгий фильтр
(doc_type + syndrome + diagnosis_class) для exam_10d мог вернуть 0 образцов →
    генерация шла БЕЗ few-shot (теряется суть RAG). Фильтры ПОСЛЕДОВАТЕЛЬНО
ослабляются, пока не найдутся образцы — но diagnosis_class, если он задан,
не снимается: иначе депрессия/F3x получит few-shot от УО/F7x и модель
скопирует чужой регистр.

    L0 strict   : doc_type + syndrome + diagnosis_class (+section, если задан)
    L1 diagnosis: doc_type + diagnosis_class
    L2 doc_type : doc_type          — только если diagnosis_class НЕ задан
    L3 none     : без фильтров      — только если diagnosis_class НЕ задан

Если класс МКБ известен, а в корпусе нет совпадений — лучше 0 few-shot,
чем чужой клинический регистр. Уровень и число образцов ЛОГИРУЮТСЯ
(без ПДн — только метаданные/счётчики).
"""

from __future__ import annotations

import logging
from dataclasses import dataclass

from qdrant_client.http import models as qmodels

from app.config import get_settings
from app.embeddings import Embedder
from app.questionnaire import normalize_syndrome
from app.qdrant_store import QdrantStore

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class _Level:
    """Один уровень фолбэка: человекочитаемое имя + активные поля фильтра."""

    name: str
    doc_type: str | None
    syndrome: str | None
    diagnosis_class: str | None
    section: str | None


def _build_filter(level: _Level):
    must: list = []
    for field_name, value in (
        ("doc_type", level.doc_type),
        ("syndrome", level.syndrome),
        ("diagnosis_class", level.diagnosis_class),
        ("section", level.section),
    ):
        if value:
            must.append(qmodels.FieldCondition(
                key=field_name, match=qmodels.MatchValue(value=value)))
    return qmodels.Filter(must=must) if must else None


def _fallback_levels(doc_type: str | None, syndrome: str | None,
                     diagnosis_class: str | None,
                     section: str | None) -> list[_Level]:
    """Ступени ослабления фильтра (от строгой к пустой), без дублей.

    Уровни, не добавляющие НИ ОДНОГО доп. условия относительно уже виденного
    набора полей, схлопываются — чтобы не делать одинаковые запросы дважды.
    """
    candidates = [
        _Level("L0_strict", doc_type, syndrome, diagnosis_class, section),
        _Level("L1_diagnosis", doc_type, None, diagnosis_class, None),
    ]
    # Не снимаем diagnosis_class: L2/L3 подмешивают чужой МКБ-регистр.
    if not diagnosis_class:
        candidates.extend([
            _Level("L2_doc_type", doc_type, None, None, None),
            _Level("L3_none", None, None, None, None),
        ])
    levels: list[_Level] = []
    seen_keys: set[tuple] = set()
    for lvl in candidates:
        key = (lvl.doc_type, lvl.syndrome, lvl.diagnosis_class, lvl.section)
        if key in seen_keys:
            continue
        seen_keys.add(key)
        levels.append(lvl)
    return levels


def search_with_fallback(client, collection: str, query_vector, top_k: int,
                         levels: list[_Level]) -> list[dict]:
    """Прогнать уровни фолбэка по `client`, вернуть первый НЕПУСТОЙ результат.

    Выделено отдельно (без get_settings/Embedder/QdrantStore) для юнит-тестов:
    можно передать ФЕЙКОВЫЙ qdrant-client без сети/torch (Этап 4.1).
    """
    for lvl in levels:
        hits = client.search(
            collection_name=collection,
            query_vector=query_vector,
            query_filter=_build_filter(lvl),
            limit=top_k,
            with_payload=True,
        )
        if hits:
            logger.info(
                "retrieve: фолбэк-уровень=%s дал образцов=%d (doc_type=%s, "
                "syndrome=%s, diagnosis_class=%s, section=%s)",
                lvl.name, len(hits), lvl.doc_type, lvl.syndrome,
                lvl.diagnosis_class, lvl.section)
            return [
                {"score": h.score, **(dict(h.payload) if h.payload else {})}
                for h in hits
            ]

    logger.warning(
        "retrieve: образцы НЕ найдены ни на одном уровне фолбэка — коллекция '%s' "
        "пуста? Генерация пойдёт без few-shot.", collection)
    return []


def retrieve(query: str, doc_type: str | None = None, top_k: int = 5,
             *, syndrome: str | None = None, diagnosis_class: str | None = None,
             section: str | None = None) -> list[dict]:
    """Найти top-k обезличенных образцов со ступенчатым ослаблением фильтров.

    docs/03 §6 + Этап 4.1. Возвращает первый НЕПУСТОЙ результат по уровням
    L0→L1 (если задан diagnosis_class) или L0→L3 (если класс неизвестен).
    Нет совпадений / пустая коллекция — [].
    """
    settings = get_settings()
    embedder = Embedder(settings)
    store = QdrantStore(settings)

    # Техдолг §1: нормализуем syndrome в канонич. форму перед фильтрацией
    # (payload в Qdrant тоже хранит канонич. форму — совпадение гарантировано).
    canon_syndrome = normalize_syndrome(syndrome)
    query_vector = embedder.embed_query(query)
    levels = _fallback_levels(doc_type, canon_syndrome,
                              diagnosis_class, section)
    return search_with_fallback(
        store.client, store.collection, query_vector, top_k, levels)
