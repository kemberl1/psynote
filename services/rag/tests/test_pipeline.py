"""Юнит-тесты конвейера генерации (Этап 4) с моками — без сети.

Проверяем:
  (а) свободный текст ввода ПРОХОДИТ анонимизацию ДО промпта/LLM (docs/04 §1);
  (б) гейт заблокировал ПДн → PiiBlockedError (→ 422);
  (в) сборка промпта для daily и exam_10d (структура шаблона + few-shot + ответы);
  (г) «все модели недоступны» пробрасывается из LLM в конвейер;
  (д) retrieval-сбой не роняет генерацию (few-shot=0);
  (е) маппинг ответов опросника → клинические формулировки + метаданные.

Запуск: cd services/rag && python -m pytest tests/test_pipeline.py -q
"""

from __future__ import annotations

import pytest

from app.anonymizer_client import AnonymizeResult
from app.config import Settings
from app.generation import build_messages, build_query_text
from app.llm_client import AllModelsUnavailableError, LLMMessage, LLMResult
from app.pipeline import DiaryGenerator, PiiBlockedError, UnsupportedDocTypeError
from app.questionnaire import iter_free_text, map_answers
from app.templates import DOC_TYPE_DAILY, DOC_TYPE_EXAM_10D


def _settings() -> Settings:
    return Settings(llm_api_key="test-key", retrieval_top_k=3)


# ─── Фейки компонентов (без сети) ───────────────────────────────────────────
class FakeAnonymizer:
    """Фейк гейта: записывает входы, заменяет ПДн плейсхолдером.

    `block` — множество подстрок: если текст содержит любую — гейт блокирует.
    """

    def __init__(self, *, block: set[str] | None = None) -> None:
        self.block = block or set()
        self.seen: list[str] = []
        self.closed = False

    def anonymize(self, text: str) -> AnonymizeResult:
        self.seen.append(text)
        if any(b in text for b in self.block):
            return AnonymizeResult(passed=False, reason="pii_detected")
        # Имитация: заменяем «Иванов» на плейсхолдер, считаем удаления.
        cleaned = text.replace("Иванов", "[ФИО]")
        removed = 1 if cleaned != text else 0
        return AnonymizeResult(passed=True, content=cleaned,
                               removed_count=removed, reason="ok")

    def close(self) -> None:
        self.closed = True


class FakeLLM:
    def __init__(self, *, raise_exc: Exception | None = None,
                 content: str = "СГЕНЕРИРОВАННЫЙ ДНЕВНИК [ДАТА]") -> None:
        self.raise_exc = raise_exc
        self.content = content
        self.last_messages: list[LLMMessage] | None = None

    def generate(self, messages, *, temperature=None, max_tokens=None) -> LLMResult:
        self.last_messages = messages
        if self.raise_exc:
            raise self.raise_exc
        return LLMResult(content=self.content,
                         model="deepseek-v4-flash",
                         usage={"total_tokens": 200})


def _fake_retrieve(samples):
    def _fn(query, doc_type=None, top_k=5, *, syndrome=None,
            diagnosis_class=None, section=None):
        _fn.calls.append({"query": query, "doc_type": doc_type, "top_k": top_k,
                          "syndrome": syndrome, "diagnosis_class": diagnosis_class})
        return samples
    _fn.calls = []
    return _fn


# ─── Тесты приватности ──────────────────────────────────────────────────────
def test_free_text_is_anonymized_before_llm() -> None:
    """Свободный текст с ПДн обезличивается ДО попадания в промпт LLM."""
    anon = FakeAnonymizer()
    llm = FakeLLM()
    gen = DiaryGenerator(_settings(), anonymizer=anon, llm=llm,
                         retrieve_fn=_fake_retrieve([]))
    answers = {
        "mood": "lowered",
        "complaints_detail": "Пациент Иванов жалуется на тревогу",
    }
    res = gen.generate(DOC_TYPE_DAILY, answers)

    # Гейт вызван на свободном поле.
    assert any("Иванов" in s for s in anon.seen)
    # В обезличенных ответах ПДн заменены.
    assert "[ФИО]" in res.answers_anonymized["complaints_detail"]
    assert "Иванов" not in res.answers_anonymized["complaints_detail"]
    # В промпт ушёл уже обезличенный текст.
    user_msg = next(m.content for m in llm.last_messages if m.role == "user")
    assert "Иванов" not in user_msg
    assert "[ФИО]" in user_msg
    assert res.anonymizer_removed_count == 1


