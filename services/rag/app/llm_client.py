"""OpenAI-совместимый LLM-клиент с АВТОФОЛБЭКОМ моделей.

Этап 4 (docs/03 §9–10). Работает с любым OpenAI-совместимым API
(DeepSeek / OpenRouter / Groq / др.): openai SDK → base_url + Bearer-ключ из ENV,
опциональный CA-bundle (TLS-верификация ВКЛЮЧЕНА), ретраи через tenacity
(экспоненциальный backoff с джиттером).

ОТКЛОНЕНИЕ ОТ docs/02/03: в исходном дизайне LLM-клиент с фолбэком жил в
Go-Gateway. По заданию Этапа 4 он реализован здесь, в services/rag (Python),
как «План Б» из docs/03 §9 — чтобы весь конвейер генерации (retrieval+промпт+LLM)
был в одном сервисе. Абстрактный интерфейс (docs/03 §9.4) сохранён — провайдера
можно заменить, не трогая бизнес-логику.

Стратегия фолбэка (docs/03 §10):
  • Ретраи ВНУТРИ одной модели на 429/5xx/timeout/сетевых (backoff с джиттером).
  • Переключение на следующую модель при исчерпании ретраев / недоступности.
  • Ошибки авторизации (401/403) НЕ ретраим и НЕ перебираем модели — это
    конфиг-проблема ключа, сразу понятная ошибка (AuthError).
  • Если ВСЕ модели исчерпаны — AllModelsUnavailableError (→ 503 в API).

ПРИВАТНОСТЬ: ключ НИКОГДА не логируется; содержимое промпта/ответа целиком
в логи не пишется (только модель, число токенов, finish_reason, статусы).
"""

from __future__ import annotations

import abc
import logging
import ssl
from dataclasses import dataclass, field
from typing import Any

import httpx
from openai import (
    APIConnectionError,
    APIStatusError,
    APITimeoutError,
    AuthenticationError,
    OpenAI,
    PermissionDeniedError,
    RateLimitError,
)
from tenacity import (
    retry,
    retry_if_exception,
    stop_after_attempt,
    wait_exponential_jitter,
)

from app.config import Settings

logger = logging.getLogger(__name__)


# ─── Структуры данных (нормализованный контракт, независимый от провайдера) ──

@dataclass(frozen=True)
class LLMMessage:
    """Одно сообщение чата."""

    role: str  # "system" | "user" | "assistant"
    content: str


@dataclass(frozen=True)
class LLMResult:
    """Нормализованный результат генерации одной моделью."""

    content: str
    model: str
    usage: dict[str, int] = field(default_factory=dict)


# ─── Иерархия ошибок ────────────────────────────────────────────────────────

class LLMError(Exception):
    """Базовая ошибка LLM-слоя."""

    def __init__(self, message: str, *, status_code: int | None = None,
                 retryable: bool = False) -> None:
        super().__init__(message)
        self.status_code = status_code
        self.retryable = retryable


class LLMAuthError(LLMError):
    """Ошибка авторизации (401/403). НЕ ретраится и НЕ переключает модели."""

    def __init__(self, message: str, *, status_code: int | None = None) -> None:
        super().__init__(message, status_code=status_code, retryable=False)


class LLMNotConfiguredError(LLMError):
    """Ключ/конфиг LLM не задан — генерация невозможна (понятная ошибка)."""


class AllModelsUnavailableError(LLMError):
    """Все модели фолбэка исчерпаны (5xx/timeout/сеть) — сервис недоступен."""

    def __init__(self, message: str, *, attempts: list[str] | None = None) -> None:
        super().__init__(message, status_code=503, retryable=False)
        self.attempts = attempts or []


