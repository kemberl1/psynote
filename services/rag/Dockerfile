# syntax=docker/dockerfile:1

# RAG-сервис (Python / FastAPI). См. docs/02_system_architecture.md §2.1.
# Этап 3 (ingestion): добавлены ML-зависимости (torch, sentence-transformers)
# и antiword для извлечения текста из старого формата .doc (docs/03 §5).
FROM python:3.12-slim AS runtime

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PIP_NO_CACHE_DIR=1 \
    # Кэш HF-моделей (монтируется volume в compose, чтобы не качать модель
    # при каждом запуске one-shot ingestion-контейнера, docs/03 §3).
    HF_HOME=/app/.cache/huggingface

# curl — для healthcheck; antiword — для извлечения текста из .doc (docs/03 §5).
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl antiword \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --uid 1000 app

WORKDIR /app

COPY requirements.txt ./
RUN pip install --upgrade pip && pip install -r requirements.txt

COPY app ./app

# Каталог кэша моделей с правами пользователя app (volume монтируется сюда).
RUN mkdir -p /app/.cache/huggingface && chown -R app:app /app/.cache

USER app
EXPOSE 8000

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=5 \
    CMD curl -fsS http://127.0.0.1:8000/health || exit 1

# RAG_HOST/RAG_PORT читаются в app/config.py; uvicorn слушает их.
# Ingestion запускается отдельно: `docker compose run --rm rag python -m app.ingest ingest`.
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