def test_custom_select_value_anonymized() -> None:
    """Кастомное значение select {value:__custom__} проходит гейт."""
    anon = FakeAnonymizer()
    gen = DiaryGenerator(_settings(), anonymizer=anon, llm=FakeLLM(),
                         retrieve_fn=_fake_retrieve([]))
    answers = {"mood": {"value": "__custom__",
                        "custom_text": "пациент Иванов угрюм"}}
    res = gen.generate(DOC_TYPE_DAILY, answers)
    assert any("Иванов" in s for s in anon.seen)
    assert "Иванов" not in str(res.answers_anonymized)


def test_pii_blocked_raises() -> None:
    """Гейт заблокировал свободный текст → PiiBlockedError (→ 422)."""
    anon = FakeAnonymizer(block={"СЕКРЕТ"})
    gen = DiaryGenerator(_settings(), anonymizer=anon, llm=FakeLLM(),
                         retrieve_fn=_fake_retrieve([]))
    answers = {"complaints_detail": "СЕКРЕТ неустранимые ПДн"}
    with pytest.raises(PiiBlockedError):
        gen.generate(DOC_TYPE_DAILY, answers)


# ─── Тесты retrieval / фолбэка ──────────────────────────────────────────────
def test_retrieval_filters_passed() -> None:
    """В retrieve передаются doc_type и метаданные (syndrome/diagnosis_class).

    Этап 7: syndrome — coded select (anxious → «тревожный» для retrieval).
    """
    retrieve = _fake_retrieve(
        [{"text": "образец", "syndrome": "тревожный"}])
    gen = DiaryGenerator(_settings(), anonymizer=FakeAnonymizer(), llm=FakeLLM(),
                         retrieve_fn=retrieve)
    answers = {
        "syndrome": "anxious",
        "diagnosis": "F41.2 смешанное тревожное расстройство",
    }
    res = gen.generate(DOC_TYPE_EXAM_10D, answers)
    call = retrieve.calls[0]
    assert call["doc_type"] == DOC_TYPE_EXAM_10D
    assert call["syndrome"] == "тревожный"
    assert call["diagnosis_class"] == "F4x"
    assert call["top_k"] == 3
    assert res.chunks_used == 1


def test_retrieval_failure_does_not_break_generation() -> None:
    """Сбой retrieval → генерация продолжается с few-shot=0."""
    def broken(*a, **k):
        raise RuntimeError("qdrant down")
    gen = DiaryGenerator(_settings(), anonymizer=FakeAnonymizer(), llm=FakeLLM(),
                         retrieve_fn=broken)
    res = gen.generate(DOC_TYPE_DAILY, {"mood": "even"})
    assert res.chunks_used == 0
    assert res.content


def test_all_models_unavailable_propagates() -> None:
    """AllModelsUnavailableError из LLM пробрасывается в API-слой."""
    gen = DiaryGenerator(
        _settings(), anonymizer=FakeAnonymizer(),
        llm=FakeLLM(raise_exc=AllModelsUnavailableError("down")),
        retrieve_fn=_fake_retrieve([]))
    with pytest.raises(AllModelsUnavailableError):
        gen.generate(DOC_TYPE_DAILY, {"mood": "even"})


def test_unsupported_doc_type() -> None:
    gen = DiaryGenerator(_settings(), anonymizer=FakeAnonymizer(), llm=FakeLLM(),
                         retrieve_fn=_fake_retrieve([]))
    with pytest.raises(UnsupportedDocTypeError):
        gen.generate("unknown_type", {})


