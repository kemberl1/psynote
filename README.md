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

## Ingestion корпуса в Qdrant (Этап 3)

Пайплайн индексации обезличенного корпуса дневников в векторную БД Qdrant.
Строгий порядок (приватность, [`docs/04_anonymization.md`](docs/04_anonymization.md) §1):
**парсинг → анонимизация через gateway `/api/v1/anonymize` → чанкинг
обезличенного текста → локальные эмбеддинги e5 → upsert в Qdrant**. Сырой текст
с ПДн и имена файлов с ФИО НИКОГДА не попадают в Qdrant/логи/репозиторий.

```bash
# 0. Поднять зависимости пайплайна (Qdrant + Go-анонимайзер gateway)
docker compose up -d qdrant gateway

# 1. Полный прогон ingestion (one-shot контейнер; первый запуск качает модель e5 ~2 ГБ)
docker compose run --rm rag python -m app.ingest ingest

# 1а. Быстрый smoke-прогон на нескольких документах
docker compose run --rm rag python -m app.ingest ingest --limit 3

# 1б. Включить .xlsx (листы назначений) — по умолчанию только дневники .docx/.odt/.doc
docker compose run --rm rag python -m app.ingest ingest --include-tables

# 2. АУДИТ ПРИВАТНОСТИ: выгрузить выборку обезличенных чанков для ручной проверки
docker compose run --rm rag python -m app.ingest audit --sample 20
#    либо через FastAPI-эндпоинт:
curl "http://localhost:8001/admin/audit-sample?sample=20"
```

- Корпус монтируется в контейнер `rag` **только для чтения** (`./Документы:/data/corpus:ro`).
- Идемпотентность: id чанка детерминирован (UUIDv5 от обезличенного текста + метаданных),
  повторный прогон не плодит дубликаты.
- Логи содержат только счётчики (`removed_by_type`, `blocked_pii`, `chunks_written`)
  и обезличенный `source_ref` — без ПДн.
- Подробности дизайна — [`docs/03_rag_design.md`](docs/03_rag_design.md) §4–5,
  [`docs/05_data_model.md`](docs/05_data_model.md) §3.

---

## Генерация дневника (Этап 4) — RAG + LLM X5 с автофолбэком

Главный эндпоинт продукта: по ответам опросника + типу дневника собирается промпт
из **шаблона** (жёсткий каркас разделов), **few-shot образцов** из Qdrant (стиль
корпуса) и **ответов опросника** (содержание), затем вызывается корпоративный LLM
X5 CoPilot с **автоматическим фолбэком** моделей `large → medium → small`
(см. [`docs/03_rag_design.md`](docs/03_rag_design.md) §6–10,
[`docs/07_api_contract.md`](docs/07_api_contract.md) §5).

Конвейер: **анонимизация свободного ввода через gateway-гейт → retrieval из
Qdrant → сборка промпта (daily/exam_10d) → LLM с фолбэком → обезличенный ответ**.
Реализация в RAG-сервисе: [`llm_client.py`](services/rag/app/llm_client.py:1),
[`generation.py`](services/rag/app/generation.py:1),
[`questionnaire.py`](services/rag/app/questionnaire.py:1),
[`templates.py`](services/rag/app/templates.py:1),
[`pipeline.py`](services/rag/app/pipeline.py:1).

### Запуск РЕАЛЬНОЙ генерации (нужен ключ X5 + корпоративный CA)

```bash
# 1. Прописать секреты в .env (НЕ коммитить!):
#    X5_API_KEY=<ваш Bearer-ключ X5 CoPilot>
#    X5_BASE_URL=https://api-copilot.x5.ru/aigw/v1/
#    LLM_MODEL_LARGE=x5-airun-large   (порядок фолбэка large→medium→small)
#    LLM_MODEL_MEDIUM=x5-airun-medium
#    LLM_MODEL_SMALL=x5-airun-small

# 2. Положить корпоративный CA X5 (PEM) и подключить его:
#    cp <ваш x5_root_ca.pem> deploy/certs/x5_root_ca.pem      # папка вне git
#    В docker-compose.yml (сервис rag) раскомментировать volume:
#      - ./deploy/certs/x5_root_ca.pem:/app/certs/x5_root_ca.pem:ro
#    В .env: LLM_CA_BUNDLE=/app/certs/x5_root_ca.pem
#    (TLS-верификация ОСТАЁТСЯ включённой — указываем доверенный bundle)

# 3. Поднять зависимости и сам RAG:
docker compose up -d qdrant gateway rag

# 4. (опц., для few-shot) проиндексировать корпус — см. раздел Ingestion выше:
docker compose run --rm rag python -m app.ingest ingest

# 5. Проверить, что LLM сконфигурирован:
curl http://localhost:8001/health
#   → {"status":"ok","llm":{"configured":true,"models":["x5-airun-large",...]}, ...}
```

