"""OpenAI-compatible X5 CoPilot client — «План Б» на Python (заглушка).

Основной LLM-клиент с fallback реализуется в Go-Gateway (docs/02 §4,
docs/03 §10). Этот модуль — резервный Python-вариант по паттерну
референс-проекта hh_analyser (docs/03 §9): base_url, Bearer-ключ,
корпоративный CA-bundle (LLM_CA_BUNDLE), тайм-ауты/ретраи, fallback
large→medium→small.

Этап 1 (каркас): только сигнатура. Реализация — при необходимости на Этапе 3.
"""

from __future__ import annotations


def chat(prompt: str, model: str, temperature: float = 0.4) -> str:
    """Выполнить chat-completion к X5 CoPilot. TODO(этап 3, опц.)."""
    raise NotImplementedError("llm_client.chat — этап 3 / План Б (docs/03 §9)")
