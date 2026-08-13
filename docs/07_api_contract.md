# 07. Контракт API (API Contract — черновик)

> REST/JSON-контракт между фронтендом (React) и Go-Gateway. Формат ответов — единый конверт `{ "meta": {...}, "data": {...} }` (паттерн из `hh_analyser`). Все защищённые эндпоинты требуют `Authorization: Bearer <access_token>`.

---

## 1. Общие соглашения

- **Базовый префикс:** `/api/v1`
- **Формат:** JSON, UTF-8.
- **Конверт ответа:**
  ```jsonc
  { "meta": { "request_id": "...", "ts": "ISO8601" }, "data": { /* ... */ } }
  ```
- **Ошибки:**
  ```jsonc
  { "meta": { "request_id": "..." }, "error": { "code": "PII_DETECTED", "message": "..." } }
  ```
- **Коды:** `200` OK, `201` Created, `400` валидация, `401` не аутентифицирован, `403` нет доступа, `404` не найдено, `409` конфликт, `422` **PII-гейт заблокировал**, `429` лимит, `5xx` сервер/LLM.
- **Приватность:** тела запросов могут содержать ПДн только на входе `/generate` и `/attachments` — они немедленно проходят гейт; в хранилище и ответах ПДн уже нет.

---

## 2. Аутентификация

### POST /api/v1/auth/register
Регистрация врача.
```jsonc
// Request
{ "email": "doctor@example.com", "password": "••••••••", "display_name": "Врач Т." }
// Response 201
{ "meta": {...}, "data": { "doctor_id": "uuid", "email": "doctor@example.com" } }
```

### POST /api/v1/auth/login
```jsonc
// Request
{ "email": "doctor@example.com", "password": "••••••••" }
// Response 200
{ "meta": {...}, "data": {
  "access_token": "jwt...",      // короткоживущий
  "refresh_token": "opaque...",  // в httpOnly cookie (рекоменд.)
  "expires_in": 900
}}
```

### POST /api/v1/auth/refresh
Обновление access-токена по refresh-токену. → новый `access_token`.

### POST /api/v1/auth/logout
Отзыв refresh-токена (revoked=true). → `204`.

### GET /api/v1/auth/me
Текущий врач. → `{ doctor_id, email, display_name, role }`.

---

## 3. Справочники и схема опросника

### GET /api/v1/document-types
Список доступных типов документов.
```jsonc
// Response 200
{ "meta": {...}, "data": [
  { "code": "daily",    "title": "Ежедневный дневник",      "is_active": true },
  { "code": "exam_10d", "title": "Осмотр (раз в 10 дней)",  "is_active": true }
]}
```

### GET /api/v1/questionnaire?document_type=daily
Актуальная JSON-схема опросника для типа (см. [`06_dynamic_questionnaire.md`](06_dynamic_questionnaire.md)).
```jsonc
// Response 200
{ "meta": { "version": 3 }, "data": {
  "document_type": "daily",
  "questions": [ { "id": "dynamics", "label": "Динамика состояния", "type": "select", "required": true, "allow_custom": true, "default": "no_change", "options": [ /* ... */ ], "conditional": [ /* ... */ ] } /* ... */ ]
}}
```

---

## 4. Приложенные документы

### POST /api/v1/attachments
Загрузка документа для обогащения контекста (multipart/form-data). **Сразу анонимизируется**; если новый — дозагружается в Qdrant.
```jsonc
// Response 201
{ "meta": {...}, "data": {
  "attachment_id": "uuid",
  "original_filename_safe": "consult_psychologist.docx",
  "anonymizer_removed_count": 7,
  "ingested_to_vector_db": true,
  "vector_collection": "corpus_diaries"
}}
// Response 422 (гейт нашёл ПДн, которые не удалось обезличить надёжно)
{ "meta": {...}, "error": { "code": "PII_DETECTED", "message": "Документ не удалось безопасно обезличить" } }
```

---

## 5. Генерация

