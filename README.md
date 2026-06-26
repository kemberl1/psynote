<!-- PsyNote — README -->
<!-- Badges row -->
<p align="center">
  <img src="https://img.shields.io/badge/Go-1.23-00ADD8?logo=go&logoColor=white" alt="Go 1.23" />
  <img src="https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black" alt="React 19" />
  <img src="https://img.shields.io/badge/Python-3.12-3776AB?logo=python&logoColor=white" alt="Python 3.12" />
  <img src="https://img.shields.io/badge/FastAPI-0.115-009688?logo=fastapi&logoColor=white" alt="FastAPI" />
  <img src="https://img.shields.io/badge/TypeScript-5.7-3178C6?logo=typescript&logoColor=white" alt="TypeScript" />
  <img src="https://img.shields.io/badge/Docker_Compose-2496ED?logo=docker&logoColor=white" alt="Docker Compose" />
  <img src="https://img.shields.io/badge/PostgreSQL-16-4169E1?logo=postgresql&logoColor=white" alt="PostgreSQL 16" />
  <img src="https://img.shields.io/badge/Qdrant-Vector_DB-DC244C?logo=qdrant&logoColor=white" alt="Qdrant" />
  <img src="https://img.shields.io/badge/License-MIT-green" alt="MIT License" />
  <img src="https://img.shields.io/badge/ИТМО-Go--разработчик-blueviolet" alt="ИТМО Go-разработчик" />
</p>

<h1 align="center">🧠 PsyNote</h1>
<p align="center">
  <b>RAG-система автоматизации медицинской документации для врачей-психиатров</b><br/>
  Курсовой проект · ИТМО, курс «Go-разработчик» · 2025–2026
</p>

<p align="center">
  <a href="#-архитектура">Архитектура</a> ·
  <a href="#-стек-технологий">Стек</a> ·
  <a href="#-ключевые-фичи">Фичи</a> ·
  <a href="#-быстрый-старт">Быстрый старт</a> ·
  <a href="#-go-компоненты">Go-компоненты</a> ·
  <a href="#roadmap">Roadmap</a>
</p>

---

## 📖 Описание

**PsyNote** — веб-приложение, позволяющее детскому врачу-психиатру за **2–3 минуты** сгенерировать структурированный медицинский дневник (ежедневный / осмотр 10 дней) с опорой на реальный обезличённый корпус отделения.

### Проблема

Врачи-психиатры тратят **30–60 % рабочего времени** на рутинное заполнение дневников, осмотров и эпикризов. Тексты шаблонны, но требуют индивидуального контекста. Существующие решения (шаблоны Word, ChatGPT) либо не учитывают стиль конкретного отделения, либо **нарушают ФЗ-152** о персональных данных.

### Решение

PsyNote совмещает:

- **Динамический опросник** — врач отвечает на структурированные вопросы вместо набора текста с нуля.
- **RAG (Retrieval-Augmented Generation)** — перед генерацией находит в векторной базе похожие реальные записи из корпуса отделения, «подкладывая» LLM стиль и терминологию.
- **Корпоративный LLM (X5 CoPilot)** — генерирует финальный текст дневника.
- **Многоуровневую анонимизацию** — все персональные данные (ФИО, даты, адреса, названия учреждений) заменяются плейсхолдерами **до** попадания в LLM и векторную БД.

> **Принцип:** персональные данные пациентов **никогда** не покидают локальный периметр в открытом виде.

> ⚠️ Автор проекта — frontend-разработчик, студент ИТМО, курс «Go-разработчик». Врач-психиатр выступил заказчиком и экспертом предметной области.

---

## 🏗 Архитектура

Микросервисная архитектура (3 сервиса + 3 хранилища), оркестрируемая через **Docker Compose**.

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Браузер врача                                │
│                   React 19 + Vite + TypeScript                      │
└──────────────────────────────┬──────────────────────────────────────┘
                               │ REST/JSON + JWT
                               ▼
