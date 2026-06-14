"""Маппинг ответов опросника → клинические формулировки промпта (docs/06 §6).

Вход — структура ответов по docs/07 §5 (`answers`): {question_id: value}, где
значение бывает:
  • строкой (select): "lowered";
  • списком (multiselect): ["anxiety", "tearfulness"];
  • кастомным объектом: {"value": "__custom__", "custom_text": "..."} —
    свободный текст врача (docs/06 §1.4), потенциально содержит ПДн.

Каждый ответ несёт «клиническую формулировку» (`prompt`-строка, docs/06 §6).
Здесь зашита таблица соответствий из docs/06 §4–5 (опции → формулировки).
Кастомный свободный текст НЕ маппится по словарю — он помечается как
free-text и ОБЯЗАН пройти анонимайзер-гейт перед попаданием в промпт (docs/04).

Модуль также:
  • извлекает метаданные для фильтрованного retrieval (syndrome,
    diagnosis_class) — docs/03 §6;
  • строит поисковый текст запроса (query) из формулировок — docs/03 §6 п.1;
  • формирует безопасный заголовок истории (title_safe) — docs/05 §2.2.

ПРИВАТНОСТЬ: сюда приходит уже-анонимизированный свободный текст (оркестратор
прогнал его через гейт ДО маппинга). Сырой ввод тут не логируется.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field

from app.templates import DOC_TYPE_DAILY, DOC_TYPE_EXAM_10D

CUSTOM_SENTINEL = "__custom__"


# ─── Таблицы маппинга опций → клинические формулировки (docs/06 §4–5) ────────
# Структура: {question_id: {"label": <человеческий>, "options": {value: prompt}}}.
# Для multiselect формулировки соединяются. Дефолты соответствуют docs/06.

_DAILY_QUESTIONS: dict[str, dict] = {
    "dynamics": {
        "label": "Динамика состояния",
        "options": {
            "no_change": "Динамика состояния: без существенных изменений.",
            "positive": "Состояние с положительной динамикой.",
            "slight_improvement": "Состояние с незначительным улучшением.",
            "worsening": "Состояние с ухудшением.",
            "stable_positive": "Состояние со стойкой положительной динамикой.",
        },
    },
    "productive_symptoms": {
        "label": "Психопродуктивная симптоматика",
        "options": {
            "not_detected": "Психопродуктивная симптоматика не выявлена.",
            "detected": "Выявлена психопродуктивная симптоматика.",
        },
    },
    "productive_symptoms_detail": {
        "label": "Характер психопродуктивной симптоматики",
        "multi": True,
        "options": {
            "hallucinatory": "галлюцинаторная симптоматика",
            "delusional": "бредовая симптоматика",
            "illusory": "иллюзорные расстройства",
            "obsessive": "навязчивые расстройства",
        },
        "multi_prefix": "Характер психопродуктивной симптоматики: ",
    },
    "mood": {
        "label": "Фон настроения",
        "options": {
            "even": "Фон настроения ровный, без снижения.",
            "lowered": "Настроение снижено.",
            "unstable": "Фон настроения неустойчивый.",
            "dysphoric": "Фон настроения с оттенком дисфории.",
            "elevated": "Настроение ситуационно повышено.",
        },
    },
    "mood_detail": {
        "label": "Детали настроения",
        "multi": True,
        "options": {
            "tearfulness": "плаксивость",
            "anxiety": "тревога",
            "irritability": "раздражительность",
            "melancholy": "тоскливость",
            "lability": "эмоциональная лабильность",
        },
        "multi_prefix": "Отмечаются: ",
    },
    "behavior": {
        "label": "Поведение и режим",
        "options": {
            "ordered": "Поведение упорядоченное, режим соблюдает.",
            "minor_remarks": "Режим соблюдает на негрубых замечаниях.",
            "violates": "Нарушает режим отделения.",
            "restless": "Двигательно беспокоен.",
        },
    },
    "behavior_detail": {
        "label": "Характер нарушений режима",
        "multi": True,
        "options": {
            "conflict": "конфликтность",
            "protest": "протестные реакции",
            "refusal": "отказ от соблюдения режима",
            "aggression": "агрессивные проявления",
        },
        "multi_prefix": "Отмечаются: ",
    },
    "contact": {
        "label": "Общение и контакт",
        "multi": True,
        "options": {
            "productive": "доступен продуктивному контакту",
            "selective_children": "общается с детьми избирательно",
            "isolated": "держится обособленно",
            "polite_staff": "с персоналом вежлив",
            "negativistic": "негативистичен",
        },
        "multi_prefix": "Контакт: ",
    },
    "sleep": {
        "label": "Сон",
        "options": {
            "not_disturbed": "Сон не нарушен.",
            "hard_to_fall_asleep": "Сон с трудностями засыпания.",
            "superficial": "Сон поверхностный.",
            "sufficient": "Сон достаточный.",
        },
    },
    "sleep_detail": {
        "label": "Характер нарушения сна",
        "multi": True,
        "options": {
            "hard_to_fall_asleep": "трудности засыпания",
            "frequent_awakenings": "частые пробуждения",
            "superficial": "поверхностный сон",
            "no_rest": "отсутствие чувства отдыха после сна",
        },
        "multi_prefix": "Нарушение сна: ",
    },
    "appetite": {
        "label": "Аппетит",
        "options": {
            "preserved": "Аппетит сохранён.",
            "decreased": "Аппетит снижен.",
            "selective": "Аппетит избирательный.",
            "increased": "Аппетит повышен.",
        },
    },
    "tolerance": {
        "label": "Переносимость терапии",
        "options": {
            "good": "Терапию переносит хорошо.",
            "satisfactory": "Терапию переносит удовлетворительно.",
            "adverse": "Отмечаются нежелательные явления на фоне терапии.",
            "none": "Терапию не получает.",
        },
    },
    "complaints": {
        "label": "Жалобы",
        "options": {
            "none": "Жалоб активно не предъявляет.",
            "cannot_formulate": "Жалобы самостоятельно не формирует.",
            "present": "Предъявляет жалобы.",
        },
    },
    "events": {
        "label": "События дня",
        "multi": True,
        "options": {
            "consultation": "проведена консультация специалиста",
            "therapy_correction": "проведена коррекция терапии",
            "somatic": "отмечается сопутствующее соматическое заболевание",
            "examination": "выполнено обследование",
        },
        "multi_prefix": "События дня: ",
    },
}

# Условные/уточняющие вопросы со свободным вводом (docs/06 §4.2).
# Это поля типа text (и detail-уточнения), значения которых — свободный текст
# врача; они проходят анонимайзер-гейт ДО маппинга (docs/04). Multiselect-поля
# с allow_custom (mood_detail, sleep_detail, …) обрабатываются как select-блок
# выше, а их кастом-элементы извлекаются iter_free_text напрямую.
_DAILY_FREETEXT_QUESTIONS: dict[str, str] = {
    "dynamics_detail": "В чём проявляется ухудшение",
    "adverse_detail": "Нежелательные явления",
    "complaints_detail": "Жалобы",
    "events_detail": "Детали событий дня",
}

# Расширенный осмотр (docs/06 §5). Базовый блок наследуется от daily.
_EXAM_QUESTIONS: dict[str, dict] = {
    "anamnesis_disease": {
        "label": "Анамнез заболевания (дополнения)",
        "options": {
            "no_additions": "Анамнез заболевания: без дополнений.",
            "present": "Имеются дополнения к анамнезу заболевания.",
        },
    },
    "physical_status": {
        "label": "Физикальный статус",
        "options": {
            "unremarkable": ("Физикальное исследование: состояние "
                             "удовлетворительное, без особенностей."),
            "changes": "В физикальном статусе отмечаются изменения.",
        },
    },
    "neuro_status": {
        "label": "Неврологический статус",
        "options": {
            "no_acute": "Неврологический статус: без острой неврологической симптоматики.",
            "detailed_normal": ("Неврологический статус: очаговой и "
                                "менингеальной симптоматики не выявлено."),
            "changes": "В неврологическом статусе отмечаются изменения.",
        },
    },
    "criticism": {
        "label": "Критика к состоянию",
        "options": {
            "absent": "Критика к своему состоянию отсутствует.",
            "formal": "Критика к состоянию формальная.",
            "conciliatory": "Критика к состоянию соглашательская.",
            "forming": "Критика к состоянию формируется.",
            "preserved": "Критика к состоянию сохранна.",
        },
    },
    "thinking": {
        "label": "Мышление",
        "options": {
            "no_gross": "Мышление без грубых нарушений.",
            "concrete": "Мышление конкретное.",
            "detailed": "Мышление обстоятельное.",
            "slowed": "Мышление замедленное.",
        },
    },
    "attention_memory": {
        "label": "Внимание и память",
        "options": {
            "no_gross": "Внимание и память без грубых нарушений.",
            "reduced": "Внимание и память снижены.",
            "exhausted": "Внимание истощаемо.",
        },
    },
    "intellect": {
        "label": "Интеллект",
        "options": {
            "age_norm": "Интеллект на уровне возрастной нормы.",
            "low_norm": "Интеллект на уровне низкой возрастной нормы.",
            "reduced": "Интеллект снижен.",
        },
    },
    "suicidal": {
        "label": "Суицидальные тенденции",
        "options": {
            "not_detected": "Суицидальных тенденций не выявлено.",
            "detected": "Выявлены суицидальные тенденции.",
        },
    },
    "syndrome": {
        "label": "Синдром",
        "options": {
            "anxiety_depressive": "тревожно-депрессивный синдром",
            "psychopathic": "психопатоподобный синдром",
            "emotional_volitional": "синдром эмоционально-волевой неустойчивости",
            "anxious": "тревожный синдром",
            "asthenic": "астенический синдром",
        },
    },
    "comorbidities": {
        "label": "Сопутствующие заболевания",
        "multi": True,
        "options": {
            "j00": "J00 — острый назофарингит",
            "r51": "R51 — головная боль",
            "e66_9": "E66.9 — ожирение",
            "none": "Сопутствующих заболеваний не выявлено",
        },
        "multi_prefix": "Сопутствующие заболевания: ",
    },
    "prescriptions": {
        "label": "Назначения",
        "options": {
            "see_list": "Назначения — согласно листу назначений.",
            "no_change": "Назначения без изменений.",
        },
    },
    "interventions": {
        "label": "Выполненные вмешательства",
        "multi": True,
        "options": {
            "pediatrician": "осмотр педиатра",
            "neurologist": "осмотр невролога",
            "psychologist": "консультация психолога",
            "psychotherapist": "консультация психотерапевта",
            "speech_therapist": "занятие с логопедом",
            "physiotherapist": "физиотерапия",
            "ecg": "ЭКГ",
            "eeg": "ЭЭГ",
            "ultrasound": "УЗИ",
            "lab": "лабораторное обследование",
        },
        "multi_prefix": "Выполнены: ",
    },
    "period_dynamics": {
        "label": "Динамика за период (эпикриз)",
        "options": {
            "improvement": ("Психическое состояние с улучшением в условиях "
                            "отделения."),
            "slight_improvement": "Психическое состояние с незначительным улучшением.",
            "no_improvement": "Психическое состояние без заметного улучшения.",
            "no_change": "Психическое состояние без существенных изменений.",
        },
    },
}

_EXAM_FREETEXT_QUESTIONS: dict[str, str] = {
    "anamnesis_detail": "Дополнения к анамнезу",
    "physical_detail": "Физикальный статус (изменения)",
    "neuro_detail": "Неврологический статус (изменения)",
    "suicidal_detail": "Суицидальные тенденции (уточнение)",
    "diagnosis": "Основное заболевание (МКБ-10)",
    "interventions_detail": "Заключения по вмешательствам",
    "discharge_detail": "Заключение и рекомендации при выписке",
}


def _question_registry(doc_type: str) -> tuple[dict, dict]:
    """Вернуть (select-вопросы, freetext-вопросы) для типа документа."""
    if doc_type == DOC_TYPE_DAILY:
        return _DAILY_QUESTIONS, _DAILY_FREETEXT_QUESTIONS
    if doc_type == DOC_TYPE_EXAM_10D:
        # Расширенный = базовый daily + разделы осмотра (docs/06 §5).
        merged = {**_DAILY_QUESTIONS, **_EXAM_QUESTIONS}
        merged_ft = {**_DAILY_FREETEXT_QUESTIONS, **_EXAM_FREETEXT_QUESTIONS}
        return merged, merged_ft
    raise ValueError(f"Неизвестный тип документа: {doc_type}")


# ─── Извлечение МКБ-класса верхнего уровня (docs/05 §3.2 — diagnosis_class) ──
_ICD_RE = re.compile(r"\bF(\d)\d", re.IGNORECASE)


def extract_diagnosis_class(diagnosis_text: str) -> str | None:
    """Верхний уровень МКБ из текста диагноза, напр. 'F84.11' → 'F8x' (docs/05)."""
    m = _ICD_RE.search(diagnosis_text or "")
    if not m:
        return None
    return f"F{m.group(1)}x"


@dataclass
class MappedAnswers:
    """Результат маппинга ответов в промпт-структуру и метаданные."""

    # Готовые клинические строки в порядке вопросов (для блока «ОТВЕТЫ»).
    prompt_lines: list[str] = field(default_factory=list)
    # Текст для эмбеддинга поискового запроса (docs/03 §6 п.1).
    query_text: str = ""
    # Метаданные для фильтрованного retrieval (docs/03 §6).
    syndrome: str | None = None
    diagnosis_class: str | None = None
    dynamics: str | None = None
    # Безопасный заголовок истории (docs/05 §2.2) — без ПДн.
    title_safe: str = ""


def _normalize_value(raw: object) -> tuple[str | None, str | None, bool]:
    """Разобрать значение ответа → (select_value, free_text, is_custom).

    Возвращает один из вариантов:
      • select: (value, None, False)
      • custom: (None, custom_text, True)
    Списки обрабатываются отдельно вызывающим кодом.
    """
    if isinstance(raw, dict):
        val = raw.get("value")
        if val == CUSTOM_SENTINEL:
            return None, str(raw.get("custom_text", "")).strip() or None, True
        return (str(val) if val is not None else None), None, False
    if isinstance(raw, str):
        return raw, None, False
    return (str(raw) if raw is not None else None), None, False


# Маппинг dynamics-значений → метаданное dynamics для retrieval (docs/05 §3.2).
_DYNAMICS_META = {
    "no_change": "без_динамики",
    "positive": "улучшение",
    "slight_improvement": "улучшение",
    "stable_positive": "улучшение",
    "worsening": "ухудшение",
    "improvement": "улучшение",
    "no_improvement": "без_динамики",
}

# Маппинг syndrome-кодов → метаданное syndrome для retrieval-фильтра.
# Значения согласованы с экстрактором корпуса (chunking._SYNDROME_RE,
# хранит синдром в payload lowercase). Это сохраняет сквозную фильтрацию L0
# (doc_type + syndrome + diagnosis_class), docs/03 §6.
_SYNDROME_META = {
    "anxiety_depressive": "тревожно-депрессивный",
    "psychopathic": "психопатоподобный",
    "emotional_volitional": "эмоционально-волевой",
    "anxious": "тревожный",
    "asthenic": "астенический",
}


def map_answers(doc_type: str, answers: dict) -> MappedAnswers:
    """Смаппить ответы опросника в промпт-строки + метаданные (docs/06 §6).

    ВАЖНО: свободный текст в `answers` должен быть УЖЕ обезличен (оркестратор
    прогнал его через гейт ДО вызова этой функции).
    """
    select_q, freetext_q = _question_registry(doc_type)
    result = MappedAnswers()
    query_parts: list[str] = []

    # Идём по известным select-вопросам в порядке их объявления (стабильность).
    for qid, spec in select_q.items():
        if qid not in answers:
            continue
        raw = answers[qid]
        options: dict = spec.get("options", {})

        if spec.get("multi"):
            values = raw if isinstance(raw, list) else [raw]
            phrases: list[str] = []
            for v in values:
                sval, ctext, is_custom = _normalize_value(v)
                if is_custom and ctext:
                    phrases.append(ctext)
                elif sval and sval in options:
                    phrases.append(options[sval])
            if phrases:
                prefix = spec.get("multi_prefix", "")
                line = prefix + ", ".join(phrases) + "."
                result.prompt_lines.append(line)
                query_parts.extend(phrases)
            continue

        sval, ctext, is_custom = _normalize_value(raw)
        if is_custom and ctext:
            line = f"{spec['label']}: {ctext}."
            result.prompt_lines.append(line)
            query_parts.append(ctext)
            # «Свой вариант» синдрома: свободный текст идёт в метаданное как есть
            # (lowercase) — частичное совпадение с корпусом всё ещё возможно.
            if qid == "syndrome":
                result.syndrome = ctext.lower()
        elif sval and sval in options:
            line = options[sval]
            result.prompt_lines.append(line)
            query_parts.append(line)
            # Извлекаем метаданное динамики (для retrieval-фильтра).
            if qid in ("dynamics", "period_dynamics") and sval in _DYNAMICS_META:
                result.dynamics = _DYNAMICS_META[sval]
            # Извлекаем метаданное синдрома (select-код → метка корпуса).
            if qid == "syndrome" and sval in _SYNDROME_META:
                result.syndrome = _SYNDROME_META[sval]

    # Свободнотекстовые/уточняющие поля (уже обезличены).
    for qid, label in freetext_q.items():
        if qid not in answers:
            continue
        sval, ctext, is_custom = _normalize_value(answers[qid])
        text = ctext if (is_custom and ctext) else (sval or "")
        text = text.strip()
        if not text:
            continue
        result.prompt_lines.append(f"{label}: {text}.")
        query_parts.append(text)

        if qid == "diagnosis":
            result.diagnosis_class = extract_diagnosis_class(text)

    result.query_text = " ".join(query_parts).strip()
    result.title_safe = _build_title(doc_type, result)
    return result


def _build_title(doc_type: str, mapped: MappedAnswers) -> str:
    """Безопасный заголовок истории (docs/05 §2.2): тип · синдром · динамика."""
    type_label = ("Ежедневный дневник" if doc_type == DOC_TYPE_DAILY
                  else "Осмотр (раз в 10 дней)")
    bits = [type_label]
    if mapped.syndrome:
        bits.append(mapped.syndrome)
    if mapped.dynamics:
        bits.append(mapped.dynamics.replace("_", " "))
    return " · ".join(bits)


def iter_free_text(doc_type: str, answers: dict):
    """Извлечь все фрагменты свободного текста из ответов (для анонимизации).

    Возвращает генератор (path, text), где path — «адрес» значения внутри
    answers для последующей замены обезличенным текстом. Покрывает:
      • кастомные значения {"value":"__custom__","custom_text":...} в select;
      • элементы multiselect-списков с кастомом;
      • явные freetext-поля (строки).
    """
    select_q, freetext_q = _question_registry(doc_type)

    for qid, value in answers.items():
        # Списки (multiselect) — проверяем кастомные элементы.
        if isinstance(value, list):
            for idx, item in enumerate(value):
                if isinstance(item, dict) and item.get("value") == CUSTOM_SENTINEL:
                    txt = str(item.get("custom_text", "")).strip()
                    if txt:
                        yield (f"{qid}[{idx}].custom_text", txt)
            continue
        # Кастомный select.
        if isinstance(value, dict) and value.get("value") == CUSTOM_SENTINEL:
            txt = str(value.get("custom_text", "")).strip()
            if txt:
                yield (f"{qid}.custom_text", txt)
            continue
        # Явные freetext-поля (строковое значение, не из словаря опций).
        if isinstance(value, str) and qid in freetext_q:
            txt = value.strip()
            if txt:
                yield (f"{qid}", txt)
