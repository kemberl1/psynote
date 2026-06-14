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
import re
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
    """Собрать поддерживаемые файлы корпуса (рекурсивно), БЕЗ отбора по типу.

    Низкоуровневый обход: только фильтр по расширению и временным файлам Office
    (`~$...`). Отбор именно ДНЕВНИКОВ выполняет :func:`select_diary_files`.
    """
    suffixes = set(SUPPORTED_SUFFIXES)
    if not include_tables:
        suffixes -= SUPPORTED_TABLE_SUFFIXES
    files: list[Path] = []
    for p in sorted(root.rglob("*")):
        if p.is_file() and p.suffix.lower() in suffixes and not p.name.startswith("~$"):
            files.append(p)
    return files


@dataclass(frozen=True)
class DiarySelector:
    """Конфигурируемый отбор файлов-ДНЕВНИКОВ из дерева корпуса (Этап 4.1).

    Критерий «это дневник» (docs/03 §4–5):
      • имя файла матчит ``name_re`` (по умолчанию «дневник», регистронезависимо), ИЛИ
      • файл лежит в одной из «дневниковых» папок ``diary_dirs``
        (``сборник_дневников_ИБ`` / ``заготовки_дневников``).

    Отсев (НЕ индексируем на этом этапе — первички/эпикризы/выписки/листы
    назначений/статкарты): если имя матчит ``exclude_re`` И при этом НЕ матчит
    ``name_re`` (явный «дневник» в имени имеет приоритет и не отсеивается).

    Отбор СОЗНАТЕЛЬНО расширяем: ``exclude_re``/``name_re``/``diary_dirs``
    задаются через ENV — будущие типы документов (docs/03 §11) добавляются без
    правки кода (отдельной коллекцией/настройкой).

    ПРИВАТНОСТЬ: ни имена файлов, ни сегменты пути (папки `выписанные/<ФИО>/…`
    содержат ПДн) НЕ логируются и НЕ попадают в payload — используется только
    обезличенный source_ref (sha256 относительного пути).
    """

    name_re: re.Pattern[str]
    exclude_re: re.Pattern[str] | None
    diary_dirs: frozenset[str]

    @classmethod
    def from_settings(cls, settings: Settings) -> "DiarySelector":
        name_re = re.compile(settings.corpus_diary_name_re, re.IGNORECASE)
        exclude_raw = (settings.corpus_exclude_name_re or "").strip()
        exclude_re = re.compile(
            exclude_raw, re.IGNORECASE) if exclude_raw else None
        dirs = frozenset(
            d.strip().casefold()
            for d in settings.corpus_diary_dirs.split(",")
            if d.strip()
        )
        return cls(name_re=name_re, exclude_re=exclude_re, diary_dirs=dirs)

    def _in_diary_dir(self, path: Path) -> bool:
        parts = {p.casefold() for p in path.parts}
        return bool(self.diary_dirs & parts)

    def is_diary(self, path: Path) -> bool:
        """True, если файл следует индексировать как ДНЕВНИК на этом этапе."""
        name = path.name
        name_hit = bool(self.name_re.search(name))
        dir_hit = self._in_diary_dir(path)
        if not (name_hit or dir_hit):
            return False
        # Явный «дневник» в имени побеждает отсев; иначе применяем exclude.
        if not name_hit and self.exclude_re is not None and self.exclude_re.search(name):
            return False
        return True


def select_diary_files(files: list[Path], selector: DiarySelector) -> list[Path]:
    """Отфильтровать список файлов до файлов-дневников (детерминированно)."""
    return [p for p in files if selector.is_diary(p)]


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
