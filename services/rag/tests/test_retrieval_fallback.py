"""Юнит-тесты ступенчатого retrieval-фолбэка (Этап 4.1, docs/03 §6).

Проверяем graceful degradation фильтров БЕЗ сети и БЕЗ torch:
  • тестируем чистую функцию search_with_fallback() с ФЕЙКОВЫМ qdrant-client;
  • фейк отдаёт пусто на строгих уровнях и непустой результат — на более слабом;
  • проверяем, что фолбэк ДОХОДИТ до непустого уровня, логирует уровень,
    и гарантирует chunks_used > 0, пока в коллекции есть данные.

Запуск: cd services/rag && python -m pytest tests/test_retrieval_fallback.py -q
"""

from __future__ import annotations

import logging
from dataclasses import dataclass

from app.retrieval import _fallback_levels, search_with_fallback


@dataclass
class _Hit:
    score: float
    payload: dict


class FakeClient:
    """Фейк qdrant-client: возвращает hits, начиная с уровня `hit_from_call`.

    Каждый вызов search() инкрементирует счётчик. Пока счётчик < hit_from_call —
    возвращаем []. Это имитирует «строгий фильтр пуст → ослабляем → нашли».
    Записывает применённые фильтры для проверки последовательности уровней.
    """

    def __init__(self, hit_from_call: int, *, n_hits: int = 3) -> None:
        self.hit_from_call = hit_from_call
        self.n_hits = n_hits
        self.calls = 0
        self.filters_seen: list[object] = []

    def search(self, *, collection_name, query_vector, query_filter, limit,
               with_payload):
        self.calls += 1
        self.filters_seen.append(query_filter)
        if self.calls < self.hit_from_call:
            return []
        return [
            _Hit(score=1.0 / i,
                 payload={"text": f"sample-{i}", "doc_type": "exam_10d"})
            for i in range(1, self.n_hits + 1)
        ]


_LEVELS = _fallback_levels(
    doc_type="exam_10d", syndrome="тревожно-депрессивный",
    diagnosis_class="F4x", section=None)


def test_strict_level_hits_immediately() -> None:
    client = FakeClient(hit_from_call=1)
    out = search_with_fallback(client, "corpus_diaries", [0.1], 5, _LEVELS)
    assert len(out) == 3
    assert client.calls == 1  # нашли сразу на L0, дальше не идём


def test_fallback_reaches_nonempty_level(caplog) -> None:
    # Строгий (L0) и diagnosis (L1) пусты → результат на L2 (doc_type).
    client = FakeClient(hit_from_call=3)
    with caplog.at_level(logging.INFO, logger="app.retrieval"):
        out = search_with_fallback(client, "corpus_diaries", [0.1], 5, _LEVELS)
    assert len(out) == 3  # chunks_used > 0 гарантировано
    assert client.calls == 3  # дошли до третьего уровня
    # Залогирован уровень, на котором нашли (L2_doc_type).
    assert any("L2_doc_type" in r.message for r in caplog.records)


def test_fallback_to_no_filter_level() -> None:
    """Если фильтры всё отсекают — последний уровень БЕЗ фильтра (query_filter=None)."""
    client = FakeClient(hit_from_call=len(_LEVELS))
    out = search_with_fallback(client, "corpus_diaries", [0.1], 5, _LEVELS)
    assert len(out) == 3
    assert client.calls == len(_LEVELS)
    # На последнем (успешном) вызове фильтр должен быть None (L3_none).
    assert client.filters_seen[-1] is None


def test_empty_collection_returns_empty(caplog) -> None:
    """Пустая коллекция → [] на всех уровнях + предупреждение в логе."""
    client = FakeClient(hit_from_call=999)  # никогда не отдаёт hits
    with caplog.at_level(logging.WARNING, logger="app.retrieval"):
        out = search_with_fallback(client, "corpus_diaries", [0.1], 5, _LEVELS)
    assert out == []
    assert client.calls == len(_LEVELS)  # перебрали все уровни
    assert any("НЕ найдены" in r.message for r in caplog.records)


def test_fallback_levels_dedup_when_no_diagnosis() -> None:
    """Без diagnosis_class уровни L0/L1 совпали бы → схлопываются (нет дублей)."""
    levels = _fallback_levels(
        doc_type="daily", syndrome=None, diagnosis_class=None, section=None)
    keys = [(l.doc_type, l.syndrome, l.diagnosis_class, l.section)
            for l in levels]
    assert len(keys) == len(set(keys))  # все уровни уникальны
    # Минимум должно остаться: {doc_type-only, none}.
    assert ("daily", None, None, None) in keys
    assert (None, None, None, None) in keys