### POST /api/v1/generate
Главный эндпоинт. Принимает ответы опросника (+ ссылки на приложения). **Анонимизация-гейт → RAG → LLM (с fallback) → пост-валидация.**
```jsonc
// Request
{
  "document_type": "daily",
  "answers": {
    "dynamics": "no_change",
    "mood": "lowered",
    "mood_detail": ["anxiety", "tearfulness"],
    "sleep": "hard_to_fall_asleep",
    "appetite": "decreased"
    // custom-значения приходят как { "value": "__custom__", "custom_text": "..." }
  },
  "attachment_ids": ["uuid"],
  "options": { "stream": true }
}
// Response 200 (нестриминговый вариант)
{ "meta": { "llm_model_used": "deepseek-v4-flash", "tokens_used": 812, "request_id": "uuid" },
  "data": {
    "request_id": "uuid",
    "content": "обезличенный текст дневника с плейсхолдерами [ДАТА], [ФИО_ВРАЧА]...",
    "status": "done",
    // Сводка обезличивания СВОБОДНОГО ВВОДА врача (для UX-плашки «мы убрали X ПДн»).
    // Обратно-совместимое ДОБАВЛЕНИЕ. ТОЛЬКО счётчики/категории — НИКОГДА значения.
    // removed_by_type: человекочитаемые категории (ФИО/ДАТА/АДРЕС/...).
    "anonymization": { "removed_count": 3, "removed_by_type": { "ФИО": 2, "ДАТА": 1 } }
  }}
// Response 422 — PII-гейт
{ "meta": {...}, "error": { "code": "PII_DETECTED", "message": "Во входных данных обнаружены ПДн" } }
// Response 503 — все модели fallback недоступны
{ "meta": {...}, "error": { "code": "LLM_UNAVAILABLE", "message": "Сервис генерации временно недоступен" } }
```

**Стриминг (рекомендуется для UX):** `options.stream=true` → ответ как `text/event-stream` (SSE) с инкрементальными токенами и финальным событием `done` с метаданными (модель, токены).

### Пакетная генерация (UI `/diary/batch`)

Отдельного batch-эндпоинта **нет**. Фронтенд оркестрирует последовательные вызовы `POST /generate`:

- для каждого календарного дня в выбранном периоде (макс. 31);
- тип документа на день: `daily`, кроме дней **10, 20, 30…** от **даты поступления** → `exam_10d`;
- сжатый опросник маппится в полный набор `answers` на клиенте (`frontend/src/lib/batchDiary.ts`);
- каждый успешный ответ сохраняется в истории как отдельный `request_id`.

---

## 6. История запросов

### GET /api/v1/requests?limit=20&offset=0
Список запросов текущего врача (обезличенный). Сортировка по `created_at` desc.
```jsonc
// Response 200
{ "meta": { "total": 134 }, "data": [
  { "request_id": "uuid", "document_type": "daily", "title_safe": "Ежедневный дневник · сниженное настроение · без динамики", "llm_model_used": "deepseek-v4-flash", "status": "done", "created_at": "..." }
]}
```

### GET /api/v1/requests/{id}
Детали запроса (обезличенные ответы + сгенерированный документ).
```jsonc
{ "meta": {...}, "data": {
  "request_id": "uuid",
  "document_type": "daily",
  "answers_anonymized": { /* ... */ },
  "content": "обезличенный текст...",
  "title_safe": "Ежедневный дневник · ...",
  "llm_model_used": "deepseek-v4-flash",
  "status": "done",
  "anonymizer_removed_count": 5,   // аудит, без значений ПДн (docs/05 §2.2)
  "created_at": "..."
}}
```

### DELETE /api/v1/requests/{id}
Удаление записи истории врача. → `204`.

---

## 7. Экспорт

### POST /api/v1/requests/{id}/export
Экспорт сгенерированного документа.
```jsonc
// Request
{ "format": "docx",   // docx | txt
  "substitutions": {  // подстановка реальных значений ЛОКАЛЬНО на этапе экспорта
    "[ДАТА]": "19.09.2025",
    "[ФИО_ВРАЧА]": "..."
  }}
// Response 200 — бинарный файл (Content-Disposition: attachment)
```
> Подстановки приходят с клиента и применяются Go-сервисом экспорта **в памяти**, результат отдаётся файлом и **не сохраняется** с реальными ПДн.

Форматирование DOCX соответствует корпусному сборнику (эталонный DOCX в `Документы/02_корпус/сборник_дневников_ИБ/`, только локально): Calibri 11pt, поля 2,5 см, выравнивание по ширине, жирные секции.

### POST /api/v1/export/batch
Пакетный экспорт нескольких записей истории **в один файл** (сценарий `/diary/batch`).
```jsonc
// Request
{ "format": "docx",   // docx | txt
  "request_ids": ["uuid-1", "uuid-2", "..."],  // макс. 31, сортируются по дате
  "substitutions": { "[ДАТА]": "19.09.2025", "[ФИО_ВРАЧА]": "..." } }
// Response 200 — бинарный файл (Content-Disposition: diaries_batch_<from>_to_<to>.<ext>)
```

