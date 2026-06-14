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
{ "meta": { "llm_model_used": "x5-airun-large", "tokens_used": 812, "request_id": "uuid" },
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

---

## 6. История запросов

### GET /api/v1/requests?limit=20&offset=0
Список запросов текущего врача (обезличенный). Сортировка по `created_at` desc.
```jsonc
// Response 200
{ "meta": { "total": 134 }, "data": [
  { "request_id": "uuid", "document_type": "daily", "title_safe": "Ежедневный дневник · сниженное настроение · без динамики", "llm_model_used": "x5-airun-large", "status": "done", "created_at": "..." }
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
  "llm_model_used": "x5-airun-large",
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
{ "format": "docx",   // docx | pdf | txt
  "substitutions": {  // подстановка реальных значений ЛОКАЛЬНО на этапе экспорта
    "[ДАТА]": "19.09.2025",
    "[ФИО_ВРАЧА]": "..."
  }}
// Response 200 — бинарный файл (Content-Disposition: attachment)
```
> Подстановки приходят с клиента и применяются Go-сервисом экспорта **в памяти**, результат отдаётся файлом и **не сохраняется** с реальными ПДн.

---

## 8. Служебные

### GET /api/v1/health
Состояние сервиса (gateway, доступность Qdrant, доступность LLM по `GET /models`).
```jsonc
{ "meta": {...}, "data": { "status": "ok", "llm": { "enabled": true, "models_available": ["x5-airun-large","x5-airun-medium"] }, "vector_db": "ok" } }
```

### (Будущее) Админка корпуса
- `POST /api/v1/admin/corpus` — загрузка документа в корпус (анонимизация + ingestion). Роль `admin`. Низкий приоритет.

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
| POST | `/requests/{id}/export` | экспорт docx/pdf/txt | ✅ |
| GET | `/health` | статус | — |
| POST | `/admin/corpus` *(future)* | загрузка корпуса | ✅ admin |

---

Дальше: UI/UX — [`08_ui_ux.md`](08_ui_ux.md).
