# syntax=docker/dockerfile:1

# Frontend (React 19 + Vite + TS). См. docs/02_system_architecture.md §2.1.
# Этап 1 (каркас): dev-режим через vite (быстрый старт, HMR в compose).
# TODO(этап 7): при необходимости — multi-stage build + nginx для прод-раздачи.
FROM node:22-alpine AS dev

WORKDIR /app

# Кэшируем установку зависимостей отдельно от исходников.
COPY package.json ./
# package-lock.json появится после первого `npm install` локально.
RUN npm install

COPY . .

EXPOSE 5173

# --host уже задан в vite.config.ts (0.0.0.0), но дублируем для надёжности.
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0", "--port", "5173"]
