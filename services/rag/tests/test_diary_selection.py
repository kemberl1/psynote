"""Юнит-тесты отбора файлов-ДНЕВНИКОВ из дерева корпуса (Этап 4.1).

Проверяем, ЧТО берётся как дневник и ЧТО отсеивается, на синтетическом дереве
путей (БЕЗ реальных ПДн и БЕЗ обращения к файловой системе/сети). DiarySelector
работает с pathlib.Path по имени/сегментам пути — реальные файлы не нужны.

Запуск: cd services/rag && python -m pytest tests/test_diary_selection.py -q
"""

from __future__ import annotations

from pathlib import Path

from app.config import Settings
from app.ingestion import DiarySelector, select_diary_files


def _selector() -> DiarySelector:
    return DiarySelector.from_settings(Settings())


# Синтетическое дерево корпуса (имена с «ФИО» — выдуманные, не реальные ПДн).
# (относительный_путь, ожидается_ли_отбор_как_дневник)
_TREE: list[tuple[str, bool]] = [
    # 1) сборник дневников ИБ — берём по ПАПКЕ (даже если имя без «дневник»).
    ("02_корпус/сборник_дневников_ИБ/case_01.docx", True),
    ("02_корпус/сборник_дневников_ИБ/случай_02.odt", True),
    # 2) заготовки дневников — берём по ПАПКЕ.
    ("02_корпус/заготовки_дневников/шаблон_A.docx", True),
    # 3) дневники в папках пациентов — берём по ИМЕНИ («дневник»).
    ("02_корпус/выписанные/Петров П.П./дневники_наблюдения.docx", True),
    ("02_корпус/истории/Сидоров С.С./Дневник_осмотров.docx", True),
    ("02_корпус/текущие_пациенты/Котов К.К./Дневники.docx", True),
    ("02_корпус/текущие_пациенты/Котов К.К./ДНЕВНИК_10дней.doc", True),
    # 4) НЕ-дневники в папках пациентов — отсев по exclude (нет «дневник» в имени).
    ("02_корпус/выписанные/Петров П.П./первичный_осмотр.docx", False),
    ("02_корпус/выписанные/Петров П.П./выписной_эпикриз.docx", False),
    ("02_корпус/истории/Сидоров С.С./выписка.docx", False),
    ("02_корпус/текущие_пациенты/Котов К.К./лист_назначений.docx", False),
    ("02_корпус/текущие_пациенты/Котов К.К./статкарта.docx", False),
    # 5) посторонние файлы без признаков дневника — отсев.
    ("02_корпус/прочее/анамнез_жизни.docx", False),
    ("02_корпус/прочее/заметки.odt", False),
]


def test_selector_picks_only_diaries() -> None:
    selector = _selector()
    for rel, expected in _TREE:
        path = Path(rel)
        # DiarySelector.is_diary использует .name/.parts.
        assert selector.is_diary(path) is expected, f"mismatch for {rel}"


def test_select_diary_files_filters_tree() -> None:
    selector = _selector()
    paths = [Path(rel) for rel, _ in _TREE]
    picked = select_diary_files(paths, selector)
    expected = {rel for rel, exp in _TREE if exp}
    assert {p.as_posix() for p in picked} == expected


def test_explicit_diary_name_beats_exclude() -> None:
    """Файл с «дневник» в имени НЕ отсеивается, даже если совпал бы с exclude."""
    selector = _selector()
    # Имя содержит и «дневник», и «эпикриз» — приоритет у «дневник».
    p = Path("02_корпус/истории/Иванов И.И./дневник_и_эпикриз.docx")
    assert selector.is_diary(p) is True


def test_case_insensitive_dir_match() -> None:
    selector = _selector()
    p = Path("02_корпус/СБОРНИК_ДНЕВНИКОВ_ИБ/x.docx")
    assert selector.is_diary(p) is True


def test_configurable_exclude_can_be_extended() -> None:
    """Отбор расширяем через ENV: добавив тип в exclude — он отсеивается."""
    settings = Settings(corpus_exclude_name_re="консультаци")
    selector = DiarySelector.from_settings(settings)
    # «консультация» теперь отсеивается (нет «дневник» в имени)…
    assert selector.is_diary(
        Path("02_корпус/выписанные/А./консультация_невролога.docx")) is False
    # …а файл-дневник по-прежнему берётся.
    assert selector.is_diary(
        Path("02_корпус/выписанные/А./дневники.docx")) is True
