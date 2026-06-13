"""Оркестратор пайплайна ingestion корпуса (docs/03 §5, docs/04 §1).

Строгий порядок (критичный принцип приватности, docs/04 §1):

    1. распарсить документ → извлечь СЫРОЙ текст (extractors);
    2. отправить сырой текст в анонимайзер gateway (POST /api/v1/anonymize);
    3. ТОЛЬКО при gate_passed=true получить ОБЕЗЛИЧЕННЫЙ текст;
    4. чанковать ОБЕЗЛИЧЕННЫЙ текст (chunking);
    5. локальные эмбеддинги (embeddings, e5 passage:);
    6. upsert в Qdrant с детерминированными id (qdrant_store).

Обоснование порядка «анонимизация ДО чанкинга» (docs/03 §5, docs/04 §1):
анонимизируем на уровне ЦЕЛОГО документа (крупный фрагмент) — это даёт гейту
максимальный контекст для детекции ФИО в падежах/адресов/дат (качество recall
выше, чем на мелких кусках), и лишь затем режем уже безопасный текст на чанки.
Так сырой текст с ПДн НИКОГДА не доходит до чанкинга/эмбеддинга/Qdrant.

Fail-closed (docs/04 §1): если гейт не пройден (HTTP 422 или gate_passed=false
или ошибка) — документ НЕ индексируется, считается пропущенным, логируется БЕЗ
вывода исходного текста/ПДн. Идём к следующему документу.

ПРИВАТНОСТЬ имён файлов: имена файлов/папок в корпусе содержат ФИО пациентов —
они НЕ попадают в payload и НЕ логируются. Для трассировки используется
обезличенный идентификатор source_ref = sha256(относительный_путь)[:16].
"""

from __future__ import annotations

import hashlib
import logging
from dataclasses import dataclass, field
from pathlib import Path

from app.anonymizer_client import AnonymizerClient
from app.chunking import chunk_document
from app.config import Settings
from app.embeddings import Embedder
from app.extractors import (
    SUPPORTED_SUFFIXES,
    SUPPORTED_TABLE_SUFFIXES,
    ExtractionError,
    extract_text,
)
from app.qdrant_store import PointRecord, QdrantStore, build_payload, make_point_id

logger = logging.getLogger(__name__)


@dataclass
class IngestStats:
    """Счётчики прогона (БЕЗ ПДн — только числа/статусы, docs/04 §7)."""

    files_seen: int = 0
    files_extracted: int = 0
    files_skipped_extract: int = 0
    files_blocked_pii: int = 0
    chunks_built: int = 0
    chunks_written: int = 0
    removed_by_type: dict[str, int] = field(default_factory=dict)

    def merge_removed(self, by_type: dict[str, int]) -> None:
        for k, v in by_type.items():
            self.removed_by_type[k] = self.removed_by_type.get(k, 0) + int(v)

    def summary(self) -> str:
        return (
            f"files_seen={self.files_seen} extracted={self.files_extracted} "
            f"skipped_extract={self.files_skipped_extract} "
            f"blocked_pii={self.files_blocked_pii} "
            f"chunks_built={self.chunks_built} chunks_written={self.chunks_written} "
            f"removed_by_type={self.removed_by_type}"
        )


def source_ref(path: Path, corpus_root: Path) -> str:
    """Обезличенный идентификатор источника (НЕ имя файла с ФИО, docs/03 §4)."""
    try:
        rel = path.relative_to(corpus_root).as_posix()
    except ValueError:
        rel = path.name
    return "src_" + hashlib.sha256(rel.encode("utf-8")).hexdigest()[:16]


def iter_corpus_files(root: Path, *, include_tables: bool) -> list[Path]:
    """Собрать поддерживаемые файлы корпуса (рекурсивно)."""
    suffixes = set(SUPPORTED_SUFFIXES)
    if not include_tables:
        suffixes -= SUPPORTED_TABLE_SUFFIXES
    files: list[Path] = []
    for p in sorted(root.rglob("*")):
        if p.is_file() and p.suffix.lower() in suffixes and not p.name.startswith("~$"):
            files.append(p)
    return files


class IngestionPipeline:
    """Координирует extract → anonymize → chunk → embed → upsert."""

    def __init__(self, settings: Settings, anonymizer: AnonymizerClient,
                 embedder: Embedder, store: QdrantStore) -> None:
        self._settings = settings
        self._anon = anonymizer
        self._embedder = embedder
        self._store = store

    def run(self, files: list[Path], corpus_root: Path) -> IngestStats:
        stats = IngestStats()
        # Создаём коллекцию заранее (нужна размерность модели).
        dim = self._embedder.dimension
        self._store.ensure_collection(dim)

        for path in files:
            stats.files_seen += 1
            ref = source_ref(path, corpus_root)

            # 1. Извлечение сырого текста.
            try:
                raw = extract_text(path)
            except ExtractionError as exc:
                stats.files_skipped_extract += 1
                logger.warning("[%s] пропуск извлечения: %s", ref, exc)
                continue
            except Exception as exc:  # любой неожиданный сбой — не падаем целиком
                stats.files_skipped_extract += 1
                logger.warning("[%s] пропуск (ошибка чтения): %s", ref,
                               type(exc).__name__)
                continue

            if not raw or not raw.strip():
                stats.files_skipped_extract += 1
                logger.warning("[%s] пустой документ — пропуск", ref)
                continue
            stats.files_extracted += 1

            # 2-3. Анонимизация ЦЕЛОГО документа через gateway (fail-closed).
            result = self._anon.anonymize(raw)
            del raw  # сырой текст с ПДн больше не нужен — убираем из памяти
            if not result.passed:
                stats.files_blocked_pii += 1
                logger.warning("[%s] заблокирован гейтом (reason=%s) — не индексируется",
                               ref, result.reason)
                continue
            stats.merge_removed(result.removed_by_type)

            # 4. Чанкинг ОБЕЗЛИЧЕННОГО текста.
            chunks = chunk_document(result.content, self._settings)
            if not chunks:
                logger.info(
                    "[%s] нет валидных чанков после обезличивания", ref)
                continue
            stats.chunks_built += len(chunks)

            # 5. Локальные эмбеддинги (passage:).
            vectors = self._embedder.embed_passages([c.text for c in chunks])

            # 6. Идемпотентный upsert в Qdrant.
            points: list[PointRecord] = []
            for chunk, vector in zip(chunks, vectors):
                payload = build_payload(
                    chunk.text,
                    doc_type=chunk.doc_type,
                    section=chunk.section,
                    syndrome=chunk.syndrome,
                    diagnosis_class=chunk.diagnosis_class,
                    dynamics=chunk.dynamics,
                    source="corpus",
                )
                points.append(PointRecord(
                    point_id=make_point_id(chunk.text, payload),
                    vector=vector,
                    payload=payload,
                ))
            written = self._store.upsert(points)
            stats.chunks_written += written
            logger.info("[%s] записано чанков: %d (тип записей: daily/exam смешанно)",
                        ref, written)

        return stats
