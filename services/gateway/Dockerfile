# syntax=docker/dockerfile:1

# ─── Stage 1: build ─────────────────────────────────────────────────────────
# Статически слинкованный бинарь — «золотой стандарт» для привратника ПДн
# (см. docs/02_system_architecture.md §4.2, docs/09 §6: минимальный образ).
FROM golang:1.23-alpine AS build

WORKDIR /src

# Кэшируем зависимости отдельно от исходников.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO выключен → полностью статический бинарь, переносимый в scratch/distroless.
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway

# ─── Stage 2: runtime ───────────────────────────────────────────────────────
# Минимальный runtime. alpine выбран ради shell/healthcheck-утилит на каркасе;
# позже можно ужать до distroless/scratch (см. docs/09 §6).
FROM alpine:3.20 AS runtime

RUN apk add --no-cache wget ca-certificates \
    && addgroup -S app && adduser -S -G app app

WORKDIR /app
COPY --from=build /out/gateway /app/gateway

# Корпоративный CA-bundle монтируется через volume/env (LLM_CA_BUNDLE),
# см. docs/03 §9.2. На каркасе директория создаётся пустой.
RUN mkdir -p /app/certs && chown -R app:app /app

USER app
EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=5 \
    CMD wget -qO- http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["/app/gateway"]
