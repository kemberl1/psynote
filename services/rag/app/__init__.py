"""AI MED RAG service (Python / FastAPI).

Ответственность сервиса (см. docs/02_system_architecture.md §2.1):
чанкинг медицинских документов, локальные эмбеддинги, retrieval из Qdrant,
построение промпта, ingestion корпуса.

Этап 3 (ingestion): реализован пайплайн extract → anonymize(gateway) →
chunk → embed(e5 локально) → upsert в Qdrant + CLI и аудит приватности.
"""

__version__ = "0.3.0"
