"""Постобработка дневника: назначения, план лечения, английские утечки."""

from app.postprocess import polish_diary, sanitize_corpus_sample


_SAMPLE = """Осмотр лечащим врачом
Дата: [ДАТА] [ВРЕМЯ]

Жалобы: не предъявляет
Психический статус: После завтрака отмечалось нарастание двигательного беспокойства: постоянно стремился к движению, залезал на кровати, прыгал с них. При попытке остановить начинал кричать, замахиваться на окружающих, вербальной коррекции не поддавался, самостоятельно успокоиться не мог. В связи с возбуждением с седативной целью выполнена инъекция р-ра Хлорпромазина 2,5%-1 мл в/м, после чего в течение 15 минут успокоился. В дальнейшем в течение дня без эпизодов грубой психомоторной расторможенности. Фон настроения неустойчивый, в течение дня отмечаются колебания от ровного до mildly раздражительного. На оклики реагирует непродолжительно.
Назначения: в связи с неустойчивостью состояния выполнена инъекция р-ра Хлорпромазина 2,5% — 2 мл/сут
Выполнены медицинские вмешательства: осмотр врачом-психиатром детским
План обследования (дополнения к плану): без дополнений
План лечения (дополнения к плану): без дополнений
"""


def test_polish_moves_injection_to_plan_and_keeps_behavior() -> None:
    out = polish_diary(_SAMPLE)
    assert "Назначения: см. лист назначений" in out
    plan = next(
        line for line in out.splitlines()
        if line.startswith("План лечения")
    )
    assert "инъекц" in plan.lower()
    assert "Хлорпромазина" in plan
    status = next(
        line for line in out.splitlines()
        if line.startswith("Психический статус")
    )
    assert "залезал на кровати" in status
    assert "инъекц" not in status.lower()
    assert "Хлорпромазина" not in status


def test_polish_strips_english_and_ocliks() -> None:
    out = polish_diary(_SAMPLE)
    assert "mildly" not in out
    assert "слегка раздражительного" in out
    assert "на оклики" not in out.lower()
    assert "на замечания" in out.lower()


def test_polish_keeps_see_list_and_empty_plan() -> None:
    src = (
        "Психический статус: Сознание ясное. Ориентирован верно.\n"
        "Назначения: см. лист назначений\n"
        "План лечения (дополнения к плану): без дополнений\n"
    )
    assert polish_diary(src) == src


def test_polish_rewrites_illegal_hold_to_soft_fixation() -> None:
    src = (
        "Психический статус: Вербальной коррекции не поддавался. "
        "В такие моменты требуется физическое удержание и помощь персонала. "
        "Фон настроения неустойчивый.\n"
        "Назначения: см. лист назначений\n"
        "План лечения (дополнения к плану): без дополнений\n"
    )
    out = polish_diary(src)
    assert "физическое удержание" not in out.lower()
    assert "мягкая фиксация" in out.lower()
    assert "вербальной коррекции не поддавался" in out.lower()
    assert "фон настроения неустойчивый" in out.lower()


def test_polish_keeps_hygiene_staff_help() -> None:
    src = (
        "Психический статус: Одевается, гигиенические мероприятия "
        "выполняет с помощью персонала.\n"
        "Назначения: см. лист назначений\n"
        "План лечения (дополнения к плану): без дополнений\n"
    )
    out = polish_diary(src)
    assert "с помощью персонала" in out


def test_polish_drops_standing_regimen_from_assignments() -> None:
    src = (
        "Психический статус: Сознание ясное.\n"
        "Назначения: таб. Рисперидон 1 мг утром, р-р перициазина 4% по 1-2-2 капли\n"
        "План лечения (дополнения к плану): без дополнений\n"
    )
    out = polish_diary(src, allowed_drugs={"рисперидон"})
    assert "Назначения: см. лист назначений" in out
    plan = next(line for line in out.splitlines()
                if line.startswith("План лечения"))
    assert "перициазин" not in plan.lower()
    assert "рисперидон" not in plan.lower()
    assert "без дополнений" in plan


def test_polish_strips_foreign_periciazine() -> None:
    src = (
        "Психический статус: Сознание ясное. В связи с возбуждением "
        "выполнена инъекция р-ра перициазина 4%.\n"
        "Назначения: см. лист назначений\n"
        "План лечения (дополнения к плану): без дополнений\n"
    )
    out = polish_diary(src, allowed_drugs={"рисперидон"})
    assert "перициазин" not in out.lower()
    status = next(line for line in out.splitlines()
                  if line.startswith("Психический статус"))
    assert "инъекц" not in status.lower()


