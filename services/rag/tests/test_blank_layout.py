"""Инвариант бланка: постобработка не склеивает и не удаляет строки МИС.

Регрессия (прод, 16.09): `\\s{2,}` в постобработке сливал «\\n\\n» и
« \\n» в пробел — «Дата: … Жалобы:», «…без дополнений Физикальное
исследование…», «…симптоматики Диагноз:». С lock_anamnesis склеенная
строка анамнеза переписывалась целиком и заголовок «Физикальное
исследование…» пропадал.
"""

import pytest

from app.postprocess import polish_diary
from app.templates import get_template

# Синтетический бланк в формате ответа модели + пробелы в конце строк.
_DAILY = """Осмотр лечащим врачом 
Дата: [ДАТА] [ВРЕМЯ]

Жалобы: не предъявляет 
Анамнез заболевания (дополнения к анамнезу): без дополнений
Анамнез жизни (дополнения к анамнезу): без дополнений 

Физикальное исследование, локальный статус (его изменение): 
Психический статус: Сознание ясное. Ориентирован частично. Зрительный контакт непродолжительный. В игровой перебирает кубики. Фон настроения ровный. Критика не выявляется. 
Соматический статус: Кожа чистая. Зев спокоен. Живот мягкий.
Неврологический статус: без острой неврологической симптоматики 

Диагноз:
Основное заболевание: F00.00
Сопутствующие заболевания: не выявлено
Дополнительные сведения о заболевании: нет
Обоснование диагноза (при наличии дополнительных сведений): не требуется
Назначения: дозировка препарата скорректирована до 5 мг/сут
Выполнены медицинские вмешательства: осмотр врачом-психиатром детским
План обследования (дополнения к плану): без дополнений
План лечения (дополнения к плану): без дополнений 

[ДОЛЖНОСТЬ_ВРАЧА] [ФИО_ВРАЧА]"""

_EXAM = """ИБ №[НОМЕР_ИБ]
ОСМОТР
лечащим врачом совместно с заведующим отделением
[ДАТА] время: [ВРЕМЯ]

Жалобы: не предъявляет
Анамнез заболевания (дополнения к анамнезу): без дополнений
Анамнез жизни (дополнения к анамнезу): без дополнений
Физикальное исследование, локальный статус (его изменение): кожа чистая.
Неврологический статус (его изменение): без острой неврологической симптоматики
Психический статус (его изменение): Сознание ясное. Ориентирован верно. Фон настроения ровный.

Диагноз:
Основное заболевание: F00.00
Синдром: астенический
Сопутствующие заболевания: не выявлено
Дополнительные сведения о заболевании: нет
Обоснование диагноза (при наличии дополнительных сведений): не требуется
Назначения: см. лист назначений
Выполнены медицинские вмешательства: осмотр врачом-психиатром детским
План обследования (дополнения к плану): без дополнений
План лечения (дополнения к плану): без дополнений
Этапный эпикриз: за период состояние стабильное.

Фамилия, имя, отчество (при наличии) врача, должность, специальность, подпись
[ФИО_ВРАЧА], [ДОЛЖНОСТЬ_ВРАЧА]"""


def _line_key(line: str) -> str:
    s = line.strip().replace("**", "")
    return s.split(":", 1)[0].strip() if ":" in s else s


def _skeleton_keys(doc_type: str) -> list[str]:
    keys = []
    for line in get_template(doc_type).render_skeleton().splitlines():
        if line.strip() and ":" in line:
            keys.append(_line_key(line))
    return keys


def _blank_before(doc_type: str) -> list[str]:
    lines = get_template(doc_type).render_skeleton().splitlines()
    return [
        _line_key(lines[i]) for i in range(1, len(lines))
        if lines[i].strip() and not lines[i - 1].strip()
        and ":" in lines[i]
    ]


def _assert_blank_intact(out: str, doc_type: str) -> None:
    lines = out.splitlines()
    starts = [_line_key(line) for line in lines]
    # Каждая метка шаблона — в начале своей строки, в порядке шаблона.
    positions = []
    for key in _skeleton_keys(doc_type):
        assert key in starts, f"метка «{key}» пропала или склеена:\n{out}"
        positions.append(starts.index(key))
    assert positions == sorted(positions), f"порядок строк нарушен:\n{out}"
    # Ни одна строка не несёт две метки бланка.
    for line in lines:
        inner = [k for k in _skeleton_keys(doc_type)
                 if f" {k}:" in line.replace("**", "")]
        assert not inner, f"склеенная строка: {line!r}"
    # Пустые строки бланка на месте.
    for key in _blank_before(doc_type):
        i = starts.index(key)
        assert i > 0 and not lines[i - 1].strip(), (
            f"нет пустой строки перед «{key}»:\n{out}")