┌──────────────────────────────────────────────────────────────────────┐
│              API-Gateway  (Go 1.23 · net/http)                       │
│  ┌──────────┐ ┌──────────────┐ ┌────────┐ ┌────────┐ ┌───────────┐ │
│  │   Auth   │ │ Anonymizer   │ │ Export │ │  RAG   │ │  Catalog  │ │
│  │  (JWT)   │ │ (PII-gate)   │ │DOCX/PDF│ │Orchest.│ │(question- │ │
│  │          │ │ 5 стадий     │ │        │ │        │ │  naires)  │ │
│  └──────────┘ └──────────────┘ └────────┘ └────┬───┘ └───────────┘ │
└─────────────────────────────────────────────────┼────────────────────┘
                               ┌──────────────────┼──────────────┐
                               │                  │              │
                    ┌──────────▼───┐    ┌─────────▼────┐  ┌──────▼───────┐
                    │  PostgreSQL  │    │  Qdrant      │  │  RAG-сервис  │
                    │  (metadata,  │    │  (векторы    │  │  (Python     │
                    │  users,      │    │  корпуса,    │  │  FastAPI)    │
                    │  history)    │    │  поиск)      │  │  чанкинг,    │
                    │  — без ПДн!  │    │              │  │  retrieval,  │
                    └──────────────┘    └──────────────┘  │  prompt,     │
                                                          │  generation  │
                                                          └──────────────┘
                                                                  │
                                                          обезличенный текст
                                                                  ▼
                                                    ┌─────────────────────┐
                                                    │  X5 CoPilot LLM    │
                                                    │  (корпоративный)    │
                                                    │  large→medium→small │
                                                    │  fallback           │
                                                    └─────────────────────┘
```

Подробная архитектура (C4-модели, Mermaid-диаграммы) — [`docs/02_system_architecture.md`](docs/02_system_architecture.md).

---

## 🛠 Стек технологий

| Компонент | Технология | Роль |
|---|---|---|
| **API-Gateway** | Go 1.23 (`net/http`) | Маршрутизация, аутентификация (JWT HS256), анонимизация PII, оркестрация RAG↔LLM, экспорт DOCX/PDF |
| **Анонимизатор** | Go (in-process) | 5-стадийный пайплайн замены ПДн: стоп-слова → FIO-детектор → словари учреждений → regex-паттерны → NER (Natasha, опц.) |
| **RAG-сервис** | Python 3.12, FastAPI | Чанкинг корпуса, локальные эмбеддинги (e5-large), векторный поиск (Qdrant), сборка промптов, генерация через LLM |
| **Frontend** | React 19, Vite 6, TypeScript 5.7 | Динамический опросник, рабочая область дневников, история запросов, экспорт, тёмная тема в стиле Cursor |
| **PostgreSQL** | PostgreSQL 16 | Пользователи, сессии, история запросов, метаданные документов (**без ПДн**) |
| **Qdrant** | Qdrant v1.12 | Векторное хранилище чанков обезличённого корпуса |
| **Embeddings** | `multilingual-e5-large` (sentence-transformers) | Локальные эмбеддинги — текст пациентов **не уходит** в облако |
| **LLM** | X5 CoPilot API (OpenAI-совместимый) | Генерация текста дневника; автоматический фолбэк `large → medium → small` |
| **Оркестрация** | Docker Compose | Единый `docker compose up --build` для всего стека |

---

## ✨ Ключевые фичи

| Фича | Статус | Описание |
|---|---|---|
| 🩺 **Динамический опросник** | ✅ Этап 4 | Структурированные вопросы вместо свободного набора; ответы подставляются в промпт |
| 🤖 **RAG-генерация дневников** | ✅ Этап 3–4 | Retrieval из Qdrant (few-shot) + шаблон + LLM → структурированный дневник |
| 🔒 **Многоуровневая анонимизация** | ✅ Этап 1 | 5 стадий: словари ФИО, regex дат/адресов, словари учреждений, NER, fail-closed гейт |
| 🔄 **LLM-фолбэк** | ✅ Этап 4 | Автоматический fallback моделей: `large → medium → small` при ошибках/таймаутах |
| 📄 **Экспорт Word/PDF** | ✅ Этап 6 | DOCX и PDF в формате корпусного сборника; одиночный и пакетный экспорт |
| 🔐 **Аутентификация (JWT)** | ✅ Этап 5 | Access + Refresh токены, bcrypt-хеши паролей, middleware авторизации |
| 👤 **Админ-панель** | ✅ Этап 5 | Управление врачами, просмотр статистики использования |
| 📋 **История запросов** | ✅ Этап 5 | Сохранение и просмотр ранее сгенерированных дневников |
| 📅 **Пакетная генерация** | ✅ | Дневники за период одним проходом; дни 10/20/30… → осмотр раз в 10 дней |
| 📊 **Ingestion корпуса** | ✅ Этап 3 | Парсинг .docx/.odt/.xlsx → анонимизация → чанкинг → эмбеддинги → Qdrant |

---

## 🚀 Быстрый старт

### Требования

- Docker + Docker Compose v2
- 4+ ГБ RAM (для модели эмбеддингов ~2 ГБ кэш при первом ingestion)
- Порт `5174`, `8081`, `8001`, `6333`, `5433` свободны на хосте (или измените в `.env`)

### Шаг 1 — Клонировать и настроить окружение

```bash
git clone https://github.com/<your-username>/PsyNote.git
cd PsyNote

