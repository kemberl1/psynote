"""Local embedding model wrapper (заглушка).

Эмбеддинги считаются ЛОКАЛЬНО (docs/03_rag_design.md §3,
multilingual-e5-large) — медицинский текст не уходит в облако.

Этап 1 (каркас): только сигнатуры. Загрузка модели через
sentence-transformers и сам инференс — Этап 2 роадмапа.

NB: тяжёлые ML-зависимости (torch, sentence-transformers) намеренно
НЕ включены в requirements.txt на каркасе ради быстрой сборки образа —
они помечены как future в requirements.txt.
"""

from __future__ import annotations


def embed_texts(texts: list[str]) -> list[list[float]]:
    """Вернуть эмбеддинги для списка текстов. TODO(этап 2)."""
    raise NotImplementedError("embeddings.embed_texts — этап 2 (docs/03 §3)")
