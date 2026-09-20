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


def test_daily_records_inherit_document_diagnosis() -> None:
    """Код МКБ стоит в шапке ИБ один раз — дневники дня его не повторяют.

    Без наследования метаданных 99% ежедневных фрагментов уходили в базу
    без класса диагноза и не находились строгим фильтром (прод, 20.09).
    """
    text = (
        "История болезни\n"
        "Основное заболевание: F92.0. Депрессивное расстройство поведения\n"
        "Синдром тревожно-депрессивный\n\n"
        "[ДАТА] Осмотр лечащим врачом\n"
        "Жалобы: не предъявляет\n"
        "Психический статус: Сознание ясное. В игровой читал книги, "
        "с детьми общался избирательно. Настроение ровное.\n\n"
        "[ДАТА] Осмотр лечащим врачом\n"
        "Жалобы: не предъявляет\n"
        "Психический статус: Сознание ясное. Держался обособленно, "
        "большую часть дня провёл в палате. Настроение снижено.\n"
    )
    chunks = chunk_document(text, _settings())
    daily = [c for c in chunks if c.doc_type == "daily"]
    assert len(daily) >= 2
    assert all(c.diagnosis_class == "F9x" for c in daily)


def test_document_diagnosis_is_not_guessed_when_ambiguous() -> None:
    """Два разных класса в документе — лучше без метки, чем с чужой."""
    text = (
        "Сборник\nОсновное заболевание: F92.0\nОсновное заболевание: F71.18\n\n"
        "[ДАТА] Осмотр лечащим врачом\n"
        "Психический статус: Сознание ясное. Настроение ровное, поведение "
        "упорядоченное, замечаний не получал.\n"
    )
    # Сам фрагмент дневника метки не получает (у шапки свой код — в её тексте).
    diary = [c for c in chunk_document(text, _settings())
             if "Психический статус" in c.text]
    assert diary and all(c.diagnosis_class is None for c in diary)


def test_record_diagnosis_wins_over_document() -> None:
    text = (
        "Основное заболевание: F92.0\n\n"
        "[ДАТА] Осмотр лечащим врачом\n"
        "Основное заболевание: F71.18. Умственная отсталость умеренная\n"
        "Психический статус: Сознание ясное. Играл с кубиками, речь "
        "представлена отдельными словами, критика отсутствует.\n"
    )
    daily = [c for c in chunk_document(text, _settings()) if c.doc_type == "daily"]
    assert daily and all(c.diagnosis_class == "F7x" for c in daily)


def test_daily_blank_is_not_mistaken_for_a_10_day_exam() -> None:
    """«Жалобы» и «анамнез» есть в каждом бланке — это не признак осмотра."""
    daily = (
        "[ДАТА] Осмотр лечащим врачом\n"
        "Жалобы: не предъявляет\n"
        "Анамнез заболевания (дополнения к анамнезу): без дополнений\n"
        "Психический статус: Сознание ясное. В игровой читал книги, "
        "с детьми общался избирательно, замечаний не получал.\n"
        "Соматический статус: Кожа чистая, зев спокоен, живот мягкий.\n"
        "Неврологический статус: без острой неврологической симптоматики\n"
    )
    chunks = chunk_document(daily, _settings())
    assert {c.doc_type for c in chunks} == {"daily"}
    assert "psych_status" in {c.section for c in chunks}


def test_real_10_day_exam_is_still_detected() -> None:
    exam = (
        "[ДАТА] ОСМОТР лечащим врачом совместно с заведующим отделением\n"
        "Жалобы: не предъявляет\n"
        "Психический статус (его изменение): Сознание ясное, ориентирован верно, "
        "настроение ровное, поведение упорядоченное.\n"
        "Этапный эпикриз: за период состояние с положительной динамикой, "
        "тревога уменьшилась, режим отделения соблюдает.\n"
    )
    assert {c.doc_type for c in chunk_document(exam, _settings())} == {"exam_10d"}


def test_recreate_collection_drops_old_points() -> None:
    """Разметка чанков изменилась — старые точки должны уйти, а не остаться."""
    from app.qdrant_store import QdrantStore

    class _FakeClient:
        def __init__(self) -> None:
            self.calls: list[str] = []
            self.exists = True

        def collection_exists(self, name: str) -> bool:
            return self.exists

        def delete_collection(self, name: str) -> None:
            self.calls.append("delete")
            self.exists = False

        def create_collection(self, **kwargs) -> None:
            self.calls.append("create")

        def create_payload_index(self, **kwargs) -> None:
            self.calls.append("index")

    fake = _FakeClient()
    store = QdrantStore(_settings(), client=fake)
    store.recreate_collection(1024)
    assert fake.calls[:2] == ["delete", "create"]


def test_document_header_is_not_indexed_as_style_sample() -> None:
    """Шапка ИБ попадала в корпус как «дневник» и могла уйти в промпт."""
    text = (
        "История болезни\nОсновное заболевание: F92.0\n\n"
        "[ДАТА] Осмотр лечащим врачом\n"
        "Психический статус: Сознание ясное. В игровой читал книги, "
        "с детьми общался избирательно, замечаний не получал.\n"
    )
    chunks = chunk_document(text, _settings())
    assert all("История болезни" not in c.text for c in chunks)
    assert any("читал книги" in c.text for c in chunks)