# Скопировать шаблон переменных окружения и заполнить секреты
cp .env.example .env
# Отредактируйте .env:
#   — POSTGRES_PASSWORD (сильный пароль)
#   — JWT_SECRET        (openssl rand -base64 48)
#   — X5_API_KEY        (ключ корпоративного LLM; пусто = заглушка)
```

### Шаг 2 — Поднять весь стек

```bash
docker compose up --build
```

### Шаг 3 — Проверить health-эндпоинты

```bash
curl http://localhost:8081/health        # gateway → {"status":"ok"}
curl http://localhost:8001/health        # rag     → {"status":"ok"}
curl http://localhost:6333/              # qdrant  (REST dashboard)
# postgres: psql postgres://aimed:***@localhost:5433/aimed
```

### Шаг 4 — Открыть фронтенд

```bash
open http://localhost:5174
```

### Остановка и очистка

```bash
docker compose down        # остановить контейнеры
docker compose down -v     # + удалить volume-данные (postgres, qdrant, HF-кэш)
```

### Авто-подбор свободных портов

Если порты по умолчанию заняты другим docker-стеком:

```bash
./scripts/dev-up.sh         # = docker compose up --build, но с авто-поиском свободных портов
./scripts/dev-up.sh -d      # в фоне (detached)
```

### Ingestion корпуса (RAG-индексация)

```bash
# 1. Убедитесь, что корпус монтирован (см. volumes в docker-compose.yml)
#    Папка ./Документы/ монтируется read-only → /data/corpus

# 2. Полный прогон ingestion (первый запуск скачивает модель e5 ~2 ГБ)
docker compose run --rm rag python -m app.ingest ingest

# 3. Smoke-прогон на нескольких документах
docker compose run --rm rag python -m app.ingest ingest --limit 3

# 4. Аудит приватности — выгрузка обезличённых чанков для ручной проверки
docker compose run --rm rag python -m app.ingest audit --sample 20
```

> Корпус монтируется **только для чтения** (`:ro`). Сырой текст с ПДн **никогда** не попадает в Qdrant, логи или репозиторий.

### Пакетная генерация за период

В UI: вкладка **«Пакет за период»** (`/diary/batch`).

1. Укажите **дату поступления** (обязательно) и период **с — по** (до 31 дня).
2. Заполните **сжатый опросник** (динамика, сон, аппетит и т.д.) и при необходимости — свободный контекст.
3. Нажмите «Сгенерировать пакет». Дни **10, 20, 30…** от поступления автоматически получают шаблон `exam_10d`, остальные — `daily`.
4. Каждый день — отдельный вызов `POST /api/v1/generate`; результаты сохраняются в истории.

Подробнее: [`docs/08_ui_ux.md`](docs/08_ui_ux.md) §5a.

### Пример запроса генерации дневника

```bash
curl -sS -X POST http://localhost:8081/api/v1/generate \
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

