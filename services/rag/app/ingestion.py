"""Corpus ingestion pipeline (заглушка).

Pipeline (docs/03_rag_design.md §5): извлечение текста (.docx/.odt) →
анонимизация (Go-гейт) → валидация → структурный чанкинг + метаданные →
локальные эмбеддинги → upsert в Qdrant.

ВАЖНО: в Qdrant попадает ТОЛЬКО обезличенный текст (docs/03 §4).
Реальный корпус лежит в «Документы/» и НЕ коммитится в git.

Этап 1 (каркас): только сигнатуры. Реализация — Этап 2 роадмапа.
"""

from __future__ import annotations


def extract_text(path: str) -> str:
    """Извлечь сырой текст из .docx/.odt. TODO(этап 2)."""
    raise NotImplementedError("ingestion.extract_text — этап 2 (docs/03 §5)")


def chunk_document(text: str) -> list[dict]:
    """Структурный чанкинг + метаданные (doc_type/section/...). TODO(этап 2)."""
    raise NotImplementedError("ingestion.chunk_document — этап 2 (docs/03 §4)")
