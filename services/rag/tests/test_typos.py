from app.typos import fix_obvious_typos


def test_fix_obvious_typos_syndrome() -> None:
    src = "Основное заболевание: F71.18 Сидрос психмоторной расторможенности"
    out = fix_obvious_typos(src)
    assert "Сидрос" not in out
    assert "психмоторной" not in out
    assert "Синдром психомоторной расторможенности" in out


def test_fix_psikhomotonoy_typo() -> None:
    src = "Синдром психомотоной расторможенности. убгает через забор."
    out = fix_obvious_typos(src)
    assert "психомотоной" not in out
    assert "психомоторной" in out
    assert "убегает" in out


def test_fix_obvious_typos_leaves_correct_text() -> None:
    src = "Синдром психомоторной расторможенности"
    assert fix_obvious_typos(src) == src


def test_fix_complaints_without_samostoyatelno() -> None:
    src = "Жалобы: самостоятельно не предъявляет"
    assert fix_obvious_typos(src) == "Жалобы: не предъявляет"
    assert "ест самостоятельно" in fix_obvious_typos("ест самостоятельно")


def test_fix_english_leak_and_ocliks() -> None:
    src = "Фон настроения mildly раздражительный. На оклики реагирует непродолжительно."
    out = fix_obvious_typos(src)
    assert "mildly" not in out
    assert "слегка раздражительный" in out
    assert "на оклики" not in out.lower()
    assert "на замечания" in out.lower()