---

## 📁 Структура проекта

```
PsyNote/
├── README.md                        # ← вы здесь
├── docker-compose.yml               # Оркестрация всех сервисов
├── .env.example                     # Шаблон переменных окружения
├── .gitignore
│
├── deploy/
│   └── initdb/                      # SQL-инициализация PostgreSQL
│       ├── 01_schema.sql            # Схема БД (врачи, сессии, история)
│       ├── 02_seed.sql              # Seed-данные
│       ├── 03_migration_*.sql       # Миграции
│       ├── 04_auth_session_index.sql
│       └── 05_admin_role.sql
│
├── frontend/                        # 🎨 React 19 + Vite + TypeScript
│   ├── src/
│   │   ├── api/                     # HTTP-клиент, endpoints, types, queries
│   │   ├── auth/                    # AuthContext, ProtectedRoute, AdminRoute
│   │   ├── components/
│   │   │   ├── layout/              # AppShell, TopBar, HistorySidebar
│   │   │   ├── questionnaire/       # QuestionnaireRenderer, QuestionField
│   │   │   ├── result/              # DocumentView, GenerationResult
│   │   │   └── ui/                  # Примитивы UI (Button, Input, Modal, Toast)
│   │   ├── pages/                   # DiaryPage, BatchDiaryPage, AuthPage, AdminPage, RequestDetailPage
│   │   ├── lib/                     # Утилиты: clipboard, download, format, batchDiary
│   │   ├── App.tsx
│   │   └── main.tsx
│   ├── Dockerfile
│   └── package.json
│
├── services/
│   ├── gateway/                     # 🚀 Go-сервис (API-Gateway)
│   │   ├── cmd/gateway/main.go      # Точка входа
│   │   ├── internal/
│   │   │   ├── anonymizer/          # 5-стадийный PII-гейт + словари
│   │   │   ├── auth/                # JWT, bcrypt-пароли
│   │   │   ├── catalog/             # Управление опросниками (каталог)
│   │   │   ├── config/              # Конфигурация из ENV
│   │   │   ├── export/              # DOCX (gofpdf) + PDF-генерация
│   │   │   ├── handlers/            # HTTP-хендлеры + middleware
│   │   │   ├── llm/                 # LLM-клиент (X5 CoPilot)
│   │   │   ├── ragclient/           # Клиент RAG-сервиса
│   │   │   └── store/               # PostgreSQL (pgx v5)
│   │   └── Dockerfile
│   │
│   └── rag/                         # 🐍 Python-сервис (RAG)
│       ├── app/
│       │   ├── main.py              # FastAPI application
│       │   ├── chunking.py          # Чанкинг обезличённого текста
│       │   ├── embeddings.py        # Локальные эмбеддинги e5-large
│       │   ├── retrieval.py         # Векторный поиск в Qdrant
│       │   ├── generation.py        # Сборка промптов (daily/exam_10d)
│       │   ├── llm_client.py        # OpenAI SDK → X5 CoPilot + фолбэк
│       │   ├── pipeline.py          # End-to-end: retrieval → prompt → LLM
│       │   ├── ingest.py / ingestion.py  # Ingestion корпуса
│       │   ├── anonymizer_client.py # Клиент gateway anonymize API
│       │   └── templates.py         # Шаблоны дневников (Джинжа-like)
│       ├── tests/                   # pytest-тесты
│       ├── requirements.txt
│       └── Dockerfile
│
├── scripts/
│   └── dev-up.sh                    # Скрипт авто-подбора портов
│
├── docs/                            # 📚 Проектная документация (12 томов)
│   ├── 00_executive_summary.md
│   ├── 01_business_requirements.md
│   ├── 02_system_architecture.md
│   ├── 03_rag_design.md
│   ├── 04_anonymization.md
│   ├── 05_data_model.md
│   ├── 06_dynamic_questionnaire.md
│   ├── 07_api_contract.md
│   ├── 08_ui_ux.md
│   ├── 09_security_privacy.md
│   ├── 10_roadmap_stepbystep.md
│   └── 11_final_synthesis.md
│
└── Документы/                       # ⚠️ Реальные мед.данные — НЕ в git
```

