"""Техдолг §2: тесты маппинга F-кодов → diagnosis_class.

Проверяем:
  1. extract_diagnosis_class() корректно извлекает класс из ВСЕХ F-кодов F0x–F9x
     (включая кириллическую «Ф»).
  2. R/J/E/… коды НЕ маппятся в diagnosis_class (соматические — не психиатрия).
  3. chunking._extract_meta() ограничивает _ICD_RE только F/Ф для diagnosis_class.

Запуск: cd services/rag && python -m pytest tests/test_diagnosis_class_mapping.py -v
"""

from __future__ import annotations

from app.chunking import chunk_document
from app.config import Settings
from app.questionnaire import extract_diagnosis_class


# ─── П.2: extract_diagnosis_class — полный набор F-кодов МКБ-10, глава V ───

# Все классы F0x–F9x существуют в МКБ-10. Проверяем граничные и типичные коды,
# которые могут встретиться в психиатрическом корпусе.
_TYPICAL_F_CODES: list[tuple[str, str]] = [
    # (код_из_текста, ожидаемый_class)
    ("F00.0", "F0x"),   # Деменция при болезни Альцгеймера
    ("F06.68", "F0x"),  # Органическое эмоц. лабильное расст.
    ("F10.1", "F1x"),   # Алкоголизм (употребление с вредом)
    ("F20.0", "F2x"),   # Шизофрения параноидная
    ("F31.1", "F3x"),   # Биполярное аффективное расстройство
    ("F32.0", "F3x"),   # Депрессивный эпизод лёгкий
    ("F41.2", "F4x"),   # Смешанное тревожное расстройство
    ("F43.0", "F4x"),   # Острая реакция на стресс
    ("F50.0", "F5x"),   # Нервная анорексия
    ("F60.3", "F6x"),   # Эмоц. неустойч. расстр. личности
    ("F70", "F7x"),     # Умств. отсталость лёгкая
    ("F80.1", "F8x"),   # Расст. экспрессивной речи
    ("F84.11", "F8x"),  # Атипичный аутизм (дети — часто в корпусе)
    ("F90.0", "F9x"),   # Синдром дефицита внимания
    ("F91.0", "F9x"),   # Расст. поведения в семье
    ("F92.0", "F9x"),   # Смешанное расст. поведения и эмоций
    ("F92.8", "F9x"),   # Другие смешанные расстройства (часто в корпусе)
    ("F95.2", "F9x"),   # Комбинир. вокализм + моторные тики
]


def test_extract_diagnosis_class_all_f_codes() -> None:
    """Все типичные F-коды из психиатрического корпуса → корректный класс."""
    for code, expected_class in _TYPICAL_F_CODES:
        result = extract_diagnosis_class(code)
        assert result == expected_class, (
            f"{code} → {result!r}, ожидалось {expected_class!r}"
        )


def test_extract_diagnosis_class_cyrillic_f() -> None:
    """Кириллическая «Ф92.8» тоже должна маппиться (встречается в корпусе)."""
    assert extract_diagnosis_class("Ф92.8") == "F9x"
    assert extract_diagnosis_class("Ф06.68") == "F0x"


def test_extract_diagnosis_class_with_context() -> None:
    """F-код внутри текста диагноза — извлекается корректно."""
    text = "Основное заболевание: F92.8 смешанное расстройство поведения и эмоций"
    assert extract_diagnosis_class(text) == "F9x"


def test_extract_diagnosis_class_r_code_ignored() -> None:
    """R-коды (соматические) НЕ маппятся в diagnosis_class."""
    assert extract_diagnosis_class("R51") is None
    assert extract_diagnosis_class("R10.4") is None


def test_extract_diagnosis_class_j_e_codes_ignored() -> None:
    """J (дыхание), E (эндокрин) — не психиатрические → не маппятся."""
    assert extract_diagnosis_class("J00") is None
    assert extract_diagnosis_class("E66.9") is None


def test_extract_diagnosis_class_empty_and_none() -> None:
    """Пустой/None ввод → None."""
    assert extract_diagnosis_class("") is None
    assert extract_diagnosis_class(None) is None  # type: ignore[arg-type]


# ─── П.2: chunking._extract_meta — diagnosis_class только для F/Ф ───────────


def _settings() -> Settings:
    return Settings()


def test_chunk_meta_f_code_in_exam() -> None:
    """F41.2 в тексте расширенного осмотра → diagnosis_class='F4x'."""
    text = (
        "[ДАТА]\n"
        "Жалобы: тревога, сниженное настроение, подробно описаны.\n"
        "Анамнез заболевания: ухудшение за последний месяц.\n"
        "Психический статус: настроение снижено, подробно описано.\n"
        "Диагноз: F41.2 смешанное тревожное расстройство, тревожно-депрессивный синдром.\n"
        "Этапный эпикриз: состояние с улучшением, продолжает терапию."
    )
    chunks = chunk_document(text, _settings())
    assert any(c.diagnosis_class == "F4x" for c in chunks), (
        "F41.2 → F4x не извлечён из осмотра"
    )


def test_chunk_meta_r_code_not_diagnosis_class() -> None:
    """R51 (головная боль — сопутствующий) НЕ становится diagnosis_class.

    Ранее _ICD_RE матчит любую букву, и R51 → R5x. Теперь ТОЛЬКО F → класс.
    """
    text = (
        "[ДАТА]\n"
        "Жалобы: жалуется на головную боль, подробно описано.\n"
        "Анамнез заболевания: история подробно изложена.\n"
        "Психический статус: поведение подробно описано.\n"
        "Диагноз: F92.8. Сопутствующие: R51 головная боль.\n"
        "Этапный эпикриз: состояние без существенных изменений."
    )
    chunks = chunk_document(text, _settings())
    # diagnosis_class должен быть F9x (от F92.8), НЕ R5x (от R51).
    classes = {c.diagnosis_class for c in chunks if c.diagnosis_class}
    assert "F9x" in classes, "F92.8 → F9x не извлечён"
    assert "R5x" not in classes, "R51 не должен стать diagnosis_class"


def test_chunk_meta_j_e_ignored_for_class() -> None:
    """J00/E66.9 (сопутствующие соматические) НЕ маппятся в diagnosis_class."""
    text = (
        "[ДАТА]\n"
        "Анамнез заболевания: подробно описано в документе.\n"
        "Психический статус: подробно описано состояние пациента.\n"
        "Диагноз: F84.11 атипичный аутизм. Сопутствующие: J00, E66.9.\n"
        "Этапный эпикриз: состояние с улучшением."
    )
    chunks = chunk_document(text, _settings())
    classes = {c.diagnosis_class for c in chunks if c.diagnosis_class}
    assert "F8x" in classes, "F84.11 → F8x"
    assert "Jx" not in classes
    assert "Ex" not in classes
