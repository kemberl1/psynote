"""Локальная модель эмбеддингов (docs/03 §3 — multilingual-e5-large).

Эмбеддинги считаются ЛОКАЛЬНО — медицинский текст НЕ уходит в облако (NFR-P3).
Модель грузится через sentence-transformers; кэш весов — в volume (HF_HOME),
чтобы не качать модель при каждом запуске контейнера.

Префиксы e5 (важно для качества!):
  - "passage: <text>" — для индексируемых фрагментов корпуса (ingestion);
  - "query: <text>"   — для поискового запроса (retrieval, Этап 4).

Загрузка модели ленивая (singleton) — тяжёлый импорт torch/sentence-transformers
происходит только при первом обращении, не при импорте модуля.
"""

from __future__ import annotations

import logging
import os
import threading

from app.config import Settings

logger = logging.getLogger(__name__)

_model = None
_model_lock = threading.Lock()


def _is_e5(model_name: str) -> bool:
    return "e5" in model_name.lower()


def _load_model(settings: Settings):
    """Лениво загрузить модель (singleton, потокобезопасно)."""
    global _model
    if _model is not None:
        return _model
    with _model_lock:
        if _model is not None:
            return _model
        # Указываем кэш до импорта, чтобы sentence-transformers его подхватил.
        os.environ.setdefault("HF_HOME", settings.hf_home)
        os.environ.setdefault("SENTENCE_TRANSFORMERS_HOME", settings.hf_home)
        try:
            from sentence_transformers import SentenceTransformer
        except ImportError as exc:  # pragma: no cover
            raise RuntimeError(
                "sentence-transformers не установлен (см. requirements.txt Этап 3)"
            ) from exc

        logger.info("Загрузка модели эмбеддингов '%s' на устройство '%s'...",
                    settings.embedding_model, settings.embedding_device)
        _model = SentenceTransformer(
            settings.embedding_model,
            device=settings.embedding_device,
            cache_folder=settings.hf_home,
        )
        logger.info("Модель эмбеддингов загружена.")
        return _model


class Embedder:
    """Обёртка над локальной моделью эмбеддингов с поддержкой префиксов e5."""

    def __init__(self, settings: Settings) -> None:
        self._settings = settings
        self._use_e5_prefix = _is_e5(settings.embedding_model)

    def _prefix(self, text: str, kind: str) -> str:
        # kind: "passage" (индексация) | "query" (поиск).
        if self._use_e5_prefix:
            return f"{kind}: {text}"
        return text

    def embed_passages(self, texts: list[str]) -> list[list[float]]:
        """Эмбеддинги для индексируемых фрагментов корпуса (ingestion)."""
        return self._encode(texts, kind="passage")

    def embed_query(self, text: str) -> list[float]:
        """Эмбеддинг поискового запроса (retrieval, Этап 4)."""
        return self._encode([text], kind="query")[0]

    def _encode(self, texts: list[str], kind: str) -> list[list[float]]:
        model = _load_model(self._settings)
        prefixed = [self._prefix(t, kind) for t in texts]
        vectors = model.encode(
            prefixed,
            batch_size=self._settings.embedding_batch_size,
            # e5 рекомендует нормализацию для cosine-метрики (docs/05 §3 — cosine).
            normalize_embeddings=True,
            convert_to_numpy=True,
            show_progress_bar=False,
        )
        return [v.tolist() for v in vectors]

    @property
    def dimension(self) -> int:
        """Размерность вектора модели (для создания коллекции Qdrant)."""
        model = _load_model(self._settings)
        dim = model.get_sentence_embedding_dimension()
        return int(dim) if dim else self._settings.embedding_dim


# Удобная функция для обратной совместимости с каркасом.
def embed_texts(texts: list[str], settings: Settings) -> list[list[float]]:
    """Вернуть passage-эмбеддинги для списка текстов (ingestion)."""
    return Embedder(settings).embed_passages(texts)
