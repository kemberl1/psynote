// Типизированные обёртки над конкретными эндпоинтами gateway (docs/07).
// Этап 10: добавлены admin-эндпоинты загрузки документов.
import { request, uploadRequest } from "./client";
import type {
  AdminDocument,
  AdminDocumentListResult,
  AdminUploadResult,
  DoctorProfile,
  DocumentType,
  GenerateRequest,
  GenerateResult,
  HistoryDetail,
  HistoryItem,
  HistoryListResult,
  LoginRequest,
  PatchRequestBody,
  PendingRequest,
  PendingResult,
  QuestionnaireSchema,
  RegisterRequest,
  RegisterResult,
  TokenPair,
} from "./types";

// ─── Аутентификация (docs/07 §2, docs/09) ──────────────────────────────────

export function register(body: RegisterRequest): Promise<RegisterResult> {
  return request<RegisterResult>("/auth/register", {
    method: "POST", body, skipAuth: true,
  });
}

export function login(body: LoginRequest): Promise<TokenPair> {
  return request<TokenPair>("/auth/login", {
    method: "POST", body, skipAuth: true,
  });
}

export function logout(refreshToken: string): Promise<void> {
  return request<void>("/auth/logout", {
    method: "POST", body: { refresh_token: refreshToken }, skipAuth: true,
  });
}

export function fetchMe(signal?: AbortSignal): Promise<DoctorProfile> {
  return request<DoctorProfile>("/auth/me", { signal });
}

export function fetchDocumentTypes(signal?: AbortSignal): Promise<DocumentType[]> {
  return request<DocumentType[]>("/document-types", { signal });
}

export function fetchQuestionnaire(
  documentType: string, signal?: AbortSignal,
): Promise<QuestionnaireSchema> {
  return request<QuestionnaireSchema>("/questionnaire", {
    query: { document_type: documentType }, signal,
  });
}

export function generate(
  body: GenerateRequest, signal?: AbortSignal,
): Promise<GenerateResult> {
  return request<GenerateResult>("/generate", { method: "POST", body, signal });
}

export function createPending(
  body: PendingRequest, signal?: AbortSignal,
): Promise<PendingResult> {
  return request<PendingResult>("/requests/pending", {
    method: "POST", body, signal,
  });
}

export function patchRequest(
  id: string, body: PatchRequestBody, signal?: AbortSignal,
): Promise<{ request_id: string; title_safe: string; status: string }> {
  return request(`/requests/${encodeURIComponent(id)}`, {
    method: "PATCH", body, signal,
  });
}

export function deleteRequest(
  id: string, signal?: AbortSignal,
): Promise<void> {
  return request<void>(`/requests/${encodeURIComponent(id)}`, {
    method: "DELETE", signal,
  });
}

export async function fetchHistory(
  params: { limit?: number; offset?: number } = {},
  signal?: AbortSignal,
): Promise<HistoryListResult> {
  let total = 0;
  const items = await request<HistoryItem[]>(
    "/requests",
    { query: { limit: params.limit, offset: params.offset }, signal },
    (env) => { total = env.meta?.total ?? 0; },
  );
  const list = items ?? [];
  return { items: list, total: total || list.length };
}

export function fetchRequestDetail(
  id: string, signal?: AbortSignal,
): Promise<HistoryDetail> {
  return request<HistoryDetail>(`/requests/${encodeURIComponent(id)}`, { signal });
}

// ─── Админка: загрузка документов (Этап 10, docs/07 §8) ────────────────────

/** POST /admin/documents — multipart upload .docx/.odt/.doc */
export function uploadAdminDocument(
  file: File, signal?: AbortSignal,
): Promise<AdminUploadResult> {
  const formData = new FormData();
  formData.append("file", file);
  return uploadRequest<AdminUploadResult>("/admin/documents", formData, signal);
}

/** GET /admin/documents — список загруженных документов */
export async function fetchAdminDocuments(
  signal?: AbortSignal,
): Promise<AdminDocumentListResult> {
  let total = 0;
  const items = await request<AdminDocument[]>(
    "/admin/documents",
    { signal },
    (env) => { total = env.meta?.total ?? 0; },
  );
  const list = items ?? [];
  return { items: list, total: total || list.length };
}

/** GET /admin/documents/{id} — детали загруженного документа */
export function fetchAdminDocument(
  id: string, signal?: AbortSignal,
): Promise<AdminDocument> {
  return request<AdminDocument>(
    `/admin/documents/${encodeURIComponent(id)}`, { signal },
  );
}
