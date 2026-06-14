// Тонкий типобезопасный слой над fetch для API gateway (docs/07).
// Отвечает за: базовый URL, разбор конверта {meta, data|error}, нормализацию
// ошибок в ApiError (код контракта + HTTP-статус). Без хранения секретов.
import { ApiError } from "./errors";
import type { ApiErrorCode, Envelope } from "./types";

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
  ];
  if (code && (known as string[]).includes(code)) return code as ApiErrorCode;
  if (status === 404) return "NOT_FOUND";
  if (status === 422) return "PII_DETECTED";
  if (status === 503) return "LLM_UNAVAILABLE";
  if (status === 400) return "BAD_REQUEST";
  return "UNKNOWN";
}

/**
 * Выполняет запрос и возвращает РАЗВЁРНУТЫЙ конверт.
 * onEnvelope получает meta, чтобы вытащить total/version/llm-метаданные.
 */
export async function request<TData>(
  path: string,
  opts: RequestOptions = {},
  onEnvelope?: (env: Envelope<TData>) => void,
): Promise<TData> {
  const { method = "GET", body, query, signal } = opts;

  let res: Response;
  try {
    res = await fetch(buildUrl(path, query), {
      method,
      signal,
      headers: body
        ? { "Content-Type": "application/json; charset=utf-8" }
        : undefined,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch (e) {
    // Сетевой сбой / abort / CORS — нормализуем в ApiError.
    if (e instanceof DOMException && e.name === "AbortError") throw e;
    throw new ApiError("NETWORK", "сетевая ошибка", 0);
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