# ─── Тесты маппинга и сборки промптов ───────────────────────────────────────
def test_map_answers_daily_formulations() -> None:
    """Маппинг daily: select → клинические формулировки + метаданные динамики."""
    mapped = map_answers(DOC_TYPE_DAILY, {
        "dynamics": "no_change",
        "mood": "lowered",
        "mood_detail": ["anxiety", "tearfulness"],
        "sleep": "hard_to_fall_asleep",
        "appetite": "decreased",
    })
    joined = " ".join(mapped.prompt_lines)
    assert "без существенных изменений" in joined
    assert "Настроение снижено" in joined
    assert "тревога" in joined and "плаксивость" in joined
    assert mapped.dynamics == "без_динамики"
    assert "Ежедневный осмотр" in mapped.title_safe


def test_map_answers_patient_sex_grammar() -> None:
    mapped = map_answers(
        DOC_TYPE_DAILY, {"patient_sex": "female", "mood": "even"})
    joined = " ".join(mapped.prompt_lines)
    assert "девочка" in joined
    assert "упорядочена" in joined


def test_build_messages_daily_has_template_and_samples() -> None:
    """Промпт daily содержит каркас шаблона, few-shot и ответы."""
    mapped = map_answers(DOC_TYPE_DAILY, {"mood": "lowered"})
    samples = [
        {"text": "Настроение снижено, сон нарушен.", "syndrome": "тревожный"}]
    msgs = build_messages(DOC_TYPE_DAILY, mapped, samples)
    system = next(m.content for m in msgs if m.role == "system")
    user = next(m.content for m in msgs if m.role == "user")
    # Системная инструкция + структура шаблона daily.
    assert "психиатр" in system.lower()
    assert "Психический статус:" in system
    assert "ЕЖЕДНЕВНЫЙ" in system
    # Few-shot образец и ответы в user.
    assert "Настроение снижено, сон нарушен." in user
    assert "ОБРАЗЦЫ ИЗ КОРПУСА" in user
    assert "ОТВЕТЫ ОПРОСНИКА" in user


def test_build_messages_daily_includes_day_brief() -> None:
    mapped = map_answers(DOC_TYPE_DAILY, {
        "mood": "unstable",
        "__arc_context__": (
            "День в выбранном периоде: 1 из 12.\n"
            "СЕГОДНЯ опиши через наблюдения врача ТОЛЬКО это:\n"
            "• стаскивает простыни"
        ),
    })
    msgs = build_messages(DOC_TYPE_DAILY, mapped, [])
    system = next(m.content for m in msgs if m.role == "system")
    assert "БРИФ ЭТОГО ДНЯ" in system
    assert "стаскивает простыни" in system
    assert "СРЕЗ ДНЯ" in system
    assert "ЕЖЕДНЕВНЫЙ" in system


def test_build_messages_daily_locks_diagnosis_and_plain_markup() -> None:
    mapped = map_answers(DOC_TYPE_DAILY, {
        "mood": "even",
        "diagnosis": "F71.18 Умственная отсталость умеренная",
    })
    msgs = build_messages(DOC_TYPE_DAILY, mapped, [])
    system = next(m.content for m in msgs if m.role == "system")
    assert "F71.18" in "\n".join(mapped.prompt_lines)
    assert "не выдумывай" in system.lower()
    assert "Сознание" in system
    assert "звукокомплекс" in system.lower()
    assert "не противоречь" in system.lower() or "не противоречь" in system
    assert "под наблюдением" in system.lower()
    assert "вероятно" in system.lower()
    assert "Соматический статус:" in system
    assert "План лечения (дополнения к плану):" in system
    assert "Обоснование диагноза (при наличии дополнительных сведений):" in system