---

## 8. Служебные

### GET /api/v1/health
Состояние сервиса (gateway, доступность Qdrant, доступность LLM по `GET /models`).
```jsonc
{ "meta": {...}, "data": { "status": "ok", "llm": { "enabled": true, "models_available": ["deepseek-v4-flash","deepseek-v4-pro","deepseek-v4-flash"] }, "vector_db": "ok" } }
```

### POST /api/v1/admin/documents
Загрузка документа в корпус (admin). См. существующий admin UI.

---

## 8.1. Чат поддержки

Один диалог на врача. Сообщения из виджета (`sender_role=user`) и ответы из админки (`sender_role=support`).

### GET /api/v1/support/thread
Свой диалог. Если ещё не писали — `{ status: "none", unread: 0, messages: [] }`.

### POST /api/v1/support/messages
```jsonc
{ "body": "Не генерируется осмотр за 10 дней" }
```
Создаёт диалог при первом сообщении. → 201 + сообщение.

### POST /api/v1/support/thread/read
Сбросить непрочитанные у врача.

### GET /api/v1/admin/support/summary
`{ unread_messages, unread_threads }` — только admin.

### GET /api/v1/admin/support/threads
Список диалогов (новые/непрочитанные сверху).

### GET /api/v1/admin/support/threads/{id}
Диалог + сообщения.

### POST /api/v1/admin/support/threads/{id}/messages
Ответ поддержки. `{ "body": "…" }`

### POST /api/v1/admin/support/threads/{id}/read
Сбросить непрочитанные у админа.

---

## 8.2. Отзывы на генерации

Один отзыв на пару (дневник, врач): звёзды 1–5, комментарий, цитата.

### GET /api/v1/requests/{id}/feedback
Свой отзыв: `{ "feedback": {…} | null }`. 404, если дневник чужой.

### PUT /api/v1/requests/{id}/feedback
```jsonc
{ "rating": 4, "comment": "тонковато в динамике", "quote": "состояние стабильное…" }
```

### GET /api/v1/admin/feedback
Все отзывы с автором и заголовком дневника. Только admin.

---

## 9. Сводная таблица эндпоинтов

| Метод | Путь | Назначение | Auth |
|---|---|---|---|
| POST | `/auth/register` | регистрация | — |
| POST | `/auth/login` | вход | — |
| POST | `/auth/refresh` | обновить токен | refresh |
| POST | `/auth/logout` | выход | ✅ |
| GET | `/auth/me` | текущий врач | ✅ |
| GET | `/document-types` | типы документов | ✅ |
| GET | `/questionnaire` | схема опросника | ✅ |
| POST | `/attachments` | загрузить документ (анонимизация+ingest) | ✅ |
| POST | `/generate` | сгенерировать дневник | ✅ |
| GET | `/requests` | история | ✅ |
| GET | `/requests/{id}` | детали | ✅ |
| DELETE | `/requests/{id}` | удалить | ✅ |
| POST | `/requests/{id}/export` | экспорт docx/txt | ✅ |
| POST | `/export/batch` | пакетный экспорт в один файл | ✅ |
| GET | `/health` | статус | — |
| GET | `/support/thread` | свой чат поддержки | ✅ |
| POST | `/support/messages` | написать в поддержку | ✅ |
| POST | `/support/thread/read` | прочитано (врач) | ✅ |
| GET | `/requests/{id}/feedback` | свой отзыв на дневник | ✅ |
| PUT | `/requests/{id}/feedback` | сохранить отзыв | ✅ |
| GET | `/admin/documents` | корпус | ✅ admin |
| POST | `/admin/documents` | загрузка в корпус | ✅ admin |
| GET | `/admin/support/summary` | непрочитанные чаты | ✅ admin |
| GET | `/admin/support/threads` | инбокс поддержки | ✅ admin |
| GET | `/admin/support/threads/{id}` | диалог | ✅ admin |
| POST | `/admin/support/threads/{id}/messages` | ответить | ✅ admin |
| GET | `/admin/feedback` | все отзывы | ✅ admin |

---

Дальше: UI/UX — [`08_ui_ux.md`](08_ui_ux.md).
