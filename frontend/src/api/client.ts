// Тонкий типобезопасный слой над fetch для API gateway (docs/07).
// Этап 10: добавлен uploadRequest для multipart/form-data (admin upload).
import { ApiError } from "./errors";
import {
  clearTokens,
  getAccessToken,
  getRefreshToken,
  notifySessionEnded,
  setTokens,
} from "./session";
import type { ApiErrorCode, Envelope, TokenPair } from "./types";

export const API_BASE = (import.meta.env.VITE_API_BASE ?? "/api/v1").replace(/\/$/, "");

type Method = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

interface RequestOptions {
  method?: Method;
  body?: unknown;
  query?: Record<string, string | number | undefined>;
  signal?: AbortSignal;
  skipAuth?: boolean;
  _isRetry?: boolean;
}

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
    "FORBIDDEN",
  ];
  if (code && (known as string[]).includes(code)) return code as ApiErrorCode;
  if (status === 401) return "UNAUTHORIZED";
  if (status === 403) return "FORBIDDEN";
  if (status === 409) return "EMAIL_TAKEN";
  if (status === 404) return "NOT_FOUND";
  if (status === 422) return "PII_DETECTED";
  if (status === 503) return "LLM_UNAVAILABLE";
  if (status === 400) return "BAD_REQUEST";
  return "UNKNOWN";
}

// ─── Авто-refresh (Этап 9) ──────────────────────────────────────────────────
let refreshInFlight: Promise<boolean> | null = null;

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
        const done = refreshInFlight;
        void done;
        refreshInFlight = null;
      }
    })();
  }
  return refreshInFlight;
}

/** Выполняет JSON-запрос и возвращает data из конверта. */
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
    if (e instanceof DOMException && e.name === "AbortError") throw e;
    throw new ApiError("NETWORK", "сетевая ошибка", 0);
  }

  if (res.status === 401 && !skipAuth && !_isRetry) {
    const refreshed = await tryRefresh();
    if (refreshed) {
      return request<TData>(path, { ...opts, _isRetry: true }, onEnvelope);
    }
    clearTokens();
    notifySessionEnded();
    throw new ApiError("UNAUTHORIZED", "сессия истекла", 401);
  }

  if (res.status === 204) {
    return undefined as TData;
  }

  let env: Envelope<TData> | null = null;
  try {
    env = (await res.json()) as Envelope<TData>;
  } catch {
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

// ─── Multipart upload (Этап 10) ─────────────────────────────────────────────

/**
 * Загрузка multipart/form-data на защищённый эндпоинт (admin upload).
 * НЕ сериализует body в JSON, ставит правильный Content-Type (boundary).
 * При 401 — авто-refresh + повтор (как request).
 */
export async function uploadRequest<TData>(
  path: string,
  formData: FormData,
  signal?: AbortSignal,
): Promise<TData> {
  const headers: Record<string, string> = {};
  const access = getAccessToken();
  if (access) headers["Authorization"] = `Bearer ${access}`;

  let res: Response;
  try {
    res = await fetch(buildUrl(path), {
      method: "POST",
      headers,
      body: formData,
      signal,
    });
  } catch (e) {
    if (e instanceof DOMException && e.name === "AbortError") throw e;
    throw new ApiError("NETWORK", "сетевая ошибка", 0);
  }

  if (res.status === 401) {
    const refreshed = await tryRefresh();
    if (refreshed) {
      return uploadRequest<TData>(path, formData, signal);
    }
    clearTokens();
    notifySessionEnded();
    throw new ApiError("UNAUTHORIZED", "сессия истекла", 401);
  }

  let env: Envelope<TData> | null = null;
  try {
    env = (await res.json()) as Envelope<TData>;
  } catch {
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

  return env.data as TData;
}