def test_build_messages_samples_are_style_not_foreign_diagnosis() -> None:
    mapped = map_answers(DOC_TYPE_DAILY, {
        "mood": "even",
        "diagnosis": "F92.8 смешанное расстройство поведения и эмоций",
    })
    samples = [
        {"text": "Интеллект соответствует умеренной умственной отсталости."}]
    msgs = build_messages(DOC_TYPE_DAILY, mapped, samples)
    system = next(m.content for m in msgs if m.role == "system")
    user = next(m.content for m in msgs if m.role == "user")
    lower = system.lower()
    assert "если умственной отсталости в диагнозе нет" in lower
    assert "не копируй из образцов" in lower or "не копируй" in lower
    assert "другие пациенты" in user.lower() or "других пациент" in user.lower()
    assert "чужой диагноз" in user.lower() or "не чужой диагноз" in user.lower()
    assert "план лечения" in user.lower()
    assert "см. лист назначений" in user.lower()


def test_build_messages_allows_grounded_coloring_not_pure_invention() -> None:
    mapped = map_answers(DOC_TYPE_DAILY, {"mood": "even"})
    msgs = build_messages(DOC_TYPE_DAILY, mapped, [])
    system = next(m.content for m in msgs if m.role == "system")
    lower = system.lower()
    assert "дорисовать быт" in lower
    assert "за период выходных дней" in lower
    assert "понедельник после пропущенных" in lower or "после пропущенных сб" in lower
    assert "без агрессивных, аутоагрессивных" in lower
    assert "каркас" in system.lower() or "шаблон" in system.lower()
    user = next(m.content for m in msgs if m.role == "user")
    assert "палата" in user.lower() or "игровая" in user.lower()


def test_build_messages_exam10d_has_epicrisis() -> None:
    """Промпт exam_10d отличается: есть этапный эпикриз и совместный осмотр."""
    mapped = map_answers(DOC_TYPE_EXAM_10D, {
        "mood": "even",
        "period_dynamics": "improvement",
        "syndrome": "тревожно-депрессивный",
    })
    msgs = build_messages(DOC_TYPE_EXAM_10D, mapped, [])
    system = next(m.content for m in msgs if m.role == "system")
    assert "Этапный эпикриз:" in system
    assert "заведующим отделением" in system
    assert "ОСМОТР\nлечащим врачом совместно с заведующим отделением" in system
    assert "Психический статус (его изменение):" in system
    assert "Синдром:" in system


def test_build_query_text_includes_syndrome() -> None:
    mapped = map_answers(DOC_TYPE_DAILY, {"mood": "lowered"})
    mapped.syndrome = "тревожно-депрессивный"
    q = build_query_text(mapped, DOC_TYPE_DAILY)
    assert "тревожно-депрессивный" in q
    assert "ежедневный" in q.lower()
    assert "психический статус" in q.lower()


def test_agitation_alone_does_not_ask_for_fixation() -> None:
    """Возбуждение без слова врача о фиксации — фиксацию не ищем и не пишем."""
    from app.generation import query_mentions_fixation
    mapped = map_answers(DOC_TYPE_DAILY, {
        "mood": "unstable",
        "__arc_context__": (
            "День госпитализации: 12.\n"
            "СЕГОДНЯ опиши через наблюдения врача ТОЛЬКО это:\n"
            "• вербальной коррекции не поддавался, кричал, замахивался\n"
            "ЗАПРЕЩЕНО: физическое удержание. ГРАМОТНО: мягкая фиксация.\n"
        ),
    })
    assert query_mentions_fixation(mapped) is False
    q = build_query_text(mapped, DOC_TYPE_DAILY)
    assert "вербальной коррекции не поддавался" in q
    assert "мягкая фиксация" not in q
    assert "ЗАПРЕЩЕНО" not in q


def test_doctor_fixation_uses_fixation_style() -> None:
    from app.generation import query_mentions_fixation
    mapped = map_answers(DOC_TYPE_DAILY, {
        "mood": "unstable",
        "__arc_context__": (
            "СЕГОДНЯ опиши через наблюдения врача ТОЛЬКО это:\n"
            "• кричал, замахивался, применена мягкая фиксация на 15 минут\n"
        ),
    })
    assert query_mentions_fixation(mapped) is True
    assert "мягкая фиксация" in build_query_text(mapped, DOC_TYPE_DAILY)


