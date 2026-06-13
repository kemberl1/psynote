"""AI MED RAG service (Python / FastAPI).

Ответственность сервиса (см. docs/02_system_architecture.md §2.1):
чанкинг медицинских документов, локальные эмбеддинги, retrieval из Qdrant,
построение промпта, ingestion корпуса.

Этап 1 (каркас): рабочий /health-эндпоинт + заглушки модулей.
"""

__version__ = "0.1.0"
