"""Юнит-тесты структурного чанкинга (docs/03 §4).

Работают с ОБЕЗЛИЧЕННЫМ текстом (плейсхолдеры [ДАТА]/[ПАЦИЕНТ]) — как после
гейта gateway. Тяжёлых зависимостей не требуют (только stdlib + app.config).

Запуск: cd services/rag && python -m pytest tests/ -q
"""

from __future__ import annotations

from app.chunking import chunk_document
from app.config import Settings


def _settings() -> Settings:
    return Settings()


def test_daily_records_split_by_date_placeholder() -> None:
    text = (
        "[ДАТА] Психический статус: фон настроения снижен, сон нарушен. "
        "Поведение упорядочено. Без существенной динамики. [ФИО_ВРАЧА]\n"
        "[ДАТА] Психический статус: настроение ровное, аппетит сохранён. "
        "Отмечается улучшение состояния. [ФИО_ВРАЧА]"
    )
    chunks = chunk_document(text, _settings())
    assert len(chunks) == 2
    assert all(c.doc_type == "daily" for c in chunks)
    assert all(c.section == "full" for c in chunks)
    # Динамика извлекается эвристикой.
    assert chunks[0].dynamics == "без_динамики"
    assert chunks[1].dynamics == "улучшение"


def test_exam_record_split_into_sections() -> None:
    text = (
        "[ДАТА]\n"
        "Жалобы: на сниженное настроение, тревогу.\n"
        "Анамнез заболевания: ухудшение в течение недели описано подробно тут.\n"
        "Неврологический статус: без очаговой симптоматики, рефлексы живые равные.\n"
        "Психический статус: фон настроения гипотимный, мышление в обычном темпе.\n"
        "Диагноз: F41.2 смешанное тревожное расстройство, тревожно-депрессивный синдром.\n"
        "Этапный эпикриз: состояние с положительной динамикой, продолжает терапию."
    )
    chunks = chunk_document(text, _settings())
    assert len(chunks) >= 4
    assert all(c.doc_type == "exam_10d" for c in chunks)
    sections = {c.section for c in chunks}
    assert {"complaints", "neuro", "psych_status", "epicrisis"} & sections
    # Опциональные метаданные.
    assert any(c.diagnosis_class == "F4x" for c in chunks)
    # _SYNDROME_RE matches «тревожное расстройство» first (earlier in text),
    # normalize_syndrome → канонич. «тревожный». Тест проверяет наличие syndrome.
    assert any(c.syndrome == "тревожный" for c in chunks)


def test_short_noise_records_dropped() -> None:
    text = "[ДАТА] [ФИО_ВРАЧА]"  # короткая подпись без содержания
    chunks = chunk_document(text, _settings())
    assert chunks == []


def test_long_record_soft_split_with_overlap() -> None:
    body = "Психический статус описан очень подробно. " * 80  # > chunk_max_chars
    text = f"[ДАТА] {body} [ФИО_ВРАЧА]"
    chunks = chunk_document(text, _settings())
    assert len(chunks) >= 2
    assert all(len(c.text) <= _settings().chunk_max_chars + 50 for c in chunks)