def test_quiet_day_query_does_not_ask_for_fixation() -> None:
    from app.generation import query_mentions_fixation
    mapped = map_answers(DOC_TYPE_DAILY, {
        "mood": "even",
        "__arc_context__": (
            "СЕГОДНЯ опиши через наблюдения врача ТОЛЬКО это:\n"
            "• в игровой смотрел телевизор, замечаний не получал\n"
            "ЗАПРЕЩЕНО: физическое удержание. ГРАМОТНО: мягкая фиксация.\n"
        ),
    })
    assert query_mentions_fixation(mapped) is False
    q = build_query_text(mapped, DOC_TYPE_DAILY)
    assert "телевизор" in q
    assert "мягкая фиксация" not in q


def test_field_behavior_query_does_not_ask_for_fixation() -> None:
    from app.generation import query_mentions_fixation
    mapped = map_answers(DOC_TYPE_DAILY, {
        "mood": "even",
        "__arc_context__": (
            "СЕГОДНЯ опиши через наблюдения врача ТОЛЬКО это:\n"
            "• полевое поведение, ходил по палате, расторможен\n"
            "ЗАПРЕЩЕНО: физическое удержание. ГРАМОТНО: мягкая фиксация.\n"
        ),
    })
    assert query_mentions_fixation(mapped) is False
    q = build_query_text(mapped, DOC_TYPE_DAILY)
    assert "полевое" in q
    assert "мягкая фиксация" not in q


def test_hard_verbal_correction_query_does_not_ask_for_fixation() -> None:
    from app.generation import query_mentions_fixation
    mapped = map_answers(DOC_TYPE_DAILY, {
        "mood": "unstable",
        "__arc_context__": (
            "СЕГОДНЯ опиши через наблюдения врача ТОЛЬКО это:\n"
            "• вербальной коррекции поддаётся с трудом, на замечания реагирует "
            "непродолжительно, разрушает игрушки\n"
            "ЗАПРЕЩЕНО: физическое удержание. ГРАМОТНО: мягкая фиксация.\n"
        ),
    })
    assert query_mentions_fixation(mapped) is False
    q = build_query_text(mapped, DOC_TYPE_DAILY)
    assert "мягкая фиксация" not in q


def test_agitation_retrieve_is_single_call() -> None:
    retrieve = _fake_retrieve([{"text": "Сознание ясное. Смотрел телевизор."}])
    gen = DiaryGenerator(
        _settings(), anonymizer=FakeAnonymizer(), llm=FakeLLM(),
        retrieve_fn=retrieve)
    gen.generate(DOC_TYPE_DAILY, {
        "mood": "unstable",
        "__arc_context__": (
            "СЕГОДНЯ опиши через наблюдения врача ТОЛЬКО это:\n"
            "• вербальной коррекции не поддавался, кричал\n"
        ),
    })
    assert len(retrieve.calls) == 1


# ─── Этап 7: новые/изменённые вопросы дерева docs/06 ────────────────────────
def test_map_daily_new_conditional_multiselects() -> None:
    """Новые условные multiselect daily (sleep_detail, events, behavior_detail)."""
    mapped = map_answers(DOC_TYPE_DAILY, {
        "behavior": "violates",
        "behavior_detail": ["conflict", "aggression"],
        "sleep": "superficial",
        "sleep_detail": ["frequent_awakenings", "no_rest"],
        "events": ["consultation", "examination"],
    })
    joined = " ".join(mapped.prompt_lines)
    assert "конфликтность" in joined and "агрессивные проявления" in joined
    assert "частые пробуждения" in joined
    assert "выполнено обследование" in joined


def test_map_multiselect_custom_item_in_prompt() -> None:
    """«Свой вариант» внутри multiselect разворачивается в промпт-строку."""
    mapped = map_answers(DOC_TYPE_DAILY, {
        "mood": "lowered",
        "mood_detail": [
            "anxiety",
            {"value": "__custom__", "custom_text": "чувство опустошённости"},
        ],
    })
    joined = " ".join(mapped.prompt_lines)
    assert "тревога" in joined
    assert "чувство опустошённости" in joined


