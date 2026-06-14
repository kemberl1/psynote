// Типизированные обёртки над конкретными эндпоинтами gateway (docs/07).
// Каждая функция возвращает уже развёрнутые data (и, где нужно, meta-поля).
import { request } from "./client";
import type {
    DocumentType,
    GenerateRequest,
    GenerateResult,
    HistoryDetail,
    HistoryItem,
    HistoryListResult,
    QuestionnaireSchema,
} from "./types";

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