---

## 🚀 Go-компоненты

Компоненты, написанные на **Go 1.23**, объединены в единый бинарь `gateway`:

| Пакет | Назначение | Почему Go |
|---|---|---|
| [`anonymizer/`](services/gateway/internal/anonymizer/) | 5-стадийный PII-гейт: словари ФИО/учреждений, regex-детекторы дат/адресов, FIO-детектор (морфология), NER-интеграция (Natasha) | Высокая производительность текстовых операций, `go:embed` для встраивания словарей в бинарь |
| [`auth/`](services/gateway/internal/auth/) | JWT (HS256), bcrypt-хеширование паролей, access/refresh токены | `golang.org/x/crypto` — стандартный пакет Go для криптографии |
| [`export/`](services/gateway/internal/export/) | Генерация DOCX (XML) и PDF (`go-pdf/fpdf`) с встроенными шрифтами DejaVu | Прямая работа с бинарными форматами без внешних зависимостей |
| [`handlers/`](services/gateway/internal/handlers/) | HTTP-хендлеры: `/api/v1/generate`, `/api/v1/auth/*`, `/api/v1/export/*`, `/api/v1/anonymize`, admin-эндпоинты, middleware (CORS, auth, admin) | `net/http` — стандартная библиотека, zero external deps |
| [`ragclient/`](services/gateway/internal/ragclient/) | Клиент RAG-сервиса: проксирование генерации, ingestion, admin-операции | Контролируемые таймауты, retry-логика |
| [`store/`](services/gateway/internal/store/) | PostgreSQL-хранилище через `pgx/v5`: пользователи, сессии, история запросов, admin-учётки | pgx — самый быстрый pg-драйвер для Go |
| [`catalog/`](services/gateway/internal/catalog/) | Каталог опросников (динамические вопросы по document_type) | |
| [`config/`](services/gateway/internal/config/) | Конфигурация из ENV-переменных | |

### Зависимости (Go)

```
go 1.23
github.com/go-pdf/fpdf v0.9.0     # PDF-генерация
github.com/jackc/pgx/v5 v5.7.1     # PostgreSQL driver
golang.org/x/crypto v0.27.0        # bcrypt
```

### Запуск тестов Go

```bash
cd services/gateway && go test ./... -v
```

---

## 🔒 Безопасность и приватность

| Аспект | Реализация | Документация |
|---|---|---|
| **Анонимизация PII** | 5-стадийный пайплайн: стоп-слова → FIO → учреждения → regex → NER; fail-closed (при сомнении — блокировать) | [`docs/04_anonymization.md`](docs/04_anonymization.md) |
| **Изоляция данных** | Корпус монтирован `:ro`; ПДн НИКОГДА не попадают в PostgreSQL, Qdrant, логи, git | [`docs/05_data_model.md`](docs/05_data_model.md) |
| **Аутентификация** | JWT HS256, access (15 мин) + refresh (30 дней), bcrypt-хеши паролей | [`docs/09_security_privacy.md`](docs/09_security_privacy.md) |
| **Авторизация** | Middleware: обычный врач / администратор (role-based) | [`docs/09_security_privacy.md`](docs/09_security_privacy.md) |
| **Секреты** | Хранятся в `.env` (НЕ в коде/репозитории); `.env` исключён из git | [`.gitignore`](.gitignore) |
| **CORS** | Конфигурируемый origin (по умолчанию — localhost frontend) | [`.env.example`](.env.example) |
| **LLM-выход** | В облако (X5 CoPilot) уходит **только обезличённый текст** | [`docs/04_anonymization.md`](docs/04_anonymization.md) §1 |

---

## 🧪 Тестирование

### Go (gateway)

```bash
cd services/gateway && go test ./... -v -count=1

# Покрытие:
# - anonymizer:  fio, gate, stage-3.1, stage-3.2
# - auth:        JWT, password hashing
# - handlers:    auth, anonymize, export, generate, history, admin
# - export:      format, DOCX/PDF
# - catalog:     questionnaire catalog
# - ragclient:   RAG client mocking
```

