"""Прогрев тяжёлых зависимостей при старте сервиса.

Модель эмбеддингов (e5) грузится лениво, при первом поиске образцов. На
проде это означало, что ПЕРВЫЙ дневник после каждого деплоя ждал ~13 секунд
загрузки, а остальные шли за 3–5. Прогреваем её в фоне при старте: к первому
запросу врача модель уже в памяти.

Прогрев никогда не роняет сервис: любая ошибка только пишется в лог, а
поиск образцов и без него отработает (лениво, как раньше).
"""

from __future__ import annotations

import logging
import threading
import time

from app.config import Settings

logger = logging.getLogger(__name__)


def warm_embeddings(settings: Settings) -> bool:
    """Загрузить модель эмбеддингов. True — прогрев удался."""
    started = time.perf_counter()
    try:
        from app.embeddings import Embedder

        Embedder(settings).embed_query("прогрев модели эмбеддингов")
    except Exception as exc:  # noqa: BLE001 — прогрев не должен ронять сервис
        logger.warning("warmup: модель эмбеддингов не прогрета (%s: %s)",
                       type(exc).__name__, exc)
        return False
    logger.info("warmup: модель эмбеддингов готова за %.1fs",
                time.perf_counter() - started)
    return True


def warm_embeddings_in_background(settings: Settings) -> threading.Thread:
    """Прогреть модель, не задерживая старт сервиса и /health."""
    thread = threading.Thread(
        target=warm_embeddings, args=(settings,),
        name="warm-embeddings", daemon=True,
    )
    thread.start()
    return thread
