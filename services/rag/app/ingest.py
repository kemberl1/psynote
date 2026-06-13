"""CLI ingestion корпуса → Qdrant (docs/03 §5, docs/10 Этап 3).

Запуск в docker (one-shot контейнер):

    docker compose run --rm rag python -m app.ingest ingest
    docker compose run --rm rag python -m app.ingest ingest --limit 3   # быстрый smoke
    docker compose run --rm rag python -m app.ingest audit --sample 20   # аудит приватности

Подкоманды:
  ingest  — прогон пайплайна: extract → anonymize(gateway) → chunk → embed → upsert.
  audit   — выгрузить случайную выборку payload.text из Qdrant для ручной проверки
            пользователем на отсутствие ПДн (docs/04 §7, шаг приёмки).

Логи содержат ТОЛЬКО счётчики/статусы и обезличенные source_ref — НИКОГДА сырой
текст, ПДн или имена файлов с ФИО (docs/04 §1, §7).
"""

from __future__ import annotations

import argparse
import logging
import sys
from pathlib import Path

from app.anonymizer_client import AnonymizerClient
from app.config import Settings, get_settings
from app.embeddings import Embedder
from app.ingestion import IngestionPipeline, iter_corpus_files
from app.qdrant_store import QdrantStore

logger = logging.getLogger("ingest")


def _setup_logging(verbose: bool) -> None:
    logging.basicConfig(
        level=logging.DEBUG if verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
        stream=sys.stdout,
    )


def _resolve_corpus_root(settings: Settings) -> Path:
    """Корень для обхода: CORPUS_DIR (+ опционально подпапка дневников)."""
    base = Path(settings.corpus_dir)
    diaries = base / settings.corpus_diaries_subdir
    if diaries.is_dir():
        return diaries
    return base


def cmd_ingest(settings: Settings, args: argparse.Namespace) -> int:
    corpus_root = Path(
        args.corpus_dir) if args.corpus_dir else _resolve_corpus_root(settings)
    if not corpus_root.is_dir():
        logger.error(
            "CORPUS_DIR не найден или не смонтирован: %s", corpus_root)
        return 2

    files = iter_corpus_files(corpus_root, include_tables=args.include_tables)
    if args.limit:
        files = files[: args.limit]
    logger.info("Найдено файлов для обработки: %d (корень: %s, include_tables=%s)",
                len(files), corpus_root, args.include_tables)
    if not files:
        logger.warning("Нет поддерживаемых файлов — нечего индексировать.")
        return 0

    anon = AnonymizerClient(settings)
    if not anon.health_check():
        logger.warning("Gateway недоступен по %s — анонимизация будет fail-closed "
                       "(все документы заблокируются). Проверьте, что сервис поднят.",
                       settings.gateway_url)

    embedder = Embedder(settings)
    store = QdrantStore(settings)

    pipeline = IngestionPipeline(settings, anon, embedder, store)
    try:
        stats = pipeline.run(files, corpus_root)
    finally:
        anon.close()

    logger.info("ГОТОВО. Итог: %s", stats.summary())
    try:
        logger.info("Всего точек в коллекции '%s': %d",
                    settings.qdrant_collection, store.count())
    except Exception as exc:  # noqa: BLE001
        logger.debug("count() недоступен: %s", type(exc).__name__)
    return 0


def cmd_audit(settings: Settings, args: argparse.Namespace) -> int:
    """Выгрузить выборку обезличенных чанков для ручного аудита приватности."""
    store = QdrantStore(settings)
    try:
        samples = store.sample_payloads(limit=args.sample)
    except Exception as exc:  # noqa: BLE001
        logger.error("Не удалось прочитать коллекцию '%s': %s",
                     settings.qdrant_collection, type(exc).__name__)
        return 2

    if not samples:
        logger.info("Коллекция '%s' пуста — сначала выполните ingest.",
                    settings.qdrant_collection)
        return 0

    print("=" * 78)
    print(f"АУДИТ ПРИВАТНОСТИ: выборка из {len(samples)} обезличенных чанков "
          f"коллекции '{settings.qdrant_collection}'")
    print("Проверьте вручную: НЕ должно быть ФИО, дат, адресов, № документов.")
    print("=" * 78)
    for i, payload in enumerate(samples, 1):
        meta = {k: v for k, v in payload.items() if k != "text"}
        print(f"\n--- ЧАНК {i} | {meta} ---")
        print(payload.get("text", ""))
    print("\n" + "=" * 78)
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="python -m app.ingest",
        description="Ingestion корпуса медицинских дневников в Qdrant (Этап 3).",
    )
    parser.add_argument("-v", "--verbose",
                        action="store_true", help="подробный лог")
    sub = parser.add_subparsers(dest="command", required=True)

    p_ingest = sub.add_parser("ingest", help="прогнать пайплайн ingestion")
    p_ingest.add_argument("--corpus-dir", default=None,
                          help="переопределить корень корпуса (иначе из ENV CORPUS_DIR)")
    p_ingest.add_argument("--limit", type=int, default=None,
                          help="обработать не более N файлов (smoke-прогон)")
    p_ingest.add_argument("--include-tables", action="store_true",
                          help="включить .xlsx (по умолчанию только дневники .docx/.odt/.doc)")

    p_audit = sub.add_parser(
        "audit", help="выгрузить выборку чанков для аудита ПДн")
    p_audit.add_argument("--sample", type=int, default=20,
                         help="размер выборки (по умолчанию 20)")
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    _setup_logging(args.verbose)
    settings = get_settings()

    if args.command == "ingest":
        return cmd_ingest(settings, args)
    if args.command == "audit":
        return cmd_audit(settings, args)
    parser.print_help()
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