### Python (RAG-сервис)

```bash
cd services/rag && python -m pytest tests/ -v

# Покрытие:
# - test_chunking              — чанкинг обезличённого текста
# - test_pipeline              — end-to-end генерация (с моками LLM)
# - test_llm_client            — фолбэк large→medium→small, обработка 401/403
# - test_retrieval_fallback    — поведение при пустой Qdrant
# - test_ingest_endpoint       — ingestion API
# - test_diary_selection       — выбор дневников из корпуса
```

### Frontend (React)

```bash
cd frontend && npm test
# Vitest — unit-тесты для lib/questionnaire.ts и др.
```

---

## Roadmap

| Этап | Название | Статус |
|---|---|---|
| 0 | Каркас и инфраструктура (Docker Compose, health-эндпоинты) | ✅ Завершён |
| 1 | Анонимизация — PII-гейт (Go, 5 стадий) | ✅ Завершён |
| 2 | Корпус и RAG (ingestion, чанкинг, эмбеддинги, Qdrant) | ✅ Завершён |
| 3 | LLM-генерация дневников с фолбэк `large→medium→small` | ✅ Завершён |
| 4 | Динамический опросник и страница дневников (frontend) | ✅ Завершён |
| 5 | Аутентификация (JWT) и история запросов | ✅ Завершён |
| 6 | Экспорт Word/PDF | ✅ Завершён |
| 7 | Полировка UI в стиле Cursor, тёмная тема | ✅ Завершён |
| 8 | Админ-панель (управление врачами, статистика) | ✅ Завершён |
| 9 | Расширение каталога документов (первички, эпикризы, выписки) | 🔲 Планируется |
| 10 | Интеграция MCP (Model Context Protocol) для внешних агентов | 🔲 Планируется |
| 11 | Деплой на сервер больницы (prod-конфигурация) | 🔲 Планируется |

Подробный roadmap — [`docs/10_roadmap_stepbystep.md`](docs/10_roadmap_stepbystep.md).

---

## 📚 Документация

Полная проектная документация (12 томов) — в [`docs/`](docs/README.md):

- [`00_executive_summary`](docs/00_executive_summary.md) — резюме проекта
- [`01_business_requirements`](docs/01_business_requirements.md) — бизнес-требования
- [`02_system_architecture`](docs/02_system_architecture.md) — архитектура (C4, Mermaid)
- [`03_rag_design`](docs/03_rag_design.md) — дизайн RAG-пайплайна
- [`04_anonymization`](docs/04_anonymization.md) — анонимизация (5 стадий)
- [`05_data_model`](docs/05_data_model.md) — модель данных
- [`06_dynamic_questionnaire`](docs/06_dynamic_questionnaire.md) — динамический опросник
- [`07_api_contract`](docs/07_api_contract.md) — контракт API
- [`08_ui_ux`](docs/08_ui_ux.md) — UI/UX дизайн
- [`09_security_privacy`](docs/09_security_privacy.md) — безопасность и приватность
- [`10_roadmap_stepbystep`](docs/10_roadmap_stepbystep.md) — пошаговый план
- [`11_final_synthesis`](docs/11_final_synthesis.md) — итоговый синтез (для диплома)

---

## 🤝 Контекст проекта

| | |
|---|---|
| **Тип проекта** | Курсовой проект, ИТМО, курс «Go-разработчик» |
| **Автор** | Frontend-разработчик, студент ИТМО, курс «Go-разработчик» |
| **Заказчик** | Детский врач-психиатр |
| **Проблема** | Рутинная документация отнимает 30–60% рабочего времени врача |
| **Решение** | RAG + LLM + динамический опросник → 2–3 мин на дневник |
| **Философия** | Полностью open-source, без лицензионных затрат, приватность по design |

---

## 📄 Лицензия

Проект распространяется под лицензией [MIT](LICENSE).

```
MIT License

Copyright (c) 2025–2026 PsyNote contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

---

<p align="center">
  <sub>🧠 PsyNote — создано с ❤️ для врачей-психиатров · Дипломный проект ИТМО, 2025–2026</sub>
</p>
