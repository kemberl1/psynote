"""Configuration for the RAG service, loaded from environment variables.

See docs/05_data_model.md §4 (секреты в .env) и docs/03_rag_design.md.

Эмбеддинги считаются ЛОКАЛЬНО (docs/03 §3) — облачный embed-API не используется
для медкорпуса ради приватности (NFR-P3).

КРИТИЧНО (docs/04 §1): анонимизация выполняется ЕДИНСТВЕННЫМ источником истины —
Go-анонимайзером в gateway. Python-ingestion вызывает gateway /api/v1/anonymize
для каждого фрагмента ДО эмбеддинга и записи в Qdrant. URL берётся из ENV.
"""

from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Settings:
    """Runtime settings for the RAG service. Всё конфигурируется через ENV."""

    # ─── Qdrant vector DB (docs/02 §7 — :6333, docs/05 §3) ───────────────────
    qdrant_url: str = os.getenv("QDRANT_URL", "http://qdrant:6333")
    qdrant_collection: str = os.getenv("QDRANT_COLLECTION", "corpus_diaries")

    # ─── Локальная модель эмбеддингов (docs/03 §3 — multilingual-e5-large) ───
    embedding_model: str = os.getenv(
        "EMBEDDING_MODEL", "intfloat/multilingual-e5-large")
    # Размерность вектора e5-large = 1024 (docs/05 §3.2). Для -base = 768.
    # Если поменяли модель — поправьте размерность (или 0 = определить по модели).
    embedding_dim: int = int(os.getenv("EMBEDDING_DIM", "1024"))
    # Кэш HF-моделей (монтируется volume, чтобы не качать модель при каждом ране).
    hf_home: str = os.getenv("HF_HOME", "/app/.cache/huggingface")
    # Устройство инференса: cpu | cuda. В docker по умолчанию cpu.
    embedding_device: str = os.getenv("EMBEDDING_DEVICE", "cpu")
    embedding_batch_size: int = int(os.getenv("EMBEDDING_BATCH_SIZE", "16"))

    # ─── Анонимайзер-гейт (Go gateway, docs/04 §1) ───────────────────────────
    # URL gateway внутри docker-сети. НЕ хардкодим (docs/04, критичный принцип).
    gateway_url: str = os.getenv("GATEWAY_URL", "http://gateway:8080")
    anonymize_path: str = os.getenv("ANONYMIZE_PATH", "/api/v1/anonymize")
    # Таймауты/ретраи HTTP-клиента к анонимайзеру.
    anonymize_timeout_s: float = float(os.getenv("ANONYMIZE_TIMEOUT_S", "30"))
    anonymize_retries: int = int(os.getenv("ANONYMIZE_RETRIES", "3"))
    anonymize_backoff_s: float = float(os.getenv("ANONYMIZE_BACKOFF_S", "1.0"))

    # ─── Корпус (docs/03 §5). Монтируется volume read-only, НЕ копируется в образ. ─
    corpus_dir: str = os.getenv("CORPUS_DIR", "/data/corpus")
    # Подпапка с дневниками внутри корпуса. Используется ТОЛЬКО при scope="subdir"
    # как опциональное сужение охвата (исторический фокус Этапа 3, docs/03 §4).
    corpus_diaries_subdir: str = os.getenv(
        "CORPUS_DIARIES_SUBDIR", "02_корпус/сборник_дневников_ИБ")

    # ─── Этап 4.1: охват ingestion на ВЕСЬ корпус-дневников (docs/03 §5–6) ────
    # Дневники РАЗБРОСАНЫ по всему корпусу (сборник_дневников_ИБ,
    # заготовки_дневников, выписанные/<ФИО>/…дневники…, истории/<ФИО>/…дневники…,
    # текущие_пациенты/<ФИО>/Дневники…). По умолчанию обходим ВЕСЬ корпус
    # рекурсивно и отбираем файлы-ДНЕВНИКИ; "subdir" — историческое сужение до
    # corpus_diaries_subdir.
    corpus_diaries_scope: str = os.getenv(
        "CORPUS_DIARIES_SCOPE", "all")  # all | subdir
    # Критерий «это дневник» по ИМЕНИ файла (регистронезависимый regex). Дефолт
    # ловит «дневник»/«дневники»/«Дневники» и т.п.
    corpus_diary_name_re: str = os.getenv("CORPUS_DIARY_NAME_RE", r"дневник")
    # Папки, ВСЕ файлы которых считаются дневниками (по имени каталога в пути).
    # Список через запятую; сравнение регистронезависимое.
    corpus_diary_dirs: str = os.getenv(
        "CORPUS_DIARY_DIRS", "сборник_дневников_ИБ,заготовки_дневников")
    # Регистронезависимый regex имён НЕ-дневниковых типов документов, которые на
    # этом этапе НЕ индексируем (первички/эпикризы/выписки/листы назначений/
    # статкарты). Это страховка для папочной выборки и точка расширения под
    # будущие типы (docs/03 §11 — отдельные коллекции). Файлы с явным «дневник»
    # в имени НЕ отсеиваются этим фильтром.
    corpus_exclude_name_re: str = os.getenv(
        "CORPUS_EXCLUDE_NAME_RE",
        r"первичк|первичн|эпикриз|выписк|лист[ _]*назнач|назначени|статкарт|статистическ")

    # ─── Чанкинг (docs/03 §4) ────────────────────────────────────────────────
    # Максимальный размер чанка в символах (мягкая граница; режем по секциям/записям).
    chunk_max_chars: int = int(os.getenv("CHUNK_MAX_CHARS", "1200"))
    chunk_overlap_chars: int = int(os.getenv("CHUNK_OVERLAP_CHARS", "150"))
    # Минимальный размер чанка — короче отбрасываем как шум (подписи и т.п.).
    chunk_min_chars: int = int(os.getenv("CHUNK_MIN_CHARS", "40"))

    # ─── LLM: корпоративный X5 CoPilot (OpenAI-совместимый, docs/03 §9) ───────
    # Этап 4: автоматический фолбэк large→medium→small (docs/03 §10).
    # ВАЖНО: ключ только из ENV, никогда не в коде/логах (docs/03 §9.2, docs/09).
    x5_base_url: str = os.getenv(
        "X5_BASE_URL", "https://api-copilot.x5.ru/aigw/v1/")
    x5_api_key: str = os.getenv("X5_API_KEY", "")
    # Путь к PEM с корпоративным CA X5 ВНУТРИ контейнера (docs/03 §9.2).
    # TLS-верификация ОСТАЁТСЯ включённой — указываем кастомный bundle, не отключаем.
    llm_ca_bundle: str = os.getenv("LLM_CA_BUNDLE", "")
    # Список моделей с приоритетом фолбэка. Пустые значения отбрасываются —
    # порядок задаётся ENV без пересборки образа (docs/03 §10).
    llm_model_large: str = os.getenv("LLM_MODEL_LARGE", "x5-airun-large")
    llm_model_medium: str = os.getenv("LLM_MODEL_MEDIUM", "x5-airun-medium")
    llm_model_small: str = os.getenv("LLM_MODEL_SMALL", "x5-airun-small")
    # Тайм-аут одного запроса к LLM (сек) и число ретраев ВНУТРИ одной модели.
    llm_timeout_s: float = float(os.getenv("LLM_TIMEOUT_S", "60"))
    llm_max_retries: int = int(os.getenv("LLM_MAX_RETRIES", "3"))
    # Backoff: начальная задержка и максимум (экспоненциальный с джиттером).
    llm_backoff_initial_s: float = float(
        os.getenv("LLM_BACKOFF_INITIAL_S", "1.0"))
    llm_backoff_max_s: float = float(os.getenv("LLM_BACKOFF_MAX_S", "30.0"))
    # Низкая температура — предсказуемость и юр.аккуратность (docs/03 §8).
    llm_temperature: float = float(os.getenv("LLM_TEMPERATURE", "0.4"))
    llm_max_tokens: int = int(os.getenv("LLM_MAX_TOKENS", "2048"))

    # ─── Retrieval для генерации (docs/03 §6) ────────────────────────────────
    # Число few-shot образцов из корпуса (k=4–6 по docs/03 §6).
    retrieval_top_k: int = int(os.getenv("RETRIEVAL_TOP_K", "5"))

    # ─── HTTP server ─────────────────────────────────────────────────────────
    host: str = os.getenv("RAG_HOST", "0.0.0.0")
    port: int = int(os.getenv("RAG_PORT", "8000"))

    def llm_models(self) -> list[str]:
        """Упорядоченный список моделей для фолбэка (large→medium→small).

        Пустые значения отбрасываются. Дубликаты схлопываются с сохранением
        порядка — позволяет, напр., временно указать одну и ту же модель.
        """
        ordered = [self.llm_model_large,
                   self.llm_model_medium, self.llm_model_small]
        seen: set[str] = set()
        result: list[str] = []
        for name in ordered:
            name = (name or "").strip()
            if name and name not in seen:
                seen.add(name)
                result.append(name)
        return result


def get_settings() -> Settings:
    """Return the process settings (single instance is fine here)."""
    return Settings()
