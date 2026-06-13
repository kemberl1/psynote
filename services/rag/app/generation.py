"""Prompt building for diary generation (заглушка).

Сборка промпта (docs/03_rag_design.md §8): шаблон (жёсткий каркас) +
RAG-образцы (стиль) + ответы опросника (содержание). Низкая температура,
anti-hallucination, плейсхолдеры вместо ПДн.

Примечание о разделении ролей: сам вызов LLM с fallback выполняет
Go-Gateway (docs/02 §4, docs/03 §10). Здесь — только подготовка контекста/
промпта. llm_client.py оставлен как «План Б» (docs/03 §9 — Python-вариант).

Этап 1 (каркас): только сигнатуры. Реализация — Этапы 2–3 роадмапа.
"""

from __future__ import annotations


def build_prompt(answers: dict, samples: list[dict], doc_type: str) -> str:
    """Собрать промпт из шаблона + образцов + ответов. TODO(этап 3)."""
    raise NotImplementedError("generation.build_prompt — этап 3 (docs/03 §8)")
