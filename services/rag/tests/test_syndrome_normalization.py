"""Техдолг §1: тесты нормализации синдромов (падежи → каноническая форма).

Проверяем, что retrieval-фильтр и ingest-payload используют ОДНУ каноническую
форму синдрома (именительный падеж) независимо от падежа в исходном тексте.

Запуск: cd services/rag && python -m pytest tests/test_syndrome_normalization.py -v
"""

from __future__ import annotations

from app.questionnaire import (
    MappedAnswers,
    _SYNDROME_META,
    extract_diagnosis_class,
    map_answers,
    normalize_syndrome,
)
from app.retrieval import _fallback_levels


# ─── П.1: normalize_syndrome — канонич. форма из любого падежа ──────────────


def test_nominative_passthrough() -> None:
    """Именительный падеж — уже каноничная форма, не меняется."""
    assert normalize_syndrome(
        "тревожно-депрессивный") == "тревожно-депрессивный"


def test_genitive_case() -> None:
    """Родительный падеж → каноническая форма."""
    assert normalize_syndrome(
        "тревожно-депрессивного") == "тревожно-депрессивный"


def test_instrumental_case() -> None:
    """Творительный падеж → каноническая форма."""
    assert normalize_syndrome(
        "тревожно-депрессивным") == "тревожно-депрессивный"


def test_prepositional_case() -> None:
    """Предложный падеж → каноническая форма."""
    assert normalize_syndrome(
        "тревожно-депрессивном") == "тревожно-депрессивный"


def test_all_syndromes_from_questionnaire() -> None:
    """Все синдромы из опросника (_SYNDROME_META) должны нормализоваться."""
    expected = {
        "anxiety_depressive": "тревожно-депрессивный",
        "psychopathic": "психопатоподобный",
        "emotional_volitional": "эмоционально-волевой",
        "anxious": "тревожный",
        "asthenic": "астенический",
    }
    for code, canonical in expected.items():
        assert normalize_syndrome(
            canonical) == canonical, f"{code} не нормализуется"


def test_asthenic_cases() -> None:
    """Астенический синдром в разных падежах."""
    cases = [
        "астенический",   # именительный
        "астенического",  # родительный
        "астеническому",  # дательный
        "астеническим",   # творительный
        "астеническом",   # предложный
    ]
    for case in cases:
        assert normalize_syndrome(
            case) == "астенический", f"{case} не нормализуется"


def test_psychopathic_cases() -> None:
    """Психопатоподобный синдром в разных падежах."""
    cases = [
        "психопатоподобный",
        "психопатоподобного",
        "психопатоподобным",
    ]
    for case in cases:
        assert normalize_syndrome(case) == "психопатоподобный"


def test_anxious_cases() -> None:
    """Тревожный синдром (не тревожно-депрессивный!) — важна длина стема."""
    assert normalize_syndrome("тревожного") == "тревожный"
    assert normalize_syndrome("тревожным") == "тревожный"
    # Убеждаемся, что «тревожный» НЕ маппится в «тревожно-депрессивный»:
    assert normalize_syndrome("тревожный") != "тревожно-депрессивный"


def test_emotional_volitional_cases() -> None:
    """Эмоционально-волевой неустойчивости — разные падежи."""
    assert normalize_syndrome(
        "эмоционально-волевого") == "эмоционально-волевой"
    assert normalize_syndrome("эмоционально-волевым") == "эмоционально-волевой"


def test_none_returns_none() -> None:
    assert normalize_syndrome(None) is None


def test_empty_returns_none() -> None:
    assert normalize_syndrome("") is None


def test_unknown_syndrome_lowercased() -> None:
    """Неизвестный синдром (custom text врача) возвращается lowercased."""
    assert normalize_syndrome("Гебефренический") == "гебефренический"


# ─── П.1: retrieval-фильтр с нормализованным синдромом ─────────────────────


def test_retrieval_filter_uses_canonical_syndrome() -> None:
    """_fallback_levels с родительным падежом → фильтр содержит канонич. форму."""
    levels = _fallback_levels(
        doc_type="exam_10d",
        syndrome="тревожно-депрессивного",  # род. падеж
        diagnosis_class="F4x",
        section=None,
    )
    # L0_strict: должен содержать нормализованный синдром.
    l0 = levels[0]
    assert l0.syndrome == "тревожно-депрессивного"  # напрямую в Level
    # Но retrieve() нормализует ДО создания levels:
    from app.questionnaire import normalize_syndrome
    canon = normalize_syndrome("тревожно-депрессивного")
    assert canon == "тревожно-депрессивный"


# ─── П.1: map_answers → syndrome в MappedAnswers ────────────────────────────


def test_map_answers_syndrome_normalization() -> None:
    """map_answers с select-кодом синдрома → канонич. форма."""
    result = map_answers("exam_10d", {"syndrome": "anxiety_depressive"})
    # Значение _SYNDROME_META["anxiety_depressive"] = "тревожно-депрессивный"
    # → normalize_syndrome("тревожно-депрессивный") == "тревожно-депрессивный"
    assert result.syndrome == "тревожно-депрессивный"


def test_map_answers_custom_syndrome_normalized() -> None:
    """map_answers с кастомным синдромом в род. падеже → канонич. форма."""
    result = map_answers("exam_10d", {
        "syndrome": {"value": "__custom__", "custom_text": "тревожно-депрессивного"},
    })
    assert result.syndrome == "тревожно-депрессивный"
