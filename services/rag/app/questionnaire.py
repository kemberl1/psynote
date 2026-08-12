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
            "even": "Фон настроения ровный, без резких колебаний.",
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
            "calm_distance": "в беседе с врачом спокоен, дистанцию соблюдает",
            "productive": "доступен продуктивному контакту",
            "selective_children": "общается с детьми избирательно",
            "isolated": "держится обособленно",
            "polite_staff": "с персоналом вежлив",
            "negativistic": "негативистичен",
            "does_not_disclose": "переживаний в полном объеме не раскрывает",
            "staff_remarks": ("со слов персонала получал замечания, на них "
                              "реагировал непродолжительно"),
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
            "preserved": "Аппетит достаточен, избирателен.",
            "decreased": "Аппетит снижен (в период болезни).",
            "selective": "Аппетит избирателен.",
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
            "none": "Жалобы не предъявляет.",
            "cannot_formulate": "Жалобы самостоятельно не формирует.",
            "present": "Предъявляет жалобы.",
        },
    },
    "anamnesis_disease": {
        "label": "Анамнез заболевания (дополнения)",
        "options": {
            "no_additions": "Анамнез заболевания: без дополнений.",
            "present": "Имеются дополнения к анамнезу заболевания.",
        },
    },
    "events": {
        "label": "События дня",
        "multi": True,
        "options": {
            "therapy_correction": "проведена коррекция терапии",
            "somatic": "отмечается сопутствующее соматическое заболевание",
            "examination": "выполнено обследование",
            "weekend_duty": "детали выходного дня / наблюдения дежурного персонала",
            "relative_visit": "визит родственника / прогулка (только если реально было)",
        },
        "multi_prefix": "События дня: ",
    },
    "exam_plan": {
        "label": "План обследования",
        "options": {
            "no_change": "План обследования: без дополнений.",
            "adjusted": ("План обследования: в связи с изменением состояния "
                         "проведена корректировка."),
        },
    },
    "prescriptions": {
        "label": "Назначения",
        "options": {
            "see_list": ("Назначения: лекарственная терапия согласно листу "
                         "назначений (только препараты, без режима отделения)."),
            "no_change": ("Назначения без изменений (только препараты, "
                          "без режима отделения)."),
        },
    },
}

# Условные/уточняющие вопросы со свободным вводом (docs/06 §4.2).
# Это поля типа text (и detail-уточнения), значения которых — свободный текст
# врача; они проходят анонимайзер-гейт ДО маппинга (docs/04). Multiselect-поля
# с allow_custom (mood_detail, sleep_detail, …) обрабатываются как select-блок
# выше, а их кастом-элементы извлекаются iter_free_text напрямую.
#
# Специальный ключ __arc_context__: режиссёрский контекст пакетной генерации.
# Проходит анонимайзер-гейт, но НЕ попадает в prompt_lines —
# вместо этого сохраняется в MappedAnswers.director_note и инжектируется
# в системный промпт с пометкой «не цитировать» (см. generation.py).
_DAILY_FREETEXT_QUESTIONS: dict[str, str] = {
    "dynamics_detail": "В чём проявляется ухудшение",
    "adverse_detail": "Нежелательные явления",
    "complaints_detail": "Жалобы",
    "anamnesis_detail": "Дополнения к анамнезу",
    "events_detail": "Детали событий дня",
    "exam_plan_detail": "План обследования (причина корректировки)",
    "additional_info": "Дополнительные сведения (выходные / понедельник)",
    "diagnosis": "Основное заболевание (МКБ-10)",
    "__arc_context__": "Контекст нарратива (служебный)",
}

