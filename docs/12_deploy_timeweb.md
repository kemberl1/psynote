# Деплой PsyNote: Timeweb Cloud + Coolify (оплата в рублях)

Пошаговая инструкция для новичка. Цель: сайт для врачей с HTTPS и автообновлением при пуше в `main`.

Репозиторий: `https://github.com/kemberl1/psynote.git`  
Прод-файл: [`docker-compose.prod.yml`](../docker-compose.prod.yml)

---

## Что получится

```text
Врач → https://ваш-url  → Coolify (HTTPS)
                         → frontend (nginx) → /api → gateway → rag → DeepSeek
                                            → postgres / qdrant (закрыты снаружи)
```

Пуш в `main` на GitHub → Coolify сам пересобирает и выкатывает.

---

## Перед стартом (чеклист)

- [ ] Карта РФ для оплаты Timeweb
- [ ] Аккаунт GitHub с доступом к `kemberl1/psynote`
- [ ] Ключ DeepSeek (`LLM_API_KEY`) и баланс > 0
- [ ] Локальная папка `Документы/` (~66 MB) — корпус для RAG (в git её нет)

Ориентир по деньгам: VPS 8 GB ≈ **1–2 тыс ₽/мес** + DeepSeek по факту (обычно копейки).

---

## Шаг A — купить VPS на Timeweb (~15 мин)

