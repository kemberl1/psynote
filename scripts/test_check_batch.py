"""Тесты проверки пакета дневников (scripts/check_batch.py).

Запуск: python3 -m pytest scripts/test_check_batch.py -q
"""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from check_batch import check_batch  # noqa: E402

_GOOD_DAY = """Осмотр лечащим врачом
Дата: [ДАТА] [ВРЕМЯ]

Жалобы: не предъявляет
Анамнез заболевания (дополнения к анамнезу): без дополнений
Анамнез жизни (дополнения к анамнезу): без дополнений

Физикальное исследование, локальный статус (его изменение):
Психический статус: Сознание ясное. Ориентирован в месте, времени и собственной личности верно. {obs} Критика к своему состоянию формируется.
Соматический статус: Кожа чистая. Зев спокоен.
Неврологический статус: без острой неврологической симптоматики

Диагноз:
Основное заболевание: F00.00
Сопутствующие заболевания: не выявлено
Дополнительные сведения о заболевании: {additional}
Обоснование диагноза (при наличии дополнительных сведений): не требуется
Назначения: {prescriptions}
Выполнены медицинские вмешательства: осмотр врачом-психиатром детским
План обследования (дополнения к плану): без дополнений
План лечения (дополнения к плану): без дополнений

[ДОЛЖНОСТЬ_ВРАЧА] [ФИО_ВРАЧА]"""


def _day(title: str, *, obs: str, additional: str = "нет",
         prescriptions: str = "см. лист назначений") -> dict:
    return {
        "title_safe": title,
        "status": "done",
        "content": _GOOD_DAY.format(
            obs=obs, additional=additional, prescriptions=prescriptions),
    }


def _batch(children: list[dict]) -> dict:
    return {"title_safe": "тест", "children": children}


def test_clean_batch_has_no_problems() -> None:
    batch = _batch([
        _day("День 3 · 14.09.2026 · Ежедневный осмотр",
             obs="В игровой собирал конструктор, с детьми общался избирательно."),
        _day("День 4 · 15.09.2026 · Ежедневный осмотр",
             obs="На прогулке держался рядом с персоналом, отвечал на вопросы охотно."),
    ])
    assert check_batch(batch) == []


def test_flags_weekend_diary_and_misplaced_formula() -> None:
    # 19.09.2026 — суббота, 11-е сутки госпитализации.
    batch = _batch([
        _day("День 11 · 19.09.2026 · Ежедневный осмотр",
             obs="В игровой рисовал.",
             additional="за период выходных дней с [ВЫХОДНЫЕ] под наблюдением дежурного мед персонала."),
    ])
    problems = " ".join(check_batch(batch))
    assert "дневник за выходной" in problems
    assert "формула выходных не в понедельник" in problems


def test_flags_prescriptions_admission_history_and_drift() -> None:
    batch = _batch([
        _day("День 3 · 14.09.2026 · Ежедневный осмотр",
             obs="Доставлен в приёмный покой сантранспортом, держался напряжённо.",
             prescriptions="рисперидон 2 мг/сут"),
        _day("День 4 · 15.09.2026 · Ежедневный осмотр",
             obs="Рассказал, что были попытки суицида, наносил порезы."),
    ])
    problems = " ".join(check_batch(batch))
    assert "«Назначения» не по форме" in problems
    assert "анамнез поступления" in problems
    assert "суицидальный анамнез" in problems


def test_flags_copy_paste_between_days() -> None:
    same = "В игровой комнате собирал конструктор и наблюдал за другими детьми издалека."
    batch = _batch([
        _day("День 3 · 14.09.2026 · Ежедневный осмотр", obs=same),
        _day("День 4 · 15.09.2026 · Ежедневный осмотр", obs=same),
    ])
    assert any("дословно повторяет" in p for p in check_batch(batch))