def _is_retryable(exc: BaseException) -> bool:
    """Решить, ретраить ли вызов ВНУТРИ одной модели.

    Auth-ошибки (401/403) НЕ ретраятся. 429/5xx/timeout/сеть — ретраятся.
    """
    if isinstance(exc, LLMAuthError):
        return False
    if isinstance(exc, LLMError):
        return exc.retryable
    if isinstance(exc, (APITimeoutError, APIConnectionError, RateLimitError)):
        return True
    if isinstance(exc, APIStatusError):
        return exc.status_code >= 500
    return False


# ─── Абстрактный интерфейс (docs/03 §9.4 — заменяемость провайдера) ──────────

class LLMClient(abc.ABC):
    """Абстрактный LLM-клиент с фолбэком моделей."""

    @abc.abstractmethod
    def generate(self, messages: list[LLMMessage], *,
                 temperature: float | None = None,
                 max_tokens: int | None = None) -> LLMResult:
        """Сгенерировать ответ, перебирая модели по приоритету при сбоях."""


# ─── OpenAI-совместимая реализация ───────────────────────────────────────────

class OpenAICompatibleClient(LLMClient):
    """Клиент любого OpenAI-совместимого API с автофолбэком large→medium→small.

    Параметры берутся из Settings; openai-клиент можно подменить в тестах
    (`openai_client=`), чтобы не ходить в реальную сеть.
    """

    def __init__(self, settings: Settings, *,
                 openai_client: Any | None = None) -> None:
        self._settings = settings
        self._models = settings.llm_models()
        self._timeout = settings.llm_timeout_s
        self._max_retries = max(1, settings.llm_max_retries)
        self._temperature = settings.llm_temperature
        self._max_tokens = settings.llm_max_tokens

        if openai_client is not None:
            # Внедрённый клиент (тесты/моки) — конфиг ключа не обязателен.
            self._client = openai_client
            self._configured = True
            return

        if not settings.llm_api_key:
            # Ключ не задан — клиент создаётся «неактивным». Ошибка вылетит
            # только при попытке генерации (LLMNotConfiguredError), чтобы
            # сервис мог стартовать и отвечать на /health без ключа.
            self._client = None
            self._configured = False
            logger.warning(
                "LLM: LLM_API_KEY не задан — генерация будет недоступна")
            return

        # Опциональный CA-bundle: TLS-верификация ОСТАЁТСЯ включённой
        # (docs/03 §9.2). Не отключаем verify!
        http_client: httpx.Client | None = None
        if settings.llm_ca_bundle:
            ssl_ctx = ssl.create_default_context(cafile=settings.llm_ca_bundle)
            http_client = httpx.Client(
                verify=ssl_ctx, timeout=float(self._timeout))

        self._client = OpenAI(
            base_url=settings.llm_base_url,
            api_key=settings.llm_api_key,
            timeout=float(self._timeout),
            max_retries=0,  # ретраи делаем сами через tenacity
            http_client=http_client,
        )
        self._configured = True

        logger.info(
            "LLM init: base_url=%s, models=%s, timeout=%.0fs, retries=%d, "
            "custom_ca=%s",  # ключ НИКОГДА не логируем
            settings.llm_base_url, self._models, self._timeout,
            self._max_retries, bool(settings.llm_ca_bundle),
        )

    @property
    def models(self) -> list[str]:
        return list(self._models)

    def generate(self, messages: list[LLMMessage], *,
                 temperature: float | None = None,
                 max_tokens: int | None = None) -> LLMResult:
        """Перебрать модели по приоритету. См. правила фолбэка в docstring модуля."""
        if not self._configured or self._client is None:
            raise LLMNotConfiguredError(
                "LLM не сконфигурирован: задайте LLM_API_KEY в окружении (.env)")
        if not self._models:
            raise LLMNotConfiguredError(
                "Список моделей пуст: задайте LLM_MODEL_LARGE/MEDIUM/SMALL")

        temp = temperature if temperature is not None else self._temperature
        max_tok = max_tokens if max_tokens is not None else self._max_tokens

        failures: list[str] = []
        for model in self._models:
            try:
                result = self._generate_with_retries(
                    messages, model, temp, max_tok)
                logger.info("LLM: успешно отработала модель '%s' (токенов: %s)",
                            result.model, result.usage.get("total_tokens"))
                return result
            except LLMAuthError:
                # 401/403 — проблема ключа: не ретраим и не перебираем модели.
                logger.error(
                    "LLM: ошибка авторизации — прекращаю перебор моделей")
                raise
            except LLMError as exc:
                # Исчерпаны ретраи этой модели / она недоступна — следующая.
                failures.append(model)
                logger.warning("LLM: модель '%s' недоступна (%s) — пробую следующую",
                               model, type(exc).__name__)
                continue

        raise AllModelsUnavailableError(
            "Все LLM-модели недоступны после фолбэка", attempts=failures)

    def _generate_with_retries(self, messages: list[LLMMessage], model: str,
                               temperature: float, max_tokens: int) -> LLMResult:
        """Вызов одной модели с ретраями (экспоненциальный backoff + джиттер)."""

        @retry(
            retry=retry_if_exception(_is_retryable),
            stop=stop_after_attempt(self._max_retries),
            wait=wait_exponential_jitter(
                initial=self._settings.llm_backoff_initial_s,
                max=self._settings.llm_backoff_max_s,
                jitter=2,
            ),
            reraise=True,
        )
        def _call() -> LLMResult:
            return self._raw_chat(messages, model, temperature, max_tokens)

        return _call()

    def _raw_chat(self, messages: list[LLMMessage], model: str,
                  temperature: float, max_tokens: int) -> LLMResult:
        """Один вызов chat-completions без ретраев. Маппит ошибки в LLMError."""
        payload = [{"role": m.role, "content": m.content} for m in messages]
        create_kwargs: dict[str, Any] = {
            "model": model,
            "messages": payload,
            "temperature": temperature,
            "max_tokens": max_tokens,
        }
        # DeepSeek V4: thinking по умолчанию enabled у провайдера. Явно гасим/
        # включаем через ENV, иначе при малом max_tokens content бывает пустым.
        thinking = (self._settings.llm_thinking or "").strip().lower()
        if thinking in ("disabled", "enabled"):
            create_kwargs["extra_body"] = {"thinking": {"type": thinking}}
        try:
            response = self._client.chat.completions.create(  # type: ignore[union-attr]
                **create_kwargs,
            )
        except (AuthenticationError, PermissionDeniedError) as exc:
            status = getattr(exc, "status_code", None)
            logger.error(
                "LLM: auth error (status=%s) для модели '%s'", status, model)
            raise LLMAuthError("Ошибка авторизации LLM (проверьте LLM_API_KEY)",
                               status_code=status) from exc
        except RateLimitError as exc:
            raise LLMError("LLM rate limit", status_code=getattr(exc, "status_code", 429),
                           retryable=True) from exc
        except APITimeoutError as exc:
            raise LLMError("LLM timeout", retryable=True) from exc
        except APIConnectionError as exc:
            raise LLMError("LLM connection error", retryable=True) from exc
        except APIStatusError as exc:
            status = getattr(exc, "status_code", None) or 500
            # 402 Insufficient Balance — не ретраим (проблема баланса, не сети).
            raise LLMError(f"LLM provider error ({status})", status_code=status,
                           retryable=status >= 500) from exc

        choice = response.choices[0]
        content = (choice.message.content or "").strip()
        usage: dict[str, int] = {}
        if getattr(response, "usage", None):
            usage = {
                "prompt_tokens": int(getattr(response.usage, "prompt_tokens", 0) or 0),
                "completion_tokens": int(getattr(response.usage, "completion_tokens", 0) or 0),
                "total_tokens": int(getattr(response.usage, "total_tokens", 0) or 0),
            }
        return LLMResult(
            content=content,
            model=getattr(response, "model", None) or model,
            usage=usage,
        )