### Пример запроса — ЕЖЕДНЕВНЫЙ дневник (`daily`)

```bash
curl -sS -X POST http://localhost:8001/generate \
  -H 'Content-Type: application/json' \
  -d '{
    "document_type": "daily",
    "answers": {
      "dynamics": "no_change",
      "productive_symptoms": "not_detected",
      "mood": "lowered",
      "mood_detail": ["anxiety", "tearfulness"],
      "behavior": "ordered",
      "contact": ["productive", "polite_staff"],
      "sleep": "hard_to_fall_asleep",
      "appetite": "decreased",
      "tolerance": "good",
      "complaints": "none"
    }
  }'
```

### Пример запроса — ОСМОТР раз в 10 дней (`exam_10d`)

```bash
curl -sS -X POST http://localhost:8001/generate \
  -H 'Content-Type: application/json' \
  -d '{
    "document_type": "exam_10d",
    "answers": {
      "dynamics": "positive",
      "mood": "even",
      "behavior": "ordered",
      "sleep": "not_disturbed",
      "appetite": "preserved",
      "physical_status": "unremarkable",
      "neuro_status": "no_acute",
      "criticism": "forming",
      "suicidal": "not_detected",
      "diagnosis": "F84.11 Атипичный аутизм",
      "syndrome": "психопатоподобный",
      "period_dynamics": "improvement",
      "prescriptions": "см. лист назначений"
    }
  }'
```

Свободный текст (поля `*_detail`, `diagnosis`, кастомные «свой вариант»
`{"value":"__custom__","custom_text":"..."}`) **автоматически прогоняется через
анонимайзер-гейт** перед отправкой в LLM.

### Ожидаемый ответ (конверт по [`docs/07`](docs/07_api_contract.md) §1, §5)

```jsonc
{
  "meta": {
    "request_id": "uuid",
    "ts": "ISO8601",
    "llm_model_used": "x5-airun-large",   // какая модель отработала (после фолбэка)
    "tokens_used": 812,
    "chunks_used": 5                       // сколько few-shot образцов использовано
  },
  "data": {
    "document_type": "daily",
    "content": "обезличенный текст дневника с плейсхолдерами [ДАТА], [ФИО_ВРАЧА]...",
    "status": "done",
    "title_safe": "Ежедневный дневник · ...",
    "answers_anonymized": { /* ответы после гейта */ },
    "anonymizer_removed_count": 0,
    "retrieval": { "chunks_used": 5, "syndrome": null, "diagnosis_class": null, "dynamics": "без_динамики" }
  }
}
```

Коды ошибок: `400` неизвестный `document_type`; `422` `PII_DETECTED` (гейт
заблокировал свободный ввод); `503` `LLM_UNAVAILABLE` (все модели фолбэка
недоступны) / `LLM_NOT_CONFIGURED` (нет ключа) / `LLM_AUTH_ERROR` (401/403 —
проверьте `X5_API_KEY`, без ретраев и без перебора моделей).

### Тесты Этапа 4 (с моками LLM, без сети)

```bash
cd services/rag && python -m pytest tests/test_llm_client.py tests/test_pipeline.py -q
# 19 passed — фолбэк large→medium→small, без ретраев на 401/403, «все модели
# недоступны», анонимизация ввода перед промптом, сборка промптов daily/exam_10d.
```

> ОТКЛОНЕНИЕ от [`docs/02/03`](docs/03_rag_design.md): по исходному дизайну LLM-клиент
> с фолбэком предполагался в Go-Gateway. По заданию Этапа 4 он реализован в
> RAG-сервисе (Python — «План Б» из docs/03 §9), чтобы весь конвейер генерации был
> в одном месте. Эндпоинт публикуется как `POST /generate` (gateway на следующих
> этапах проксирует `POST /api/v1/generate` → `rag:8000/generate`). Конверт ответа
> и коды ошибок сохранены по docs/07 §1.

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

Реализованы **Этапы 0–4** (роадмап: [`docs/10_roadmap_stepbystep.md`](docs/10_roadmap_stepbystep.md)):
каркас → анонимизация (gateway) → ingestion корпуса/RAG → **генерация дневников
(RAG-retrieval + LLM X5 с автофолбэком, `POST /generate`)**.
Следующие этапы: фронтенд → динамический опросник-UI → экспорт docx/pdf → auth.
