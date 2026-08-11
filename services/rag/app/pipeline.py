"""Оркестрация генерации дневника (Этап 4): гейт → retrieval → промпт → LLM.

Конвейер (docs/03 §6–10, docs/05 §2.4 статусы):
  1. Анонимизация свободного текста ответов через gateway-гейт (docs/04 §1).
     Свободные поля опросника могут содержать ПДн (врач вписал в «свой вариант»)
     — ДО отправки в LLM они ОБЯЗАНЫ пройти гейт. Если гейт заблокировал —
     PiiBlockedError (→ 422). Структурированные select-значения ПДн не несут.
  2. Маппинг ответов в клинические формулировки + метаданные (questionnaire.py).
  3. Retrieval few-shot образцов из Qdrant с фильтром по метаданным (docs/03 §6).
  4. Сборка промпта daily/exam_10d (generation.py).
  5. Вызов LLM (OpenAI-совместимый) с автофолбэком моделей (llm_client.py).
  6. Возврат обезличенного результата + метаданных (модель, токены, k).

ПРИВАТНОСТЬ:
  • Сырой свободный текст НЕ логируется и НЕ уходит в LLM/БД без анонимизации.
  • В ответе/истории — только обезличенный текст с плейсхолдерами (docs/05 §2.3).
  • DI: компоненты (anonymizer/retriever/llm) внедряемы — тесты мокают без сети.
"""

from __future__ import annotations

import copy
import logging
from dataclasses import dataclass, field
from typing import Callable, Protocol

from app.anonymizer_client import AnonymizerClient
from app.config import Settings
from app.generation import build_messages, build_query_text
from app.llm_client import LLMClient, LLMResult, OpenAICompatibleClient
from app.questionnaire import iter_free_text, map_answers
from app.templates import SUPPORTED_DOC_TYPES, DOC_TYPE_DAILY

logger = logging.getLogger(__name__)


class PipelineError(Exception):
    """Базовая ошибка конвейера генерации."""


class PiiBlockedError(PipelineError):
    """Гейт заблокировал свободный текст ввода — остаточные ПДн (→ 422)."""


class UnsupportedDocTypeError(PipelineError):
    """Неизвестный тип документа (→ 400)."""


# Сигнатура retrieve() для DI (по умолчанию — app.retrieval.retrieve).
class RetrieveFn(Protocol):
    def __call__(self, query: str, doc_type: str | None = None, top_k: int = 5,
                 *, syndrome: str | None = None,
                 diagnosis_class: str | None = None,
                 section: str | None = None) -> list[dict]: ...


@dataclass
class GenerationResult:
    """Итог генерации — только обезличенные данные + метаданные."""

    content: str
    model_used: str
    tokens_used: int
    chunks_used: int
    title_safe: str
    answers_anonymized: dict
    anonymizer_removed_count: int
    # Метаданные retrieval для аудита/истории.
    syndrome: str | None = None
    diagnosis_class: str | None = None
    dynamics: str | None = None


@dataclass
class _AnonymizedAnswers:
    answers: dict
    removed_count: int
    blocked_fields: list[str] = field(default_factory=list)


def _set_by_path(answers: dict, path: str, new_text: str) -> None:
    """Заменить значение свободного текста по «адресу» из iter_free_text.

    Поддерживает: 'qid', 'qid.custom_text', 'qid[idx].custom_text'.
    """
    if path.endswith(".custom_text") and "[" in path:
        # qid[idx].custom_text
        base = path[: path.index("[")]
        idx = int(path[path.index("[") + 1: path.index("]")])
        answers[base][idx]["custom_text"] = new_text
    elif path.endswith(".custom_text"):
        base = path[: -len(".custom_text")]
        answers[base]["custom_text"] = new_text
    else:
        answers[path] = new_text