# Расширенный осмотр (docs/06 §5). Базовый блок наследуется от daily.
# anamnesis_disease уже в daily — здесь не дублируем.
_EXAM_QUESTIONS: dict[str, dict] = {
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
    "orientation": {
        "label": "Ориентировка",
        "options": {
            "partial_typical": ("Ориентирован(а) частично (в месте, времени, "
                                "собственной личности)."),
            "correct": "Ориентирован(а) верно в месте, времени, собственной личности.",
            "impaired": "Ориентировка нарушена.",
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
            "age_norm": ("Интеллектуально представляется на уровне возрастной "
                         "нормы, запас сведений неравномерен. На вопросы из "
                         "школьной программы отвечает выборочно."),
            "low_norm": ("Интеллектуально представляется на уровне низкой "
                         "возрастной нормы, запас сведений неравномерен. "
                         "На вопросы из школьной программы отвечает выборочно."),
            "mild_id": ("Интеллектуально представляется сниженным до уровня "
                        "легкой УО, запас сведений неравномерен. На вопросы "
                        "из школьной программы отвечает выборочно."),
            "reduced": ("Интеллектуально-мнестически снижен, запас сведений "
                        "неравномерен."),
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
            "behavioral": "синдром поведенческих нарушений",
            "anxious": "тревожный синдром",
            "depressive": "депрессивный синдром",
            "psychomotor_aggression": (
                "синдром психомоторной расторможенности (с агрессией)"),
            "psychomotor_autoaggression": (
                "синдром психомоторной расторможенности (с аутоагрессией)"),
            "affective_volitional": "синдром аффективно-волевой неустойчивости",
            "psychopathic": "психопатоподобный синдром",
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
    "interventions": {
        "label": "Выполненные вмешательства",
        "multi": True,
        "options": {
            # Консультации доп. специалистов убраны — заполняются вручную.
            # «Контроль медикаментов» не предлагаем.
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
    "physical_detail": "Физикальный статус (изменения)",
    "neuro_detail": "Неврологический статус (изменения)",
    "suicidal_detail": "Суицидальные тенденции (уточнение)",
    "diagnosis": "Основное заболевание (МКБ-10)",
    "intellect_example": "Пример ответа на вопрос из школьной программы",
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
# Техдолг §2: regex матчит И латинскую F, И кириллическую Ф — в корпусе
# встречаются оба варианта; только F-коды (психиатрия) → diagnosis_class;
# R, J, E и т.д. не маппятся (соматические/сопутствующие).
_ICD_RE = re.compile(r"\b[ФF](\d)\d", re.IGNORECASE)


def extract_diagnosis_class(diagnosis_text: str) -> str | None:
    """Верхний уровень МКБ из текста диагноза, напр. 'F92.8' → 'F9x' (docs/05).

    Техдолг §2: покрывает ВСЕ F-коды F0x–F9x (МКБ-10 глава V).
    Обрабатывает лат. F и кир. Ф; R/J/E коды игнорируются (не класс психиатрии).
    """
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
    # Режиссёрский контекст нарратива (из __arc_context__ в answers).
    # Инжектируется в системный промпт, НЕ в текст дневника.
    director_note: str | None = None


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
    # Актуальные коды UI / опросника.
    "behavioral": "поведенческих нарушений",
    "anxious": "тревожный",
    "depressive": "депрессивный",
    "psychomotor_aggression": "психомоторной расторможенности",
    "psychomotor_autoaggression": "психомоторной расторможенности",
    "affective_volitional": "аффективно-волевой",
    "psychopathic": "психопатоподобный",
    "asthenic": "астенический",
    # Legacy-коды (старые ответы / тесты) → канон для retrieval.
    "anxiety_depressive": "тревожно-депрессивный",
    "emotional_volitional": "аффективно-волевой",
}

# ─── Нормализация syndrome (падежи → каноническая форма, техдолг §1) ──────────
# Корпус хранит синдромы в разных падежах (например «тревожно-депрессивный»,
# «тревожно-депрессивного», «тревожно-депрессивным»). Для корректного
# Qdrant-фильтра (exact match по keyword-payload) нужна единая каноническая
# форма — именительный падеж, единственное число, мужской род.
#
# Стратегия: маппинг «стем → каноническая форма». Стем — это prefix до
# склоняемого суффикса (совпадает с regex-ом chunking._SYNDROME_RE).
# Итерация от длинных стемов к коротким исключает коллизии
# («тревожно-депрессивн» побеждает «тревожн»).

_SYNDROME_STEMS: list[tuple[str, str]] = sorted(
    [
        ("тревожно-депрессивн", "тревожно-депрессивный"),
        ("аффективно-волев", "аффективно-волевой"),
        ("эмоционально-волев", "аффективно-волевой"),
        ("психомоторн", "психомоторной расторможенности"),
        ("поведенческ", "поведенческих нарушений"),
        ("психопатоподобн", "психопатоподобный"),
        ("апато-абулическ", "апато-абулический"),
        ("кататоническ", "кататонический"),
        ("неврозоподобн", "неврозоподобный"),
        ("маниакальн", "маниакальный"),
        ("астеническ", "астенический"),
        ("тревожн", "тревожный"),
        ("депрессивн", "депрессивный"),
    ],
    key=lambda pair: -len(pair[0]),  # longest stems first
)


def normalize_syndrome(raw: str | None) -> str | None:
    """Привести синдром в любом падеже к канонической форме (им. п.).

    Используется и при индексации (chunking → payload), и при retrieval
    (questionnaire → фильтр). Совпадает stem-подход chunking._SYNDROME_RE —
    поэтому любое значение, извлечённое экстрактором, нормализуется.

    >>> normalize_syndrome("тревожно-депрессивного")
    'тревожно-депрессивный'
    >>> normalize_syndrome("астеническим")
    'астенический'
    >>> normalize_syndrome(None) is None
    True
    """
    if not raw:
        return None
    lower = raw.strip().lower()
    for stem, canonical in _SYNDROME_STEMS:
        if lower.startswith(stem):
            return canonical
    # Неизвестный синдром (кастомный текст врача) — возвращаем lowercased.
    return lower


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
            # «Свой вариант» синдрома: свободный текст → каноническая форма (падежи).
            if qid == "syndrome":
                result.syndrome = normalize_syndrome(ctext)
        elif sval and sval in options:
            line = options[sval]
            result.prompt_lines.append(line)
            query_parts.append(line)
            # Извлекаем метаданное динамики (для retrieval-фильтра).
            if qid in ("dynamics", "period_dynamics") and sval in _DYNAMICS_META:
                result.dynamics = _DYNAMICS_META[sval]
            # Извлекаем метаданное синдрома (select-код → каноническая форма).
            if qid == "syndrome" and sval in _SYNDROME_META:
                result.syndrome = normalize_syndrome(_SYNDROME_META[sval])

    # Свободнотекстовые/уточняющие поля (уже обезличены).
    for qid, label in freetext_q.items():
        if qid not in answers:
            continue
        sval, ctext, is_custom = _normalize_value(answers[qid])
        text = ctext if (is_custom and ctext) else (sval or "")
        text = text.strip()
        if not text:
            continue

        # __arc_context__ — режиссёрский контекст пакетной генерации.
        # Не идёт в prompt_lines (не вставляется в дневник), а сохраняется
        # в director_note и затем инжектируется в системный промпт.
        if qid == "__arc_context__":
            result.director_note = text
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
    if doc_type == DOC_TYPE_DAILY:
        type_label = "Ежедневный осмотр"
    else:
        type_label = "Осмотр за 10 дней"
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