@pytest.mark.parametrize("doc_type,src", [
    ("daily", _DAILY),
    ("exam_10d", _EXAM),
])
@pytest.mark.parametrize("locks", [False, True])
def test_polish_keeps_every_blank_line(doc_type, src, locks) -> None:
    out = polish_diary(
        src, allowed_drugs=set(), doc_type=doc_type,
        lock_anamnesis=locks, lock_additional_none=locks,
    )
    _assert_blank_intact(out, doc_type)
    assert "Физикальное исследование, локальный статус (его изменение):" in out


@pytest.mark.parametrize("doc_type,src", [
    ("daily", _DAILY),
    ("exam_10d", _EXAM),
])
def test_polish_is_idempotent(doc_type, src) -> None:
    kw = dict(allowed_drugs=set(), doc_type=doc_type,
              lock_anamnesis=True, lock_additional_none=True)
    once = polish_diary(src, **kw)
    assert polish_diary(once, **kw) == once


def test_polish_splits_lines_merged_by_model() -> None:
    merged = (
        _DAILY
        .replace("[ВРЕМЯ]\n\nЖалобы", "[ВРЕМЯ] Жалобы")
        .replace("без дополнений \n\nФизикальное", "без дополнений Физикальное")
        .replace("симптоматики \n\nДиагноз", "симптоматики Диагноз")
        .replace("без дополнений \n\n[ДОЛЖНОСТЬ", "без дополнений [ДОЛЖНОСТЬ")
    )
    out = polish_diary(merged, allowed_drugs=set(), doc_type="daily",
                       lock_anamnesis=True, lock_additional_none=True)
    _assert_blank_intact(out, "daily")
    assert out.rstrip().endswith("[ДОЛЖНОСТЬ_ВРАЧА] [ФИО_ВРАЧА]")


def test_bold_label_keeps_closing_marker() -> None:
    src = _DAILY.replace(
        "Назначения: дозировка",
        "**Назначения:** дозировка",
    )
    out = polish_diary(src, allowed_drugs=set(), doc_type="daily")
    line = next(x for x in out.splitlines() if "Назначения" in x)
    assert line == "**Назначения:** см. лист назначений"


def test_polish_still_drops_english_leak_without_joining_lines() -> None:
    src = _DAILY.replace("Критика не выявляется.", "Критика mildly снижена.")
    out = polish_diary(src, allowed_drugs=set(), doc_type="daily")
    assert "mildly" not in out
    _assert_blank_intact(out, "daily")


def test_daily_does_not_split_on_exam_only_labels() -> None:
    src = _DAILY.replace(
        "Основное заболевание: F00.00",
        "Основное заболевание: F00.00 Синдром: астенический",
    )
    out = polish_diary(src, allowed_drugs=set(), doc_type="daily")
    assert "Основное заболевание: F00.00 Синдром: астенический" in out


def _additional(out: str) -> str:
    return next(x for x in out.splitlines()
                if x.startswith("Дополнительные сведения о заболевании:"))


@pytest.mark.parametrize("model_value", [
    "за период выходных дней с [ДАТА] под наблюдением дежурного мед персонала.",
    "за период выходных дней с 12-13.09 под наблюдением дежурного мед персонала.",
    "за период выходных дней с [ДАТА]-[ДАТА] под наблюдением дежурного мед персонала.",
])
def test_weekend_formula_gets_its_own_placeholder(model_value) -> None:
    """[ДАТА] при показе = дата осмотра; даты сб–вс — отдельный плейсхолдер."""
    src = _DAILY.replace(
        "Дополнительные сведения о заболевании: нет",
        f"Дополнительные сведения о заболевании: {model_value}",
    )
    out = polish_diary(src, allowed_drugs=set(), doc_type="daily",
                       expect_weekend=True)
    assert _additional(out) == (
        "Дополнительные сведения о заболевании: за период выходных дней "
        "с [ВЫХОДНЫЕ] под наблюдением дежурного мед персонала."
    )


def test_monday_formula_is_added_when_model_dropped_it() -> None:
    out = polish_diary(_DAILY, allowed_drugs=set(), doc_type="daily",
                       expect_weekend=True)
    assert "за период выходных дней с [ВЫХОДНЫЕ]" in _additional(out)


def test_weekday_keeps_none_without_formula() -> None:
    out = polish_diary(_DAILY, allowed_drugs=set(), doc_type="daily",
                       lock_additional_none=True)
    assert _additional(out) == "Дополнительные сведения о заболевании: нет"
