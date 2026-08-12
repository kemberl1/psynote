# ─── Stage 1: build ─────────────────────────────────────────────────────────
# Статически слинкованный бинарь — «золотой стандарт» для привратника ПДн
# (см. docs/02_system_architecture.md §4.2, docs/09 §6: минимальный образ).
FROM golang:1.23-alpine AS build

# git + CA нужны для go mod на чистых VPS (Coolify/Timeweb).
RUN apk add --no-cache git ca-certificates

WORKDIR /src

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOPROXY=https://proxy.golang.org,direct

# Кэшируем зависимости отдельно от исходников.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Без кавычек в ldflags — меньше сюрпризов с BuildKit/Coolify escaping.
# При ошибке печатаем verbose-вывод, чтобы в логе Coolify было видно причину.
RUN go build -trimpath -ldflags=-s -ldflags=-w -o /out/gateway ./cmd/gateway \
    || (echo "=== go build failed; verbose retry ===" && go build -v -o /out/gateway ./cmd/gateway)

# ─── Stage 2: runtime ───────────────────────────────────────────────────────
FROM alpine:3.20 AS runtime

RUN apk add --no-cache wget ca-certificates \
    && addgroup -S app && adduser -S -G app app

WORKDIR /app
COPY --from=build /out/gateway /app/gateway

RUN mkdir -p /app/certs && chown -R app:app /app

USER app
EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=5 \
    CMD wget -qO- http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["/app/gateway"]
