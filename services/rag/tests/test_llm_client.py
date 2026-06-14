"""Юнит-тесты LLM-клиента X5 с автофолбэком (docs/03 §9–10).

Все вызовы МОКАЮТСЯ — реальная сеть/ключ не нужны. Проверяем:
  (а) фолбэк large→medium→small при 5xx/timeout;
  (б) отсутствие ретраев/перебора моделей на 401/403 (auth);
  (в) «все модели недоступны» → AllModelsUnavailableError;
  (г) успешный путь возвращает контент и имя модели;
  (д) ретраи ВНУТРИ модели на retryable-ошибках перед фолбэком.

Запуск: cd services/rag && python -m pytest tests/test_llm_client.py -q
"""

from __future__ import annotations

import httpx
import pytest
from openai import APIStatusError, APITimeoutError, AuthenticationError

from app.config import Settings
from app.llm_client import (
    AllModelsUnavailableError,
    LLMAuthError,
    LLMMessage,
    LLMNotConfiguredError,
    X5CopilotClient,
)

_REQ = httpx.Request(
    "POST", "https://api-copilot.x5.ru/aigw/v1/chat/completions")


def _status_error(code: int) -> APIStatusError:
    resp = httpx.Response(code, request=_REQ)
    return APIStatusError(f"http {code}", response=resp, body=None)


def _auth_error() -> AuthenticationError:
    resp = httpx.Response(401, request=_REQ)
    return AuthenticationError("unauthorized", response=resp, body=None)


def _timeout_error() -> APITimeoutError:
    return APITimeoutError(request=_REQ)


class _Msg:
    def __init__(self, content: str) -> None:
        self.content = content


class _Choice:
    def __init__(self, content: str) -> None:
        self.message = _Msg(content)
        self.finish_reason = "stop"


class _Usage:
    def __init__(self) -> None:
        self.prompt_tokens = 100
        self.completion_tokens = 50
        self.total_tokens = 150


class _Response:
    def __init__(self, content: str, model: str) -> None:
        self.choices = [_Choice(content)]
        self.model = model
        self.usage = _Usage()


class _Completions:
    def __init__(self, behavior) -> None:
        self._behavior = behavior
        self.calls: list[dict] = []

    def create(self, **kwargs):
        self.calls.append(kwargs)
        return self._behavior(kwargs, len(self.calls))


class _Chat:
    def __init__(self, behavior) -> None:
        self.completions = _Completions(behavior)


class FakeOpenAI:
    """Минимальный фейк openai-клиента: client.chat.completions.create(...)."""

    def __init__(self, behavior) -> None:
        self.chat = _Chat(behavior)


def _settings(**over) -> Settings:
    # max_retries=1 — чтобы тесты фолбэка не тратили время на backoff.
    base = dict(
        x5_api_key="test-key",
        llm_max_retries=1,
        llm_backoff_initial_s=0.0,
        llm_backoff_max_s=0.0,
    )
    base.update(over)
    return Settings(**base)


def _msgs() -> list[LLMMessage]:
    return [LLMMessage(role="user", content="привет")]


def test_success_first_model() -> None:
    """Успех на первой модели — возвращается контент и её имя."""
    def behavior(kwargs, n):
        return _Response("готовый дневник", kwargs["model"])

    client = X5CopilotClient(_settings(), openai_client=FakeOpenAI(behavior))
    res = client.generate(_msgs())
    assert res.content == "готовый дневник"
    assert res.model == "x5-airun-large"
    assert res.usage["total_tokens"] == 150


def test_fallback_large_to_medium_to_small() -> None:
    """large и medium падают 5xx → отрабатывает small (docs/03 §10)."""
    def behavior(kwargs, n):
        model = kwargs["model"]
        if model in ("x5-airun-large", "x5-airun-medium"):
            raise _status_error(503)
        return _Response("дневник от small", model)

    fake = FakeOpenAI(behavior)
    client = X5CopilotClient(_settings(), openai_client=fake)
    res = client.generate(_msgs())
    assert res.model == "x5-airun-small"
    # Перебрали все три модели по порядку.
    used_models = [c["model"] for c in fake.chat.completions.calls]
    assert used_models == ["x5-airun-large",
                           "x5-airun-medium", "x5-airun-small"]


def test_fallback_on_timeout() -> None:
    """Timeout основной модели → фолбэк на следующую."""
    def behavior(kwargs, n):
        if kwargs["model"] == "x5-airun-large":
            raise _timeout_error()
        return _Response("ok", kwargs["model"])

    client = X5CopilotClient(_settings(), openai_client=FakeOpenAI(behavior))
    res = client.generate(_msgs())
    assert res.model == "x5-airun-medium"


def test_auth_error_no_retry_no_fallback() -> None:
    """401/403 → LLMAuthError сразу, БЕЗ ретраев и БЕЗ перебора моделей."""
    def behavior(kwargs, n):
        raise _auth_error()

    fake = FakeOpenAI(behavior)
    client = X5CopilotClient(_settings(), openai_client=fake)
    with pytest.raises(LLMAuthError):
        client.generate(_msgs())
    # Ровно ОДИН вызов: не ретраили и не перебирали medium/small.
    assert len(fake.chat.completions.calls) == 1


def test_all_models_unavailable() -> None:
    """Все модели падают 5xx → AllModelsUnavailableError (→ 503)."""
    def behavior(kwargs, n):
        raise _status_error(500)

    fake = FakeOpenAI(behavior)
    client = X5CopilotClient(_settings(), openai_client=fake)
    with pytest.raises(AllModelsUnavailableError) as exc:
        client.generate(_msgs())
    assert exc.value.status_code == 503
    assert set(exc.value.attempts) == {
        "x5-airun-large", "x5-airun-medium", "x5-airun-small"}


def test_retries_within_model_before_fallback() -> None:
    """Ретраи ВНУТРИ модели: с retries=2 large вызывается дважды, потом фолбэк."""
    def behavior(kwargs, n):
        if kwargs["model"] == "x5-airun-large":
            raise _status_error(502)
        return _Response("ok", kwargs["model"])

    fake = FakeOpenAI(behavior)
    client = X5CopilotClient(_settings(llm_max_retries=2), openai_client=fake)
    res = client.generate(_msgs())
    assert res.model == "x5-airun-medium"
    calls = [c["model"] for c in fake.chat.completions.calls]
    # large дважды (ретрай), затем medium один раз.
    assert calls == ["x5-airun-large", "x5-airun-large", "x5-airun-medium"]


def test_not_configured_without_key() -> None:
    """Без X5_API_KEY генерация даёт понятную LLMNotConfiguredError."""
    client = X5CopilotClient(_settings(x5_api_key=""))
    with pytest.raises(LLMNotConfiguredError):
        client.generate(_msgs())


def test_models_order_dedup() -> None:
    """Список моделей: пустые отбрасываются, дубликаты схлопываются по порядку."""
    s = _settings(llm_model_large="m",
                  llm_model_medium="m", llm_model_small="")
    assert s.llm_models() == ["m"]