1. Открой [https://timeweb.cloud](https://timeweb.cloud) → регистрация / вход.
2. Создай **Cloud Servers** / **VPS**:
   - **ОС:** Ubuntu 24.04
   - **RAM:** минимум **8 GB** (лучше 16 GB, если бюджет позволяет — RAG тяжёлый)
   - **Диск:** 40–80 GB SSD
   - **Регион:** Россия
3. Дождись создания сервера.
4. Запиши в блокнот:
   - **IP-адрес**
   - **пароль root** (или скачай SSH-ключ)

> Если Timeweb предлагает «панель» / «консоль в браузере» — этого достаточно, SSH с ноутбука не обязателен на старте.

---

## Шаг B — установить Coolify (~20 мин)

1. Открой **веб-консоль** сервера в Timeweb (или SSH: `ssh root@IP`).
2. Вставь и выполни:

```bash
curl -fsSL https://cdn.coollabs.io/coolify/install.sh | bash
```

3. Дождись окончания (5–15 минут). Скрипт поставит Docker и Coolify.
4. В браузере открой: `http://ВАШ_IP:8000`
5. Создай **администратора Coolify** (email + пароль) — это НЕ логин врачей, а панель сервера.
6. В файрволе Timeweb открой порты: **80**, **443**, **8000** (если ещё не открыты).

---

## Шаг C — подключить GitHub (~10 мин)

1. В Coolify: **Sources** → подключи GitHub (OAuth).
2. Разреши доступ к репозиторию `kemberl1/psynote` (или ко всему аккаунту).
3. **New Resource** → **Docker Compose** (не «Dockerfile» и не «Static»).
4. Выбери репозиторий `psynote`, ветка **`main`**.
5. Compose file: **`docker-compose.prod.yml`**
6. Base directory: `/` (корень репо).
7. Включи **Auto Deploy** / Deploy on push для ветки `main`.
8. Пока **не жми Deploy** — сначала секреты и корпус (шаги D–E).

В настройках ресурса укажи, что публичный сервис — **`frontend`**, порт контейнера **`80`**.

---

## Шаг D — секреты (Environment)

В Coolify → твой ресурс → **Environment Variables** добавь:

| Переменная | Пример / как получить |
|---|---|
| `POSTGRES_USER` | `aimed` |
| `POSTGRES_PASSWORD` | **только hex** (см. ниже) — без `/+=` |
| `POSTGRES_DB` | `aimed` |
| `JWT_SECRET` | длинная случайная строка |
| `LLM_API_KEY` | ключ с [platform.deepseek.com](https://platform.deepseek.com) |
| `LLM_BASE_URL` | `https://api.deepseek.com` |
| `LLM_MODEL_LARGE` | `deepseek-v4-flash` |
| `LLM_MODEL_MEDIUM` | `deepseek-v4-pro` |
| `LLM_THINKING` | `disabled` |
| `CORS_ALLOWED_ORIGIN` | пока `http://placeholder.local` — **заменишь** на реальный URL (без `/` в конце) |

**Не коммить** эти значения в git. Только в Coolify.

Сгенерировать пароли (на своём Mac):

```bash
openssl rand -hex 24    # POSTGRES_PASSWORD (безопасно для DSN)
openssl rand -hex 32    # JWT_SECRET
```

> Смена `POSTGRES_PASSWORD` **после** первого деплоя не меняет пароль в уже созданном volume.
> Нужно либо вернуть старый пароль, либо удалить postgres volume и задеплоить заново (см. «Типовые проблемы»).

---

## Шаг E — загрузить корпус `Документы/` (~15 мин)

Без корпуса RAG почти бесполезен.

### На своём компьютере

```bash
cd "/путь/к/AI_MED"
tar -czf corpus.tar.gz Документы
```

### На сервер

Через SCP (подставь IP):

```bash
scp corpus.tar.gz root@ВАШ_IP:/root/
```

Или загрузи файл через SFTP-клиент (Cyberduck / FileZilla).

### На сервере (консоль)

Нужно положить файлы туда, откуда Coolify монтирует `./data/corpus`.  
Обычно рабочая директория приложения Coolify выглядит как  
`/data/coolify/applications/<id>/` — точный путь смотри в UI Coolify → Storage / General.

Практичный способ:

1. Найди на сервере каталог приложения Coolify (в UI есть путь / или `find /data/coolify -name docker-compose.prod.yml`).
2. Создай `data/corpus` рядом с compose:

```bash
# пример — подставь реальный путь из Coolify
cd /data/coolify/applications/XXXX
mkdir -p data/corpus
tar -xzf /root/corpus.tar.gz -C /tmp
# содержимое Документы/ → data/corpus/
cp -a /tmp/Документы/. data/corpus/
ls data/corpus | head
```

В compose переменная `CORPUS_HOST_PATH` по умолчанию `./data/corpus`.

После копирования с Mac папки часто приходят с правами `drwx------` — контейнер `rag` (не root) их не читает. Обязательно:

```bash
chmod -R a+rX /data/coolify/applications/<uuid>/data/corpus
# проверка: должно быть тысячи файлов, не 15
docker exec "$(docker ps --format '{{.Names}}' | grep rag | head -1)" find /data/corpus -type f | wc -l
```

Ingest на Coolify (compose-файла в applications/ может не быть):

```bash
docker exec -it "$(docker ps --format '{{.Names}}' | grep rag | head -1)" python -m app.ingest ingest
```

---

## Шаг F — первый деплой

1. В Coolify нажми **Deploy**.
2. Смотри логи сборки:
   - `rag` качает torch/e5 — **долго** (10–30+ мин в первый раз), это нормально.
   - `frontend` делает `npm run build`.
3. Когда статус зелёный — Coolify покажет **HTTPS URL** (временный поддомен или IP).
4. Сразу обнови секрет:

```text
CORS_ALLOWED_ORIGIN=https://твой-реальный-url-из-coolify
```

(без слэша в конце) и сделай **Redeploy**.

---

## Шаг G — прогнать ingestion (один раз)

Когда контейнеры уже запущены, в терминале сервера (или Coolify → Terminal у сервиса `rag`):

```bash
# из каталога приложения с docker-compose.prod.yml
docker compose -f docker-compose.prod.yml run --rm rag python -m app.ingest ingest
```

Дождись окончания. Повторно гонять нужно только если обновила корпус.

Проверка Qdrant/health:

```bash
docker compose -f docker-compose.prod.yml exec rag curl -fsS http://127.0.0.1:8000/health
```

---

## Шаг H — проверка для врачей

1. Открой HTTPS URL в браузере.
2. Войди seed-админом (смени пароль сразу после входа!):
   - email: `admin@aimed.local`
   - пароль: `admin123456`
3. Создай дневник / сгенерируй документ.
4. Убедись, что ответ приходит (модель DeepSeek в метаданных).
5. Выдай врачам **только HTTPS-ссылку** + их логины.  
   Не давай доступ к порту 8000 Coolify и не открывай postgres/qdrant наружу.

---

## Автообновление из `main`

Уже включено на шаге C. Дальше рабочий цикл:

1. Правишь код локально.
2. `git push origin main`
3. Coolify сам деплоит.
4. Корпус и volumes (postgres, qdrant, кэш e5) **сохраняются** между деплоями.

Секреты и `data/corpus` при пуше **не затираются**.

---

## Шаг I — свой домен (когда будет нужно)

1. Купи домен на Timeweb (или другом регистраторе).
2. DNS: запись **A** → IP твоего VPS.
3. В Coolify → Domain → укажи домен → Let's Encrypt выпустит HTTPS.
4. Обнови `CORS_ALLOWED_ORIGIN=https://твой-домен.ru` → Redeploy.

---

## Типовые проблемы

| Симптом | Что проверить |
|---|---|
| Сайт не открывается | Порты 80/443 в файрволе Timeweb; статус Deploy в Coolify |
| Логин → **404** или **503** | Это не «неверный пароль». Coolify → сервис **gateway** → **Logs** (не Deploy log). Ищи `postgres connect failed` / `login_enabled":false`. Чаще всего пароль БД не совпадает с volume (см. ниже) |
| Логин → **401** | Инфраструктура ок; неверный email/пароль или нет seed-админа |
| Логин есть, генерация падает | `LLM_API_KEY`, баланс DeepSeek, логи сервиса `rag` |
| Пустые/странные дневники | Забыт ingestion; пустой `data/corpus` |
| CORS / сеть в браузере | `CORS_ALLOWED_ORIGIN` точно равен URL из адресной строки (схема `http`/`https` тоже) |
| Не хватает памяти | VPS 8 GB минимум; смотри `dmesg` / OOM в логах |
| Сборка rag вечность | Первый раз качает модели — подожди; кэш в volume `rag_hf_cache` |

### Сброс Postgres volume (если логин 404/503 после смены пароля)

На VPS по SSH:

```bash
# найти volume проекта
docker volume ls | grep -i postgres

# остановить стек в Coolify (Stop), затем:
docker volume rm <имя_volume_postgres>

# в Coolify: POSTGRES_PASSWORD = результат `openssl rand -hex 24`
# Deploy заново — initdb + seed admin создадутся снова
```

Seed после чистого volume: `admin@aimed.local` / `admin123456`.

### Логин 401 / register INTERNAL (нет таблиц / нет админа)

Coolify раньше мог поднять Postgres **без** SQL из `deploy/initdb`. Накати схему вручную (приложение Running):

```bash
PG=$(docker ps --format '{{.Names}}' | grep postgres | head -1)
echo "postgres container: $PG"

for f in 01_schema.sql 02_seed.sql 03_migration_doctor_nullable.sql 04_auth_session_index.sql 05_admin_role.sql 06_history_batch_parent.sql; do
  echo "=== $f ==="
  curl -fsSL "https://raw.githubusercontent.com/kemberl1/psynote/main/deploy/initdb/$f" \
    | docker exec -i "$PG" psql -U aimed -d aimed
done

docker exec -i "$PG" psql -U aimed -d aimed -c "SELECT email, role FROM doctor;"
```

Потом логин: `admin@aimed.local` / `admin123456`.

---

## Безопасность (минимум)

- Смени пароль `admin@aimed.local` в первый день.
- Не коммить `.env` и `Документы/`.
- Не публикуй порты postgres/qdrant (в `docker-compose.prod.yml` их уже нет).
- Ограничь, кому известен URL Coolify-панели (`:8000`).

---

## Что дальше писать в чат ассистенту

Когда сделаешь шаг — напиши коротко:

1. «A готово, IP есть»  
2. «B готово, Coolify открылся»  
3. «C+D секреты внесены»  
4. «E корпус залит»  
5. «F задеплоилось / ошибка: …»  

Дальше разберём по логам точечно.