def test_map_multiselect_multiple_custom_items_in_prompt() -> None:
    """Этап 7.1: НЕСКОЛЬКО «своих вариантов» в multiselect — все в промпт-строке.

    Фронт теперь позволяет добавить серию кастом-чипов; контракт сериализует их
    как несколько объектов __custom__ в массиве. Маппинг обязан развернуть КАЖДЫЙ
    в общую формулировку (вместе со стандартными кодами), без потерь.
    """
    mapped = map_answers(DOC_TYPE_DAILY, {
        "mood": "lowered",
        "mood_detail": [
            "anxiety",
            {"value": "__custom__", "custom_text": "беспокойство"},
            {"value": "__custom__", "custom_text": "апатия"},
        ],
    })
    joined = " ".join(mapped.prompt_lines)
    assert "тревога" in joined
    assert "беспокойство" in joined
    assert "апатия" in joined


def test_iter_free_text_covers_multiple_multiselect_customs() -> None:
    """Этап 7.1: iter_free_text извлекает КАЖДЫЙ кастом multiselect по индексу.

    Гарантирует, что все несколько кастом-элементов попадают на анонимайзер-гейт
    (каждый по своему пути qid[idx].custom_text) — приватность сохранена.
    """
    answers = {
        "mood_detail": [
            "anxiety",
            {"value": "__custom__", "custom_text": "беспокойство"},
            {"value": "__custom__", "custom_text": "апатия"},
        ],
    }
    found = dict(iter_free_text(DOC_TYPE_DAILY, answers))
    assert found["mood_detail[1].custom_text"] == "беспокойство"
    assert found["mood_detail[2].custom_text"] == "апатия"


def test_map_exam_psych_status_and_syndrome_select() -> None:
    """Психический статус осмотра (мышление/внимание/интеллект) + syndrome-select.

    syndrome теперь select: код → метаданное для retrieval-фильтра.
    """
    mapped = map_answers(DOC_TYPE_EXAM_10D, {
        "thinking": "concrete",
        "attention_memory": "reduced",
        "intellect": "low_norm",
        "criticism": "conciliatory",
        "syndrome": "anxious",
        "comorbidities": ["r51"],
        "interventions": ["eeg", "lab"],
    })
    joined = " ".join(mapped.prompt_lines)
    assert "Мышление конкретное" in joined
    assert "Внимание и память снижены" in joined
    assert "низкой возрастной нормы" in joined
    assert "соглашательская" in joined
    assert "R51" in joined
    assert "ЭЭГ" in joined and "лабораторное обследование" in joined
    assert mapped.syndrome == "тревожный"


def test_map_exam_discharge_freetext() -> None:
    """Boolean discharge + условный discharge_detail (свободный текст в промпт)."""
    mapped = map_answers(DOC_TYPE_EXAM_10D, {
        "period_dynamics": "improvement",
        "discharge_detail": "Рекомендовано наблюдение по месту жительства",
    })
    joined = " ".join(mapped.prompt_lines)
    assert "наблюдение по месту жительства" in joined


def test_iter_free_text_covers_multiselect_custom() -> None:
    """iter_free_text извлекает кастом-элементы multiselect и detail-поля."""
    answers = {
        "mood_detail": [
            "anxiety",
            {"value": "__custom__", "custom_text": "своя формулировка"},
        ],
        "complaints_detail": "болит голова",
        "events": [{"value": "__custom__", "custom_text": "перевод в палату"}],
    }
    found = dict(iter_free_text(DOC_TYPE_DAILY, answers))
    texts = set(found.values())
    assert "своя формулировка" in texts
    assert "болит голова" in texts
    assert "перевод в палату" in texts


