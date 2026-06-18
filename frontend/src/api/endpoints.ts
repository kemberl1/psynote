// Типизированные обёртки над конкретными эндпоинтами gateway (docs/07).
// Каждая функция возвращает уже развёрнутые data (и, где нужно, meta-поля).
import { request } from "./client";
import type {
  DoctorProfile,
  DocumentType,
  GenerateRequest,
  GenerateResult,
  HistoryDetail,
  HistoryItem,
  HistoryListResult,
  LoginRequest,
  QuestionnaireSchema,
  RegisterRequest,
  RegisterResult,
  TokenPair
} from "./types";

// ─── Аутентификация (docs/07 §2, docs/09) ──────────────────────────────────

/** POST /auth/register — публичный (без токена). 409 при занятом email. */
export function register(body: RegisterRequest): Promise<RegisterResult> {
  return request<RegisterResult>("/auth/register", {
    method: "POST",
    body,
    skipAuth: true,
  });
}

/** POST /auth/login — публичный. Возвращает пару токенов. */
export function login(body: LoginRequest): Promise<TokenPair> {
  return request<TokenPair>("/auth/login", {
    method: "POST",
    body,
    skipAuth: true,
  });
}

/** POST /auth/logout — отзыв refresh-сессии на бэке (→ 204). */
export function logout(refreshToken: string): Promise<void> {
  return request<void>("/auth/logout", {
    method: "POST",
    body: { refresh_token: refreshToken },
    skipAuth: true,
  });
}

/** GET /auth/me — профиль текущего врача (по access-токену). */
export function fetchMe(signal?: AbortSignal): Promise<DoctorProfile> {
  return request<DoctorProfile>("/auth/me", { signal });
}

/** GET /document-types (docs/07 §3). */
export function fetchDocumentTypes(signal?: AbortSignal): Promise<DocumentType[]> {
  return request<DocumentType[]>("/document-types", { signal });
}

/** GET /questionnaire?document_type=... (docs/07 §3). */
export function fetchQuestionnaire(
  documentType: string,
  signal?: AbortSignal,
): Promise<QuestionnaireSchema> {
  return request<QuestionnaireSchema>("/questionnaire", {
    query: { document_type: documentType },
    signal,
  });
}

/** POST /generate (docs/07 §5). Нестриминговый вариант (стриминг — Этап 7). */
export function generate(
  body: GenerateRequest,
  signal?: AbortSignal,
): Promise<GenerateResult> {
  return request<GenerateResult>("/generate", {
    method: "POST",
    body,
    signal,
  });
}

/** GET /requests?limit=&offset= (docs/07 §6). Возвращает items + total из meta. */
export async function fetchHistory(
  params: { limit?: number; offset?: number } = {},
  signal?: AbortSignal,
): Promise<HistoryListResult> {
  let total = 0;
  const items = await request<HistoryItem[]>(
    "/requests",
    {
      query: { limit: params.limit, offset: params.offset },
      signal,
    },
    (env) => {
      total = env.meta?.total ?? 0;
    },
  );
  const list = items ?? [];
  return { items: list, total: total || list.length };
}

/** GET /requests/{id} (docs/07 §6). */
export function fetchRequestDetail(
  id: string,
  signal?: AbortSignal,
): Promise<HistoryDetail> {
  return request<HistoryDetail>(`/requests/${encodeURIComponent(id)}`, {
    signal,
  });
}
