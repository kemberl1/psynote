"""HTTP-клиент к Go-анонимайзеру gateway (docs/04 §1, критичный принцип приватности).

Единственный источник истины по анонимизации — Go-сервис gateway. Python-ingestion
НЕ реализует свою анонимизацию, а ВЫЗЫВАЕТ:

    POST {GATEWAY_URL}/api/v1/anonymize
    body: {"text": "<сырой фрагмент>"}
    200 → {meta, data:{content, anonymizer_removed_count, removed_by_type, gate_passed}}
    422 → {meta, error:{code:"PII_DETECTED", ...}}  (остаточные ПДн / fail-closed)

Поведение fail-closed (docs/04 §1):
  - gate_passed=false ИЛИ HTTP 422 → фрагмент НЕ индексируется (AnonymizeResult.passed=False);
  - сетевые ошибки/таймауты → ретраи с backoff; при исчерпании — тоже fail-closed.

ПРИВАТНОСТЬ: в логи пишем ТОЛЬКО статусы/счётчики, НИКОГДА сырой текст и обезличенный
контент целиком (docs/04 §7).
"""

from __future__ import annotations

import logging
import time
from dataclasses import dataclass, field

import httpx

from app.config import Settings

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class AnonymizeResult:
    """Результат обращения к гейту по одному фрагменту."""

    passed: bool
    content: str = ""
    removed_count: int = 0
    removed_by_type: dict[str, int] = field(default_factory=dict)
    # Причина непрохождения для лога (без ПДн): "pii_detected" | "error" | "ok".
    reason: str = "ok"


class AnonymizerClient:
    """Тонкий устойчивый клиент к /api/v1/anonymize с ретраями и fail-closed."""

    def __init__(self, settings: Settings, client: httpx.Client | None = None) -> None:
        self._settings = settings
        self._url = settings.gateway_url.rstrip("/") + settings.anonymize_path
        self._retries = max(1, settings.anonymize_retries)
        self._backoff = settings.anonymize_backoff_s
        self._owns_client = client is None
        self._client = client or httpx.Client(
            timeout=httpx.Timeout(settings.anonymize_timeout_s),
        )

    def close(self) -> None:
        if self._owns_client:
            self._client.close()

    def __enter__(self) -> "AnonymizerClient":
        return self

    def __exit__(self, *exc: object) -> None:
        self.close()

    def anonymize(self, text: str) -> AnonymizeResult:
        """Обезличить фрагмент через gateway. Fail-closed при любой неопределённости."""
        if not text or not text.strip():
            return AnonymizeResult(passed=False, reason="empty")

        last_exc: Exception | None = None
        for attempt in range(1, self._retries + 1):
            try:
                resp = self._client.post(self._url, json={"text": text})
            except (httpx.TimeoutException, httpx.TransportError) as exc:
                last_exc = exc
                logger.warning("anonymize: сетевая ошибка (попытка %d/%d): %s",
                               attempt, self._retries, type(exc).__name__)
                self._sleep(attempt)
                continue

            # 422 — гейт заблокировал (остаточные ПДн). Не ретраим — это вердикт.
            if resp.status_code == 422:
                return AnonymizeResult(passed=False, reason="pii_detected")

            # 5xx — временная ошибка сервиса, ретраим.
            if resp.status_code >= 500:
                logger.warning("anonymize: gateway %d (попытка %d/%d)",
                               resp.status_code, attempt, self._retries)
                self._sleep(attempt)
                continue

            if resp.status_code != 200:
                # 4xx (кроме 422) — клиентская ошибка, не ретраим, fail-closed.
                logger.warning("anonymize: неожиданный статус %d — fail-closed",
                               resp.status_code)
                return AnonymizeResult(passed=False, reason="error")

            return self._parse_ok(resp)

        logger.warning("anonymize: исчерпаны ретраи (%s) — fail-closed",
                       type(last_exc).__name__ if last_exc else "5xx")
        return AnonymizeResult(passed=False, reason="error")

    def _parse_ok(self, resp: httpx.Response) -> AnonymizeResult:
        try:
            body = resp.json()
        except ValueError:
            logger.warning("anonymize: невалидный JSON — fail-closed")
            return AnonymizeResult(passed=False, reason="error")

        data = body.get("data") or {}
        gate_passed = bool(data.get("gate_passed", False))
        content = data.get("content", "") or ""

        # Fail-closed: даже при 200, если гейт не пройден — не индексируем.
        if not gate_passed or not content.strip():
            return AnonymizeResult(passed=False, reason="pii_detected")

        return AnonymizeResult(
            passed=True,
            content=content,
            removed_count=int(data.get("anonymizer_removed_count", 0) or 0),
            removed_by_type=dict(data.get("removed_by_type") or {}),
            reason="ok",
        )

    def _sleep(self, attempt: int) -> None:
        # Экспоненциальный backoff: backoff * 2^(attempt-1).
        if attempt < self._retries:
            time.sleep(self._backoff * (2 ** (attempt - 1)))

    def health_check(self) -> bool:
        """Проверить доступность gateway перед прогоном (мягкая проверка)."""
        try:
            base = self._settings.gateway_url.rstrip("/")
            resp = self._client.get(base + "/api/v1/health")
            return resp.status_code == 200
        except httpx.HTTPError:
            return False