def test_syndrome_custom_lowercased_metadata() -> None:
    """«Свой вариант» синдрома идёт в метаданное retrieval как lowercase."""
    mapped = map_answers(DOC_TYPE_EXAM_10D, {
        "syndrome": {"value": "__custom__", "custom_text": "Смешанный Синдром"},
    })
    assert mapped.syndrome == "смешанный синдром"
    assert any("Смешанный Синдром" in line for line in mapped.prompt_lines)


def test_build_messages_redacts_foreign_drugs_in_samples() -> None:
    mapped = map_answers(
        DOC_TYPE_DAILY, {"mood": "even", "diagnosis": "F72.14"})
    samples = [{
        "text": (
            "Психический статус: расторможен, кричит. "
            "Целесообразно назначить мягкую фиксацию конечностей. "
            "Назначения: р-р Перициазина 4% 4 кап вечером."
        ),
    }]
    msgs = build_messages(DOC_TYPE_DAILY, mapped, samples)
    user = next(m.content for m in msgs if m.role == "user")
    system = next(m.content for m in msgs if m.role == "system")
    assert "перициазин" not in user.lower()
    assert "мягкую фиксацию" in user.lower()
    assert "физическое удержание" in system.lower()
    assert "мягкая фиксация" in system.lower()
    assert "коррекция терапии" in user.lower() or "план лечения" in user.lower()


def test_generate_strips_restraint_and_foreign_drug() -> None:
    llm = FakeLLM(content=(
        "Психический статус: Вербальной коррекции не поддавался. "
        "В такие моменты требуется физическое удержание и помощь персонала.\n"
        "Назначения: р-р перициазина 4% по 1-2-2 капли\n"
        "План лечения (дополнения к плану): без дополнений\n"
    ))
    gen = DiaryGenerator(
        _settings(), anonymizer=FakeAnonymizer(), llm=llm,
        retrieve_fn=_fake_retrieve([]),
    )
    res = gen.generate(DOC_TYPE_DAILY, {
        "mood": "unstable",
        "diagnosis": "F72.14",
        "key_medications": "рисперидон 1 мг/сут",
    })
    assert "физическое удержание" not in res.content.lower()
    assert "мягкая фиксация" in res.content.lower()
    assert "перициазин" not in res.content.lower()
    assert "см. лист назначений" in res.content


def test_style_excerpt_takes_mental_status_not_old_header() -> None:
    from app.generation import style_excerpt
    sample = (
        "13.08.2026 время: 10:45\nЖалобы: не предъявляет.\n"
        + "Анамнез заболевания: без дополнений. " * 30
        + "\nПсихический статус: Сознание ясное. В игровой строит из кубиков. "
        "Фон настроения ровный.\nСоматический статус: без особенностей."
    )
    out = style_excerpt(sample)
    assert out.startswith("Психический статус:")
    assert "строит из кубиков" in out
    assert "Анамнез" not in out and "Соматический" not in out


def test_style_excerpt_cuts_long_status_on_sentence() -> None:
    from app.generation import style_excerpt
    out = style_excerpt("Психический статус: " + "Сознание ясное. " * 100, limit=200)
    assert len(out) <= 200 and out.endswith(".")


def test_generator_is_reused_between_requests() -> None:
    """Дни пакета должны идти по уже открытому соединению к провайдеру."""
    try:
        from app import main
    except (ImportError, TypeError) as exc:  # локальный Python 3.9: FastAPI-модели
        pytest.skip(f"app.main недоступен в этом интерпретаторе: {exc}")
    assert main.get_generator() is main.get_generator()


def test_llm_client_and_anonymizer_survive_between_generations() -> None:
    """Клиент провайдера переиспользуется и не закрывается после дня пакета."""
    anon = FakeAnonymizer()
    llm = FakeLLM()
    gen = DiaryGenerator(_settings(), anonymizer=anon, llm=llm,
                         retrieve_fn=_fake_retrieve([]))
    gen.generate(DOC_TYPE_DAILY, {"mood": "even"})
    first = gen._get_llm()
    gen.generate(DOC_TYPE_DAILY, {"mood": "even"})
    assert gen._get_llm() is first
    assert anon.closed is False
