# syntax=docker/dockerfile:1

# RAG-сервис (Python / FastAPI). См. docs/02_system_architecture.md §2.1.
# Этап 1 (каркас): лёгкий образ без тяжёлых ML-зависимостей.
FROM python:3.12-slim AS runtime

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PIP_NO_CACHE_DIR=1

# curl нужен для healthcheck.
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --uid 1000 app

WORKDIR /app

COPY requirements.txt ./
RUN pip install --upgrade pip && pip install -r requirements.txt

COPY app ./app

USER app
EXPOSE 8000

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=5 \
    CMD curl -fsS http://127.0.0.1:8000/health || exit 1

# RAG_HOST/RAG_PORT читаются в app/config.py; uvicorn слушает их.
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
