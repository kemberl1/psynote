#!/usr/bin/env bash
# ============================================================================
# AI MED — авто-подбор свободных ХОСТ-портов перед `docker compose up`.
#
# Зачем: на машине разработчика «популярные» порты (5432/8000/8080/5173 и т.п.)
# часто уже заняты другими стеками → docker падает с
#   "Bind for 0.0.0.0:PORT failed: port is already allocated".
# Сам Docker Compose не умеет авто-переключать ФИКСИРОВАННЫЙ host-порт. Но
# docker-compose.yml читает порты из переменных ${*_HOST_PORT:-default}, поэтому
# этот скрипт просто находит первый свободный порт (начиная с предпочтительного)
# и экспортирует переменные перед запуском compose.
#
# Использование:
#   ./scripts/dev-up.sh                # = docker compose up --build (foreground)
#   ./scripts/dev-up.sh -d             # в фоне
#   ./scripts/dev-up.sh --no-build     # без пересборки
#   любые доп. аргументы пробрасываются в `docker compose up`.
#
# ВНУТРЕННИЕ порты контейнеров и DSN НЕ меняются — переключаются только
# публикуемые на хост порты (host:container).
# ============================================================================
set -euo pipefail

# Перейти в корень проекта (родитель каталога scripts/), чтобы compose находил
# docker-compose.yml независимо от того, откуда запущен скрипт.
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# --- проверка занятости TCP-порта на хосте -----------------------------------
# 0 (true)  → порт свободен
# 1 (false) → порт занят
port_is_free() {
  local port="$1"
  if command -v lsof >/dev/null 2>&1; then
    ! lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
  elif command -v nc >/dev/null 2>&1; then
    # nc -z вернёт 0, если кто-то слушает порт → значит он занят
    ! nc -z 127.0.0.1 "$port" >/dev/null 2>&1
  else
    echo "WARN: ни lsof, ни nc не найдены — пропускаю проверку порта $port" >&2
    return 0
  fi
}

# Найти первый свободный порт начиная с $1 (с шагом +1, максимум 200 попыток).
find_free_port() {
  local start="$1" port="$1" tries=0
  while ! port_is_free "$port"; do
    port=$((port + 1))
    tries=$((tries + 1))
    if [ "$tries" -ge 200 ]; then
      echo "ERROR: не нашёл свободный порт рядом с $start" >&2
      exit 1
    fi
  done
  echo "$port"
}

# Назначить переменную окружения первым свободным портом от дефолта.
# Уважает уже заданное пользователем значение (если переменная задана в окружении
# или в .env через `set -a`), но если оно занято — всё равно подбирает свободный.
assign_port() {
  local var_name="$1" default="$2"
  local preferred="${!var_name:-$default}"
  local chosen
  chosen="$(find_free_port "$preferred")"
  export "$var_name=$chosen"
  if [ "$chosen" != "$preferred" ]; then
    printf '  %-22s %s → %s (порт %s занят)\n' "$var_name" "$preferred" "$chosen" "$preferred"
  else
    printf '  %-22s %s\n' "$var_name" "$chosen"
  fi
}

# Подхватить значения из .env, если он есть (как предпочтительные дефолты).
if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi

echo "Подбор свободных host-портов:"
assign_port POSTGRES_HOST_PORT   5433
assign_port QDRANT_HOST_PORT     6333
assign_port QDRANT_GRPC_HOST_PORT 6334
assign_port RAG_HOST_PORT        8001
assign_port GATEWAY_HOST_PORT    8081
assign_port FRONTEND_HOST_PORT   5174

# CORS-origin в gateway должен указывать на ХОСТ-порт фронтенда (браузер ходит
# на него снаружи docker-сети).
export CORS_ALLOWED_ORIGIN="http://localhost:${FRONTEND_HOST_PORT}"

echo
echo "Итоговые адреса:"
echo "  frontend : http://localhost:${FRONTEND_HOST_PORT}"
echo "  gateway  : http://localhost:${GATEWAY_HOST_PORT}/health"
echo "  rag      : http://localhost:${RAG_HOST_PORT}/health"
echo "  qdrant   : http://localhost:${QDRANT_HOST_PORT}/"
echo "  postgres : localhost:${POSTGRES_HOST_PORT} (внутри сети — postgres:5432)"
echo

# По умолчанию --build, если пользователь явно не передал флаги управления сборкой.
args=("$@")
if [ "${#args[@]}" -eq 0 ]; then
  args=(--build)
fi

exec docker compose up "${args[@]}"
