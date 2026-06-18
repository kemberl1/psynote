// Тонкий типобезопасный слой над fetch для API gateway (docs/07).
// Отвечает за: базовый URL, разбор конверта {meta, data|error}, нормализацию
// ошибок в ApiError (код контракта + HTTP-статус), а также АУТЕНТИФИКАЦИЮ
// (Этап 9): подстановку Authorization: Bearer <access>, авто-refresh при 401
// и единичный повтор запроса. Без хранения секретов в коде.
import { ApiError } from "./errors";
import {
  clearTokens,
  getAccessToken,
  getRefreshToken,
  notifySessionEnded,
  setTokens,
} from "./session";
import type { ApiErrorCode, Envelope, TokenPair } from "./types";

/**
 * Базовый префикс API. По умолчанию "/api/v1" — фронт ходит на относительный
 * путь, который vite-proxy (dev) или nginx (prod) проксирует на gateway
 * (docs/07 §1). Переопределяется через VITE_API_BASE.
 */
export const API_BASE = (import.meta.env.VITE_API_BASE ?? "/api/v1").replace(/\/$/, "");

/** Допустимые методы. */
type Method = "GET" | "POST" | "DELETE";

interface RequestOptions {
  method?: Method;
  /** JSON-тело (будет сериализовано). */
  body?: unknown;
  /** Query-параметры (undefined-значения отбрасываются). */
  query?: Record<string, string | number | undefined>;
  signal?: AbortSignal;
  /** Внутреннее: пропустить аутентификацию (для самих auth-эндпоинтов). */
  skipAuth?: boolean;
  /** Внутреннее: не пытаться refresh при 401 (предотвращает рекурсию). */
  _isRetry?: boolean;
}

/** Собирает URL с query-строкой. */
function buildUrl(path: string, query?: RequestOptions["query"]): string {
  const url = `${API_BASE}${path.startsWith("/") ? path : `/${path}`}`;
  if (!query) return url;
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== null && value !== "") {
      params.set(key, String(value));
    }
  }
  const qs = params.toString();
  return qs ? `${url}?${qs}` : url;
}

/** Сужает произвольную строку к известному коду ошибки контракта. */
function normalizeCode(code: string | undefined, status: number): ApiErrorCode {
  const known: ApiErrorCode[] = [
    "BAD_REQUEST",
    "INVALID_DOCUMENT_TYPE",
    "PII_DETECTED",
    "LLM_UNAVAILABLE",
    "NOT_FOUND",
    "SERVICE_UNAVAILABLE",
    "INTERNAL",
    "UNAUTHORIZED",
    "EMAIL_TAKEN",
  ];
  if (code && (known as string[]).includes(code)) return code as ApiErrorCode;
  if (status === 401) return "UNAUTHORIZED";
  if (status === 409) return "EMAIL_TAKEN";
  if (status === 404) return "NOT_FOUND";
  if (status === 422) return "PII_DETECTED";
  if (status === 503) return "LLM_UNAVAILABLE";
  if (status === 400) return "BAD_REQUEST";
  return "UNKNOWN";
}

// ─── Авто-refresh (Этап 9, docs/09 §1.3) ────────────────────────────────────
// Один общий промис refresh на все параллельные 401, чтобы не отправлять
// несколько /auth/refresh одновременно (ротация инвалидировала бы друг друга).
let refreshInFlight: Promise<boolean> | null = null;

/** Пытается обновить access по refresh-токену. true при успехе. */
async function tryRefresh(): Promise<boolean> {
  const refresh = getRefreshToken();
  if (!refresh) return false;

  if (!refreshInFlight) {
    refreshInFlight = (async () => {
      try {
        const res = await fetch(buildUrl("/auth/refresh"), {
          method: "POST",
          headers: { "Content-Type": "application/json; charset=utf-8" },
          body: JSON.stringify({ refresh_token: refresh }),
        });
        if (!res.ok) return false;
        const env = (await res.json()) as Envelope<TokenPair>;
        if (!env.data?.access_token || !env.data?.refresh_token) return false;
        setTokens(env.data.access_token, env.data.refresh_token);
        return true;
      } catch {
        return false;
      } finally {
        // Сбрасываем после завершения, чтобы следующий 401 мог запустить новый.
        const done = refreshInFlight;
        void done;
        refreshInFlight = null;
      }
    })();
  }
  return refreshInFlight;
}

/**
 * Выполняет запрос и возвращает РАЗВЁРНУТЫЙ конверт.
 * onEnvelope получает meta, чтобы вытащить total/version/llm-метаданные.
 *
 * При 401 на защищённом запросе: один раз пробуем refresh и повторяем запрос;
 * если refresh не удался — чистим сессию и уведомляем (AuthContext → /login).
 */
export async function request<TData>(
  path: string,
  opts: RequestOptions = {},
  onEnvelope?: (env: Envelope<TData>) => void,
): Promise<TData> {
  const { method = "GET", body, query, signal, skipAuth, _isRetry } = opts;

  const headers: Record<string, string> = {};
  if (body !== undefined) {
    headers["Content-Type"] = "application/json; charset=utf-8";
  }
  if (!skipAuth) {
    const access = getAccessToken();
    if (access) headers["Authorization"] = `Bearer ${access}`;
  }

  let res: Response;
  try {
    res = await fetch(buildUrl(path, query), {
      method,
      signal,
      headers: Object.keys(headers).length ? headers : undefined,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch (e) {
    // Сетевой сбой / abort / CORS — нормализуем в ApiError.
    if (e instanceof DOMException && e.name === "AbortError") throw e;
    throw new ApiError("NETWORK", "сетевая ошибка", 0);
  }

  // 401 на защищённом запросе → попытка refresh + единичный повтор.
  if (res.status === 401 && !skipAuth && !_isRetry) {
    const refreshed = await tryRefresh();
    if (refreshed) {
      return request<TData>(path, { ...opts, _isRetry: true }, onEnvelope);
    }
    // refresh не помог — сессия мертва.
    clearTokens();
    notifySessionEnded();
    throw new ApiError("UNAUTHORIZED", "сессия истекла", 401);
  }

  // 204 No Content — тела нет (DELETE/logout).
  if (res.status === 204) {
    return undefined as TData;
  }

  let env: Envelope<TData> | null = null;
  try {
    env = (await res.json()) as Envelope<TData>;
  } catch {
    // Не-JSON ответ от прокси/инфраструктуры.
    if (!res.ok) {
      throw new ApiError(
        normalizeCode(undefined, res.status),
        `HTTP ${res.status}`,
        res.status,
      );
    }
    throw new ApiError("UNKNOWN", "не удалось разобрать ответ", res.status);
  }

  if (!res.ok || env.error) {
    const code = normalizeCode(env.error?.code, res.status);
    const message = env.error?.message ?? `HTTP ${res.status}`;
    throw new ApiError(code, message, res.status, env.meta?.request_id);
  }

  onEnvelope?.(env);
  return env.data as TData;
}
