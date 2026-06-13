"""Vector retrieval from Qdrant (docs/03 §6).

Этап 3 (ingestion): здесь — переиспользуемая основа клиента/фильтров, которой
воспользуется Этап 4 (генерация). Полная retrieval-для-генерации логика (сборка
few-shot контекста) — Этап 4; здесь реализован базовый фильтрованный поиск,
опирающийся на общие модули QdrantStore/Embedder.

Retrieval-стратегия (docs/03 §6): эмбеддинг запроса (query:) → поиск top-k в
Qdrant С ФИЛЬТРОМ по метаданным (doc_type / syndrome / diagnosis_class / section)
→ обезличенные few-shot образцы.
"""

from __future__ import annotations

from qdrant_client.http import models as qmodels

from app.config import get_settings
from app.embeddings import Embedder
from app.qdrant_store import QdrantStore


def _build_filter(doc_type: str | None, syndrome: str | None,
                  diagnosis_class: str | None, section: str | None):
    must: list = []
    for field_name, value in (
        ("doc_type", doc_type),
        ("syndrome", syndrome),
        ("diagnosis_class", diagnosis_class),
        ("section", section),
    ):
        if value:
            must.append(qmodels.FieldCondition(
                key=field_name, match=qmodels.MatchValue(value=value)))
    return qmodels.Filter(must=must) if must else None


def retrieve(query: str, doc_type: str | None = None, top_k: int = 5,
             *, syndrome: str | None = None, diagnosis_class: str | None = None,
             section: str | None = None) -> list[dict]:
    """Найти top-k обезличенных образцов нужного типа/регистра (docs/03 §6)."""
    settings = get_settings()
    embedder = Embedder(settings)
    store = QdrantStore(settings)

    query_vector = embedder.embed_query(query)
    hits = store.client.search(
        collection_name=store.collection,
        query_vector=query_vector,
        query_filter=_build_filter(
            doc_type, syndrome, diagnosis_class, section),
        limit=top_k,
        with_payload=True,
    )
    return [
        {"score": h.score, **(dict(h.payload) if h.payload else {})}
        for h in hits
    ]
