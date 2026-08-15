"""Каркас бланка МИС — ежедневный осмотр и осмотр за 10 дней."""

from app.templates import (
    DAILY_TEMPLATE,
    DOC_TYPE_DAILY,
    DOC_TYPE_EXAM_10D,
    EXAM_10D_TEMPLATE,
    get_template,
)


def test_daily_skeleton_matches_mis_form() -> None:
    text = DAILY_TEMPLATE.render_skeleton()
    assert text.splitlines()[0] == "Осмотр лечащим врачом"
    assert text.splitlines()[1].startswith("Дата:")
    for needle in (
        "Жалобы: не предъявляет",
        "Анамнез заболевания (дополнения к анамнезу): без дополнений",
        "Анамнез жизни (дополнения к анамнезу): без дополнений",
        "Физикальное исследование, локальный статус (его изменение):",
        "Психический статус:",
        "Соматический статус:",
        "Неврологический статус: без острой неврологической симптоматики",
        "Диагноз:",
        "Основное заболевание:",
        "Сопутствующие заболевания: не выявлено",
        "Дополнительные сведения о заболевании: нет",
        "Обоснование диагноза (при наличии дополнительных сведений): не требуется",
        "Назначения: см. лист назначений",
        "Выполнены медицинские вмешательства: осмотр врачом-психиатром детским",
        "План обследования (дополнения к плану): без дополнений",
        "План лечения (дополнения к плану): без дополнений",
    ):
        assert needle in text, needle
    assert "Этапный эпикриз" not in text
    assert "Синдром:" not in text
    assert text.index("Психический статус:") < text.index(
        "Соматический статус:")
    assert text.index("Соматический статус:") < text.index(
        "Неврологический статус:")
    assert text.index("Неврологический статус:") < text.index("Диагноз:")
    life = "Анамнез жизни (дополнения к анамнезу): без дополнений"
    after_life = text.split(life, 1)[1]
    assert after_life.startswith("\n\n"), "blank line after анамнез жизни"
    diag_at = text.index("\nДиагноз:")
    assert text[diag_at - 1] == "\n", "blank line before Диагноз"
    assert "[ДОЛЖНОСТЬ_ВРАЧА] [ФИО_ВРАЧА]" in text
    assert "ИБ №" not in text


def test_exam_10d_skeleton_matches_mis_form() -> None:
    text = EXAM_10D_TEMPLATE.render_skeleton()
    lines = text.splitlines()
    assert lines[1] == "ОСМОТР"
    assert lines[2] == "лечащим врачом совместно с заведующим отделением"
    for needle in (
        "Неврологический статус (его изменение):",
        "Психический статус (его изменение):",
        "Синдром:",
        "Этапный эпикриз:",
        "План лечения (дополнения к плану): без дополнений",
    ):
        assert needle in text, needle
    assert text.index("Физикальное исследование") < text.index(
        "Неврологический статус (его изменение)"
    )
    assert text.index("Неврологический статус (его изменение)") < text.index(
        "Психический статус (его изменение)"
    )
    assert text.index("Основное заболевание:") < text.index("Синдром:")
    assert text.index("Синдром:") < text.index("Сопутствующие заболевания:")
    assert text.index("Этапный эпикриз:") > text.index("План лечения")
    assert "Фамилия, имя, отчество (при наличии) заведующего отделением, подпись" in text
    assert "[ФИО_ВРАЧА], [ДОЛЖНОСТЬ_ВРАЧА]" in text
    assert "[ФИО_ЗАВ_ОТДЕЛЕНИЕМ], [ДОЛЖНОСТЬ_ЗАВ_ОТДЕЛЕНИЕМ], [ЛУ]" in text
    assert text.count(
        "Фамилия, имя, отчество (при наличии) врача, должность, специальность, подпись"
    ) == 1


def test_get_template_roundtrip() -> None:
    assert get_template(DOC_TYPE_DAILY) is DAILY_TEMPLATE
    assert get_template(DOC_TYPE_EXAM_10D) is EXAM_10D_TEMPLATE
