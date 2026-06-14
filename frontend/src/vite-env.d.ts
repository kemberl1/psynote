/// <reference types="vite/client" />

// Типы env-переменных фронтенда (docs/07 §1).
// VITE_API_BASE — базовый префикс API. По умолчанию "/api/v1" (через vite-proxy
// или nginx). Можно переопределить для прямого обращения к gateway.
interface ImportMetaEnv {
  readonly VITE_API_BASE?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
