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
        pass


class FakeLLM:
    def __init__(self, *, raise_exc: Exception | None = None) -> None:
        self.raise_exc = raise_exc
        self.last_messages: list[LLMMessage] | None = None

    def generate(self, messages, *, temperature=None, max_tokens=None) -> LLMResult:
        self.last_messages = messages
        if self.raise_exc:
            raise self.raise_exc
        return LLMResult(content="СГЕНЕРИРОВАННЫЙ ДНЕВНИК [ДАТА]",
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

    Этап 7: syndrome теперь coded select — врач выбирает код (anxiety_depressive),
    который маппится в метку корпуса для фильтра retrieval (единые коды, docs/06).
    """
    retrieve = _fake_retrieve(
        [{"text": "образец", "syndrome": "тревожно-депрессивный"}])
    gen = DiaryGenerator(_settings(), anonymizer=FakeAnonymizer(), llm=FakeLLM(),
                         retrieve_fn=retrieve)
    answers = {
        "syndrome": "anxiety_depressive",
        "diagnosis": "F41.2 смешанное тревожное расстройство",
    }
    res = gen.generate(DOC_TYPE_EXAM_10D, answers)
    call = retrieve.calls[0]
    assert call["doc_type"] == DOC_TYPE_EXAM_10D
    assert call["syndrome"] == "тревожно-депрессивный"
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
    assert "Ежедневный дневник" in mapped.title_safe


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
    assert "ЭТАПНЫЙ ЭПИКРИЗ" in system
    assert "заведующим отделением" in system


def test_build_query_text_includes_syndrome() -> None:
    mapped = map_answers(DOC_TYPE_DAILY, {"mood": "lowered"})
    mapped.syndrome = "тревожно-депрессивный"
    q = build_query_text(mapped, DOC_TYPE_DAILY)
    assert "тревожно-депрессивный" in q
    assert "ежедневный" in q.lower()


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
    assert "консультация специалиста" in joined
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
        "syndrome": "anxiety_depressive",
        "comorbidities": ["r51"],
        "interventions": ["psychologist", "eeg"],
    })
    joined = " ".join(mapped.prompt_lines)
    assert "Мышление конкретное" in joined
    assert "Внимание и память снижены" in joined
    assert "низкой возрастной нормы" in joined
    assert "соглашательская" in joined
    assert "R51" in joined
    assert "консультация психолога" in joined and "ЭЭГ" in joined
    # syndrome-код смаппился в метку корпуса для фильтра.
    assert mapped.syndrome == "тревожно-депрессивный"


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
