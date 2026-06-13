# AI MED — RAG-система генерации психиатрических дневников

Веб-приложение для врачей-психиатров: генерация медицинских дневников с опорой
на обезличенный корпус отделения (RAG) и корпоративный LLM. Главный принцип —
**персональные данные пациентов никогда не покидают локальный периметр в открытом виде**.

> Проектная документация (источник правды) — в [`docs/`](docs/README.md).
> Текущее состояние репозитория — **Этап 1 (каркас и инфраструктура)**:
> рабочие `/health`-эндпоинты, поднимающийся `docker compose`, заглушки модулей.
> Бизнес-логика добавляется по [`docs/10_roadmap_stepbystep.md`](docs/10_roadmap_stepbystep.md).

---

## Архитектура (кратко)

См. [`docs/02_system_architecture.md`](docs/02_system_architecture.md).

| Сервис | Технология | Роль | Порт (хост) | Порт (внутри сети) |
|---|---|---|---|---|
| `frontend` | React 19 + Vite + TS | UI, тёмная тема в стиле Cursor | `5174` | `5173` |
| `gateway` | Go (net/http) | API-Gateway + анонимизация + экспорт + оркестрация | `8081` | `8080` |
| `rag` | Python + FastAPI | чанкинг, эмбеддинги, retrieval, ingestion | `8001` | `8000` |
| `postgres` | PostgreSQL 16 | врачи, история, метаданные — **без ПДн** | `5433` | `5432` |
| `qdrant` | Qdrant | векторы обезличенного корпуса | `6333` / `6334` | `6333` / `6334` |

> В колонке «Порт (хост)» — порты по умолчанию, публикуемые на ХОСТ-машине.
> Дефолты намеренно смещены от «популярных» значений, часто уже занятых на машине
> разработчика (особенно на macOS): Postgres `5433` (вместо 5432), RAG `8001`
> (вместо 8000), Gateway `8081` (вместо 8080), Frontend `5174` (вместо 5173).
> Внутри docker-сети контейнеры по-прежнему слушают свои стандартные порты
> (колонка «Порт (внутри сети)»), и сервисы общаются именно по ним
> (`postgres:5432`, `rag:8000`, `gateway:8080` и т.д.).
> Все хост-порты конфигурируются через `*_HOST_PORT` в `.env` (см. `.env.example`).
> При конфликте «port is already allocated» — измените соответствующий `*_HOST_PORT`.

Поток: `frontend → gateway → rag → qdrant`, `gateway → postgres`,
`gateway → X5 CoPilot LLM` (внешний выход, только обезличенный текст).

---

## Быстрый старт (локально)

Требуется Docker + Docker Compose.

```bash
# 1. Скопировать пример окружения и заполнить значения
cp .env.example .env

# 2. Поднять весь стек
docker compose up --build

# 3. Проверить health-эндпоинты (порты — хостовые, см. таблицу выше)
curl http://localhost:8081/health        # gateway → {"status":"ok"}
curl http://localhost:8081/api/v1/health # gateway (под префиксом)
curl http://localhost:8001/health        # rag     → {"status":"ok"}
curl http://localhost:6333/              # qdrant   (dashboard/REST)
# postgres доступен на хосте по порту 5433 (внутри сети — 5432):
#   psql postgres://aimed@localhost:5433/aimed

# 4. Открыть фронтенд
open http://localhost:5174
```

Остановка и очистка томов:

```bash
docker compose down        # остановить
docker compose down -v     # + удалить volume-данные (postgres/qdrant)
```

### Авто-подбор свободных портов

Если даже смещённые дефолты заняты (на машине крутятся другие docker-стеки),
используйте обёртку — она находит первый свободный host-порт начиная с дефолта
и сама прокидывает его в `docker compose` через `*_HOST_PORT`:

```bash
./scripts/dev-up.sh         # = docker compose up --build, но с авто-портами
./scripts/dev-up.sh -d      # в фоне
```

Скрипт печатает итоговые адреса (frontend/gateway/rag/qdrant/postgres) с реально
выбранными портами. Внутренние порты контейнеров и `POSTGRES_DSN` не меняются —
переключаются только публикуемые на хост порты.

---

## Структура репозитория

```
.
├── docker-compose.yml        # оркестрация всех сервисов (Этап 1)
├── scripts/
│   └── dev-up.sh             # авто-подбор свободных host-портов + docker compose up
├── .env.example              # шаблон переменных окружения (плейсхолдеры)
├── deploy/
│   └── initdb/               # SQL-инициализация PostgreSQL (схема + seed, без ПДн)
│       ├── 01_schema.sql
│       └── 02_seed.sql
├── frontend/                 # React 19 + Vite + TS (тёмная тема-заглушка)
│   ├── src/{main.tsx,App.tsx,index.css}
│   └── Dockerfile
├── services/
│   ├── gateway/              # Go: API-Gateway + анонимизация + экспорт
│   │   ├── cmd/gateway/main.go
│   │   ├── internal/{config,handlers,anonymizer,export,auth,ragclient,llm}
│   │   └── Dockerfile
│   └── rag/                  # Python: RAG-сервис (FastAPI)
│       ├── app/{main,config,ingestion,embeddings,retrieval,generation,llm_client}.py
│       ├── requirements.txt
│       └── Dockerfile
├── docs/                     # утверждённое ТЗ (источник правды)
└── Документы/                # ⚠️ реальные мед.данные пациентов — НЕ в git (.gitignore)
```

---

## Приватность

- Папка [`Документы/`](Документы) содержит **реальные ПДн пациентов** и исключена
  из git (см. [`.gitignore`](.gitignore)). Корпус подаётся в ingestion локально.
- В PostgreSQL и Qdrant попадают **только обезличенные** данные
  (см. [`docs/05_data_model.md`](docs/05_data_model.md), [`docs/04_anonymization.md`](docs/04_anonymization.md)).
- Секреты (`X5_API_KEY`, `JWT_SECRET`, пароль БД, корпоративный CA) — в `.env`,
  не в коде (см. [`docs/09_security_privacy.md`](docs/09_security_privacy.md)).

---

## Статус по этапам

Реализован **Этап 0/1 — каркас** (роадмап: [`docs/10_roadmap_stepbystep.md`](docs/10_roadmap_stepbystep.md)).
Следующие этапы: анонимизация → корпус/RAG → LLM+генерация → опросник → auth →
экспорт → полировка UI. Все будущие точки помечены `TODO(этап N)` в коде.
