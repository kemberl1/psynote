from app.typos import fix_obvious_typos


def test_fix_obvious_typos_syndrome() -> None:
    src = "Основное заболевание: F71.18 Сидрос психмоторной расторможенности"
    out = fix_obvious_typos(src)
    assert "Сидрос" not in out
    assert "психмоторной" not in out
    assert "Синдром психомоторной расторможенности" in out


def test_fix_obvious_typos_leaves_correct_text() -> None:
    src = "Синдром психомоторной расторможенности"
    assert fix_obvious_typos(src) == src


def test_fix_complaints_without_samostoyatelno() -> None:
    src = "Жалобы: самостоятельно не предъявляет"
    assert fix_obvious_typos(src) == "Жалобы: не предъявляет"
    assert "ест самостоятельно" in fix_obvious_typos("ест самостоятельно")
