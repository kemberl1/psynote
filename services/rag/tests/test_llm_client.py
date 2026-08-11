"""Юнит-тесты OpenAI-совместимого LLM-клиента с автофолбэком (docs/03 §9–10).

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
    OpenAICompatibleClient,
)

_MODEL_LARGE = "deepseek-v4-flash"
_MODEL_MEDIUM = "deepseek-v4-pro"
_MODEL_SMALL = "deepseek-backup"

_REQ = httpx.Request(
    "POST",
    "https://api.deepseek.com/chat/completions",
)


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
    # Явно задаём 3 разные модели — покрываем полный путь фолбэка.
    base = dict(
        llm_api_key="test-key",
        llm_model_large=_MODEL_LARGE,
        llm_model_medium=_MODEL_MEDIUM,
        llm_model_small=_MODEL_SMALL,
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

    client = OpenAICompatibleClient(_settings(), openai_client=FakeOpenAI(behavior))
    res = client.generate(_msgs())
    assert res.content == "готовый дневник"
    assert res.model == _MODEL_LARGE
    assert res.usage["total_tokens"] == 150


def test_fallback_large_to_medium_to_small() -> None:
    """large и medium падают 5xx → отрабатывает small (docs/03 §10)."""
    def behavior(kwargs, n):
        model = kwargs["model"]
        if model in (_MODEL_LARGE, _MODEL_MEDIUM):
            raise _status_error(503)
        return _Response("дневник от small", model)

    fake = FakeOpenAI(behavior)
    client = OpenAICompatibleClient(_settings(), openai_client=fake)
    res = client.generate(_msgs())
    assert res.model == _MODEL_SMALL
    used_models = [c["model"] for c in fake.chat.completions.calls]
    assert used_models == [_MODEL_LARGE, _MODEL_MEDIUM, _MODEL_SMALL]


def test_fallback_on_timeout() -> None:
    """Timeout основной модели → фолбэк на следующую."""
    def behavior(kwargs, n):
        if kwargs["model"] == _MODEL_LARGE:
            raise _timeout_error()
        return _Response("ok", kwargs["model"])

    client = OpenAICompatibleClient(_settings(), openai_client=FakeOpenAI(behavior))
    res = client.generate(_msgs())
    assert res.model == _MODEL_MEDIUM


def test_auth_error_no_retry_no_fallback() -> None:
    """401/403 → LLMAuthError сразу, БЕЗ ретраев и БЕЗ перебора моделей."""
    def behavior(kwargs, n):
        raise _auth_error()

    fake = FakeOpenAI(behavior)
    client = OpenAICompatibleClient(_settings(), openai_client=fake)
    with pytest.raises(LLMAuthError):
        client.generate(_msgs())
    assert len(fake.chat.completions.calls) == 1


def test_all_models_unavailable() -> None:
    """Все модели падают 5xx → AllModelsUnavailableError (→ 503)."""
    def behavior(kwargs, n):
        raise _status_error(500)

    fake = FakeOpenAI(behavior)
    client = OpenAICompatibleClient(_settings(), openai_client=fake)
    with pytest.raises(AllModelsUnavailableError) as exc:
        client.generate(_msgs())
    assert exc.value.status_code == 503
    assert set(exc.value.attempts) == {
        _MODEL_LARGE, _MODEL_MEDIUM, _MODEL_SMALL}


def test_retries_within_model_before_fallback() -> None:
    """Ретраи ВНУТРИ модели: с retries=2 large вызывается дважды, потом фолбэк."""
    def behavior(kwargs, n):
        if kwargs["model"] == _MODEL_LARGE:
            raise _status_error(502)
        return _Response("ok", kwargs["model"])

    fake = FakeOpenAI(behavior)
    client = OpenAICompatibleClient(_settings(llm_max_retries=2), openai_client=fake)
    res = client.generate(_msgs())
    assert res.model == _MODEL_MEDIUM
    calls = [c["model"] for c in fake.chat.completions.calls]
    assert calls == [_MODEL_LARGE, _MODEL_LARGE, _MODEL_MEDIUM]


def test_not_configured_without_key() -> None:
    """Без LLM_API_KEY генерация даёт понятную LLMNotConfiguredError."""
    client = OpenAICompatibleClient(_settings(llm_api_key=""))
    with pytest.raises(LLMNotConfiguredError):
        client.generate(_msgs())


def test_models_order_dedup() -> None:
    """Список моделей: пустые отбрасываются, дубликаты схлопываются по порядку."""
    s = _settings(llm_model_large="m",
                  llm_model_medium="m", llm_model_small="")
    assert s.llm_models() == ["m"]