def test_sanitize_sample_keeps_soft_fixation_redacts_foreign_drugs() -> None:
    raw = (
        "Психический статус: расторможен. Целесообразно назначить мягкую "
        "фиксацию конечностей на 20 минут. План лечения: р-р Перициазина 4% "
        "4 кап вечером. Мать забрала из интерната. В приёмном покое метался. "
        "Доставлен сантранспортом в [УЧРЕЖДЕНИЕ], [НОМЕР_ДОКУМЕНТА]."
    )
    out = sanitize_corpus_sample(raw)
    assert "перициазин" not in out.lower()
    assert "фиксац" in out.lower()
    assert "интернат" not in out.lower()
    assert "приёмном" not in out.lower() and "приемном" not in out.lower()
    assert "сантранспорт" not in out.lower()
    assert "[УЧРЕЖДЕНИЕ]" not in out
    assert "[НОМЕР_ДОКУМЕНТА]" not in out
    assert "препарат другого пациента" in out


def test_polish_strips_admission_from_exam_anamnesis() -> None:
    src = (
        "Анамнез жизни (дополнения к анамнезу): переведён в ДДИ, "
        "мать забрала ребёнка из интерната. Выдано направление на госпитализацию.\n"
        "Психический статус: В приёмном покое метался. В отделении обособлен.\n"
        "Дополнительные сведения о заболевании: доставлен сантранспортом "
        "в приемное отделение. Одевается с помощью персонала.\n"
        "Назначения: см. лист назначений\n"
        "План лечения (дополнения к плану): без дополнений\n"
    )
    out = polish_diary(src, lock_anamnesis=True, lock_additional_none=True)
    assert "интернат" not in out.lower()
    assert "приёмном" not in out.lower() and "приемном" not in out.lower()
    assert "сантранспорт" not in out.lower()
    anamnesis = next(
        line for line in out.splitlines()
        if line.startswith("Анамнез жизни")
    )
    assert "без дополнений" in anamnesis
    extra = next(
        line for line in out.splitlines()
        if line.startswith("Дополнительные сведения")
    )
    assert extra.endswith("нет") or extra.endswith("нет.")


def test_polish_drops_injection_when_calmed_after_fixation() -> None:
    src = (
        "Психический статус: Вербальной коррекции не поддавался. "
        "Применена мягкая фиксация конечностей на 20 минут "
        "под контролем медперсонала; после фиксации успокоился.\n"
        "Назначения: см. лист назначений\n"
        "План лечения (дополнения к плану): в связи с возбуждением "
        "выполнена инъекция р-ра Хлорпромазина 2,5% — 1 мл в/м.\n"
    )
    out = polish_diary(src)
    plan = next(line for line in out.splitlines() if line.startswith("План лечения"))
    assert "инъекц" not in plan.lower()
    assert "без дополнений" in plan
    status = next(
        line for line in out.splitlines() if line.startswith("Психический статус")
    )
    assert "успокоился" in status.lower()


def test_polish_keeps_injection_if_not_calmed_after_fixation() -> None:
    src = (
        "Психический статус: Применена мягкая фиксация конечностей; "
        "после фиксации успокоился непродолжительно, вновь становился возбудимым.\n"
        "Назначения: см. лист назначений\n"
        "План лечения (дополнения к плану): выполнена инъекция р-ра "
        "Хлорпромазина 2,5% — 1 мл в/м.\n"
    )
    out = polish_diary(src)
    plan = next(line for line in out.splitlines() if line.startswith("План лечения"))
    assert "инъекц" in plan.lower()


def test_polish_rewrites_hold_attempt_without_inventing_fixation() -> None:
    src = (
        "Психический статус: При попытке удержания разбегался. "
        "Отводится только при удержании за руку.\n"
        "Назначения: см. лист назначений\n"
        "План лечения (дополнения к плану): без дополнений\n"
    )
    out = polish_diary(src)
    assert "удержания" not in out.lower()
    assert "удержании за руку" not in out.lower()
    assert "при попытке остановить" in out.lower()
    assert "если взять за руку" in out.lower()
    assert "мягкая фиксация" not in out.lower()


def test_okliki_are_rewritten_in_any_form() -> None:
    """Врач просил не писать «оклики» — ни в одной форме."""
    src = (
        "Психический статус: на оклики реагирует непродолжительно. "
        "После оклика возвращался к занятию. При оклике поворачивает голову.\n"
        "План лечения (дополнения к плану): без дополнений"
    )
    out = polish_diary(src)
    assert "оклик" not in out.lower()
    assert "на замечания реагирует" in out
    assert "После замечание" not in out
