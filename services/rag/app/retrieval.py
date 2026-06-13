"""Vector retrieval from Qdrant (заглушка).

Retrieval-стратегия (docs/03_rag_design.md §6): эмбеддинг запроса →
поиск top-k в Qdrant С ФИЛЬТРОМ по метаданным (doc_type / syndrome /
diagnosis_class / section) → обезличенные few-shot образцы.

Этап 1 (каркас): только сигнатуры. Реализация — Этап 2 роадмапа.
"""

from __future__ import annotations


def retrieve(query: str, doc_type: str, top_k: int = 5) -> list[dict]:
    """Найти top-k обезличенных образцов нужного типа/регистра. TODO(этап 2)."""
    raise NotImplementedError("retrieval.retrieve — этап 2 (docs/03 §6)")
