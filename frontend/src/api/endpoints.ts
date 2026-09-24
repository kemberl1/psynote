// Типизированные обёртки над конкретными эндпоинтами gateway (docs/07).
// Этап 10: добавлены admin-эндпоинты загрузки документов.
import { blobRequest, request, uploadRequest } from "./client";
import type {
  AdminDocument,
  AdminDocumentListResult,
  AdminFeedbackItem,
  AdminFeedbackListResult,
  AdminSupportThreadDetail,
  AdminUploadResult,
  FeedbackGetResult,
  FeedbackUpsertBody,
  GenerationFeedback,
  DoctorProfile,
  DocumentType,
  GenerateRequest,
  GenerateResult,
  HistoryDetail,
  HistoryItem,
  HistoryListResult,
  LoginRequest,
  PatchMeRequest,
  PatchRequestBody,
  PendingRequest,
  PendingResult,
  QuestionnaireSchema,
  RegisterRequest,
  RegisterResult,
  SupportDraft,
  SupportMessage,
  SupportSummary,
  NewsListResult,
  NewsPost,
  NewsWriteBody,
  SupportThreadListItem,
  SupportThreadListResult,
  SupportThreadView,
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

export function patchMe(
  body: PatchMeRequest,
  signal?: AbortSignal,
): Promise<DoctorProfile> {
  return request<DoctorProfile>("/auth/me", { method: "PATCH", body, signal });
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

// ─── Чат поддержки ─────────────────────────────────────────────────────────

export function fetchSupportThread(signal?: AbortSignal): Promise<SupportThreadView> {
  return request<SupportThreadView>("/support/thread", { signal });
}

/** Текст — JSON, с файлами — multipart (body + files[]). */
function postSupportDraft(
  path: string, draft: SupportDraft, signal?: AbortSignal,
): Promise<SupportMessage> {
  if (draft.files.length === 0) {
    return request<SupportMessage>(path, {
      method: "POST", body: { body: draft.body }, signal,
    });
  }
  const form = new FormData();
  form.append("body", draft.body);
  for (const f of draft.files) form.append("files", f, f.name);
  return uploadRequest<SupportMessage>(path, form, signal);
}

export function sendSupportMessage(
  draft: SupportDraft, signal?: AbortSignal,
): Promise<SupportMessage> {
  return postSupportDraft("/support/messages", draft, signal);
}

export type SupportAttachmentScope = "user" | "admin";

export function fetchSupportAttachment(
  id: string, scope: SupportAttachmentScope, signal?: AbortSignal,
): Promise<Blob> {
  const prefix = scope === "admin" ? "/admin/support" : "/support";
  return blobRequest(`${prefix}/attachments/${encodeURIComponent(id)}`, signal);
}

export function markSupportRead(signal?: AbortSignal): Promise<{ ok: boolean }> {
  return request<{ ok: boolean }>("/support/thread/read", {
    method: "POST", body: {}, signal,
  });
}

export function fetchAdminSupportSummary(signal?: AbortSignal): Promise<SupportSummary> {
  return request<SupportSummary>("/admin/support/summary", { signal });
}

export async function fetchAdminSupportThreads(
  params: { limit?: number; offset?: number } = {},
  signal?: AbortSignal,
): Promise<SupportThreadListResult> {
  let total = 0;
  const items = await request<SupportThreadListItem[]>(
    "/admin/support/threads",
    { query: { limit: params.limit, offset: params.offset }, signal },
    (env) => { total = env.meta?.total ?? 0; },
  );
  const list = items ?? [];
  return { items: list, total: total || list.length };
}

export function fetchAdminSupportThread(
  id: string, signal?: AbortSignal,
): Promise<AdminSupportThreadDetail> {
  return request<AdminSupportThreadDetail>(
    `/admin/support/threads/${encodeURIComponent(id)}`, { signal },
  );
}

export function replyAdminSupport(
  threadId: string, draft: SupportDraft, signal?: AbortSignal,
): Promise<SupportMessage> {
  return postSupportDraft(
    `/admin/support/threads/${encodeURIComponent(threadId)}/messages`,
    draft, signal,
  );
}

export function markAdminSupportRead(
  threadId: string, signal?: AbortSignal,
): Promise<{ ok: boolean }> {
  return request<{ ok: boolean }>(
    `/admin/support/threads/${encodeURIComponent(threadId)}/read`,
    { method: "POST", body: {}, signal },
  );
}

// ─── Отзывы на генерации ───────────────────────────────────────────────────

export function fetchRequestFeedback(
  requestId: string, signal?: AbortSignal,
): Promise<FeedbackGetResult> {
  return request<FeedbackGetResult>(
    `/requests/${encodeURIComponent(requestId)}/feedback`, { signal },
  );
}

export function upsertRequestFeedback(
  requestId: string, body: FeedbackUpsertBody, signal?: AbortSignal,
): Promise<GenerationFeedback> {
  return request<GenerationFeedback>(
    `/requests/${encodeURIComponent(requestId)}/feedback`,
    { method: "PUT", body, signal },
  );
}

export async function fetchAdminFeedback(
  params: { limit?: number; offset?: number } = {},
  signal?: AbortSignal,
): Promise<AdminFeedbackListResult> {
  let total = 0;
  const items = await request<AdminFeedbackItem[]>(
    "/admin/feedback",
    { query: { limit: params.limit, offset: params.offset }, signal },
    (env) => { total = env.meta?.total ?? 0; },
  );
  const list = items ?? [];
  return { items: list, total: total || list.length };
}

// ─── Новости и релизы ──────────────────────────────────────────────────────

export async function fetchNews(
  params: { limit?: number; offset?: number; type?: string } = {},
  signal?: AbortSignal,
): Promise<NewsListResult> {
  let total = 0;
  const items = await request<NewsPost[]>(
    "/news",
    { query: { limit: params.limit, offset: params.offset, type: params.type }, signal },
    (env) => { total = env.meta?.total ?? 0; },
  );
  const list = items ?? [];
  return { items: list, total: total || list.length };
}

export function fetchNewsPost(
  id: string, signal?: AbortSignal,
): Promise<NewsPost> {
  return request<NewsPost>(`/news/${encodeURIComponent(id)}`, { signal });
}

export async function fetchAdminNews(
  params: { limit?: number; offset?: number; type?: string } = {},
  signal?: AbortSignal,
): Promise<NewsListResult> {
  let total = 0;
  const items = await request<NewsPost[]>(
    "/admin/news",
    { query: { limit: params.limit, offset: params.offset, type: params.type }, signal },
    (env) => { total = env.meta?.total ?? 0; },
  );
  const list = items ?? [];
  return { items: list, total: total || list.length };
}

export function createNewsPost(
  body: NewsWriteBody, signal?: AbortSignal,
): Promise<NewsPost> {
  return request<NewsPost>("/admin/news", { method: "POST", body, signal });
}

export function patchNewsPost(
  id: string, body: NewsWriteBody, signal?: AbortSignal,
): Promise<NewsPost> {
  return request<NewsPost>(`/admin/news/${encodeURIComponent(id)}`, {
    method: "PATCH", body, signal,
  });
}

export function deleteNewsPost(
  id: string, signal?: AbortSignal,
): Promise<{ ok: boolean }> {
  return request<{ ok: boolean }>(`/admin/news/${encodeURIComponent(id)}`, {
    method: "DELETE", signal,
  });
}
