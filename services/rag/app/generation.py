"""Промпт-инжиниринг для генерации дневника (docs/03 §8).

Сборка промпта из четырёх частей (docs/03 §8):
  (а) SYSTEM — роль врача-психиатра, anti-hallucination, плейсхолдеры вместо ПДн;
  (б) CONTEXT: ШАБЛОН — жёсткий каркас разделов нужного типа (templates.py);
  (в) CONTEXT: ОБРАЗЦЫ — few-shot обезличенные чанки из корпуса (retrieval);
  (г) USER: ОТВЕТЫ ОПРОСНИКА — клинические формулировки (questionnaire.py).

Промпты РАЗДЕЛены для daily и exam_10d (разные шаблоны/инструкции).
Низкая температура и anti-hallucination задаются на уровне вызова LLM (docs/03 §8).

ПРИВАТНОСТЬ: на вход подаются уже обезличенные ответы и обезличенные образцы
из Qdrant. Модель инструктируется НЕ выдумывать ФИО/даты, использовать
плейсхолдеры.
"""

from __future__ import annotations

from app.llm_client import LLMMessage
from app.questionnaire import MappedAnswers
from app.templates import (
    DOC_TYPE_DAILY,
    PLACEHOLDER_DATE,
    PLACEHOLDER_DOCTOR,
    DiaryTemplate,
    get_template,
)

# Системная инструкция — общая основа (docs/03 §8 [SYSTEM]).
_SYSTEM_BASE = (
    "Ты — ассистент детского врача-психиатра. Ты составляешь медицинские "
    "дневники наблюдения строго в стиле и терминологии отделения. "
    "ЖЁСТКИЕ ПРАВИЛА:\n"
    "1. Не выдумывай факты. Используй ТОЛЬКО данные из ответов опросника и "
    "стилистику предоставленных образцов.\n"
    "2. Соблюдай структуру шаблона: все разделы в указанном порядке, с теми же "
    "названиями разделов.\n"
    "3. Пиши на русском языке. Сохраняй коды МКБ-10 и латинские названия "
    "препаратов без изменений.\n"
    f"4. НИКОГДА не вставляй реальные ФИО, даты рождения, адреса, номера историй "
    f"болезни. Для даты используй плейсхолдер {PLACEHOLDER_DATE}, для подписи "
    f"врача — {PLACEHOLDER_DOCTOR}.\n"
    "5. Для разделов, по которым нет данных в опроснике, используй стандартные "
    "«нормальные» формулировки отделения из образцов (напр., соматический и "
    "неврологический статус), не добавляя вымышленных деталей.\n"
    "6. Не копируй образцы дословно — варьируй формулировки, сохраняя стиль."
)

_SYSTEM_DAILY = (
    _SYSTEM_BASE +
    "\n\nТип документа: ЕЖЕДНЕВНЫЙ дневник (осмотр лечащим врачом). "
    "Это краткая фиксация состояния и динамики за день."
)

_SYSTEM_EXAM_10D = (
    _SYSTEM_BASE +
    "\n\nТип документа: РАСШИРЕННЫЙ ОСМОТР раз в 10 дней (лечащий "
    "врач совместно с заведующим отделением). Обязательно заполни развёрнутый "
    "психический статус и ЭТАПНЫЙ ЭПИКРИЗ (резюме динамики за период)."
)


def _system_prompt(doc_type: str) -> str:
    return _SYSTEM_DAILY if doc_type == DOC_TYPE_DAILY else _SYSTEM_EXAM_10D


def _format_samples(samples: list[dict]) -> str:
    """Few-shot блок из обезличенных образцов корпуса (docs/03 §8 [CONTEXT])."""
    if not samples:
        return ("(образцы из корпуса отсутствуют — опирайся только на шаблон и "
                "ответы опросника)")
    lines: list[str] = []
    for i, s in enumerate(samples, 1):
        text = (s.get("text") or "").strip()
        if not text:
            continue
        meta_bits = []
        for key in ("syndrome", "diagnosis_class", "dynamics"):
            if s.get(key):
                meta_bits.append(f"{key}={s[key]}")
        meta = f" ({', '.join(meta_bits)})" if meta_bits else ""
        lines.append(f"--- Образец {i}{meta} ---\n{text}")
    return "\n\n".join(lines) if lines else "(образцы пусты)"


def build_messages(doc_type: str, mapped: MappedAnswers,
                   samples: list[dict]) -> list[LLMMessage]:
    """Собрать список сообщений чата для LLM (docs/03 §8).

    Разделение ролей:
      • system — инструкция + жёсткий каркас шаблона;
      • user   — few-shot образцы + ответы опросника + финальная инструкция.
    """
    template: DiaryTemplate = get_template(doc_type)

    system_content = (
        _system_prompt(doc_type)
        + "\n\n[СТРУКТУРА ШАБЛОНА — обязательный каркас и порядок разделов]\n"
        + template.render_skeleton()
    )

    answers_block = "\n".join(f"- {line}" for line in mapped.prompt_lines) \
        or "- (ответы не предоставлены)"

    user_content = (
        "[ОБРАЗЦЫ ИЗ КОРПУСА — обезличенные, задают стиль и наполнение разделов]\n"
        + _format_samples(samples)
        + "\n\n[ОТВЕТЫ ОПРОСНИКА — индивидуальное содержание по пациенту]\n"
        + answers_block
        + "\n\n[ЗАДАНИЕ]\nСгенерируй готовый "
        + ("ежедневный дневник" if doc_type == DOC_TYPE_DAILY
           else "расширенный осмотр (раз в 10 дней) с этапным эпикризом")
        + " строго по структуре шаблона, в стиле образцов, на основе ответов "
        "опросника. Заполни все разделы. Реальные ФИО и даты не подставляй — "
        f"используй плейсхолдеры {PLACEHOLDER_DATE} и {PLACEHOLDER_DOCTOR}."
    )

    return [
        LLMMessage(role="system", content=system_content),
        LLMMessage(role="user", content=user_content),
    ]


def build_query_text(mapped: MappedAnswers, doc_type: str) -> str:
    """Поисковый текст для retrieval (docs/03 §6 п.1) из формулировок ответов.

    Если ответов мало, добавляем тип документа как опорный контекст.
    """
    base = mapped.query_text.strip()
    type_hint = ("ежедневный дневник психиатрический"
                 if doc_type == DOC_TYPE_DAILY
                 else "расширенный психиатрический осмотр этапный эпикриз")
    if mapped.syndrome:
        base = f"{mapped.syndrome} {base}"
    return f"{type_hint} {base}".strip()