class DiaryGenerator:
    """Оркестратор генерации. Компоненты внедряемы для тестов (без сети)."""

    def __init__(self, settings: Settings, *,
                 anonymizer: AnonymizerClient | None = None,
                 llm: LLMClient | None = None,
                 retrieve_fn: RetrieveFn | None = None) -> None:
        self._settings = settings
        self._anonymizer = anonymizer
        self._owns_anonymizer = anonymizer is None
        self._llm = llm
        # retrieve по умолчанию импортируется ЛЕНИВО (app.retrieval тянет
        # qdrant-client/torch) — чтобы тесты с моками не требовали тяжёлых
        # зависимостей и не ходили в сеть.
        self._retrieve: RetrieveFn | None = retrieve_fn

    def _get_anonymizer(self) -> AnonymizerClient:
        if self._anonymizer is None:
            self._anonymizer = AnonymizerClient(self._settings)
        return self._anonymizer

    def _get_llm(self) -> LLMClient:
        if self._llm is None:
            self._llm = OpenAICompatibleClient(self._settings)
        return self._llm

    def _get_retrieve(self) -> RetrieveFn:
        if self._retrieve is None:
            # Ленивый импорт: app.retrieval тянет qdrant-client/torch.
            from app.retrieval import retrieve as default_retrieve
            self._retrieve = default_retrieve
        return self._retrieve

    def close(self) -> None:
        if self._owns_anonymizer and self._anonymizer is not None:
            self._anonymizer.close()

    # ── Шаг 1: анонимизация свободного текста (docs/04 §1, fail-closed) ──────
    def _anonymize_answers(self, doc_type: str, answers: dict) -> _AnonymizedAnswers:
        safe = copy.deepcopy(answers)
        total_removed = 0
        blocked: list[str] = []
        anonymizer = self._get_anonymizer()

        for path, text in iter_free_text(doc_type, answers):
            res = anonymizer.anonymize(text)
            if not res.passed:
                # Fail-closed: остаточные ПДн / ошибка гейта → блокируем ввод.
                blocked.append(path)
                logger.warning("generate: гейт заблокировал свободное поле '%s' (%s)",
                               path, res.reason)
                continue
            _set_by_path(safe, path, res.content)
            total_removed += res.removed_count

        return _AnonymizedAnswers(answers=safe, removed_count=total_removed,
                                  blocked_fields=blocked)

    def generate(self, doc_type: str, answers: dict, *,
                 top_k: int | None = None) -> GenerationResult:
        """Полный конвейер генерации дневника. См. docstring модуля."""
        if doc_type not in SUPPORTED_DOC_TYPES:
            raise UnsupportedDocTypeError(
                f"Тип '{doc_type}' не поддерживается. "
                f"Доступны: {', '.join(SUPPORTED_DOC_TYPES)}")

        # 1. Анонимизация свободного ввода ДО любой обработки.
        anon = self._anonymize_answers(doc_type, answers)
        if anon.blocked_fields:
            raise PiiBlockedError(
                "Во входных данных обнаружены ПДн, которые не удалось безопасно "
                "обезличить (поля: " + ", ".join(anon.blocked_fields) + ")")

        # 2. Маппинг ответов → формулировки + метаданные.
        mapped = map_answers(doc_type, anon.answers)

        # 3. Retrieval few-shot образцов нужного типа/регистра (docs/03 §6).
        k = top_k if top_k is not None else self._settings.retrieval_top_k
        query = build_query_text(mapped, doc_type)
        try:
            samples = self._get_retrieve()(
                query, doc_type=doc_type, top_k=k,
                syndrome=mapped.syndrome, diagnosis_class=mapped.diagnosis_class)
        except Exception as exc:  # noqa: BLE001 — retrieval не должен ронять генерацию
            logger.warning("generate: retrieval недоступен (%s) — генерация без "
                           "few-shot образцов", type(exc).__name__)
            samples = []

        # 4. Сборка промпта (daily/exam_10d).
        messages = build_messages(doc_type, mapped, samples)

        # 5. Вызов LLM с автофолбэком. Температура зависит от типа документа:
        # ежедневные дневники — выше (живость стиля), осмотр 10 дней — чуть ниже.
        temp = (
            self._settings.llm_temperature
            if doc_type == DOC_TYPE_DAILY
            else self._settings.llm_temperature_exam10d
        )
        result: LLMResult = self._get_llm().generate(messages, temperature=temp)

        logger.info("generate: doc_type=%s, модель=%s, образцов=%d, токенов=%s",
                    doc_type, result.model, len(samples),
                    result.usage.get("total_tokens"))

        return GenerationResult(
            content=result.content,
            model_used=result.model,
            tokens_used=int(result.usage.get("total_tokens", 0) or 0),
            chunks_used=len(samples),
            title_safe=mapped.title_safe,
            answers_anonymized=anon.answers,
            anonymizer_removed_count=anon.removed_count,
            syndrome=mapped.syndrome,
            diagnosis_class=mapped.diagnosis_class,
            dynamics=mapped.dynamics,
        )
