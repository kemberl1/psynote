// Хуки TanStack Query поверх endpoints (docs/08 §3 — кэш истории/типов/схемы,
// единые статусы загрузки/ошибок). Ключи кэша централизованы здесь.
import {
    useMutation,
    useQuery,
    useQueryClient,
    type QueryKey,
    type UseQueryResult,
} from "@tanstack/react-query";
import {
    createNewsPost,
    createPending,
    deleteNewsPost,
    deleteRequest,
    fetchAdminFeedback,
    fetchAdminNews,
    fetchAdminSupportSummary,
    fetchAdminSupportThread,
    fetchAdminSupportThreads,
    fetchDocumentTypes,
    fetchHistory,
    fetchNews,
    fetchNewsPost,
    fetchQuestionnaire,
    fetchRequestDetail,
    fetchRequestFeedback,
    fetchSupportAttachment,
    fetchSupportThread,
    type SupportAttachmentScope,
    generate,
    markAdminSupportRead,
    markSupportRead,
    patchNewsPost,
    patchRequest,
    replyAdminSupport,
    sendSupportMessage,
    upsertRequestFeedback,
} from "./endpoints";
import type {
    AdminFeedbackListResult,
    AdminSupportThreadDetail,
    DocumentType,
    FeedbackGetResult,
    FeedbackUpsertBody,
    GenerateRequest,
    GenerateResult,
    GenerationFeedback,
    HistoryDetail,
    HistoryListResult,
    NewsListResult,
    NewsPost,
    NewsWriteBody,
    PatchRequestBody,
    PendingRequest,
    PendingResult,
    QuestionnaireSchema,
    SupportDraft,
    SupportMessage,
    SupportSummary,
    SupportThreadListResult,
    SupportThreadView,
} from "./types";

/** Централизованные query-ключи (предсказуемая инвалидация). */
export const queryKeys = {
  documentTypes: ["document-types"] as const,
  questionnaire: (docType: string) => ["questionnaire", docType] as const,
  history: (limit: number, offset: number) =>
    ["requests", { limit, offset }] as const,
  requestDetail: (id: string) => ["requests", id] as const,
  supportThread: ["support", "thread"] as const,
  requestFeedback: (id: string) => ["feedback", id] as const,
  adminSupportSummary: ["admin", "support", "summary"] as const,
  adminSupportThreads: ["admin", "support", "threads"] as const,
  adminSupportThread: (id: string) => ["admin", "support", "thread", id] as const,
  adminFeedback: ["admin", "feedback"] as const,
  news: (type = "all") => ["news", { type }] as const,
  newsPost: (id: string) => ["news", id] as const,
  adminNews: ["admin", "news"] as const,
};

function invalidateHistory(qc: ReturnType<typeof useQueryClient>) {
  void qc.invalidateQueries({ queryKey: ["requests"] });
}

/** Справочник типов документов. Меняется редко — длинный staleTime. */
export function useDocumentTypes(): UseQueryResult<DocumentType[]> {
  return useQuery({
    queryKey: queryKeys.documentTypes,
    queryFn: ({ signal }) => fetchDocumentTypes(signal),
    staleTime: 1000 * 60 * 30,
  });
}

/** Схема опросника для типа документа. Грузится при выборе типа. */
export function useQuestionnaire(
  documentType: string | undefined,
): UseQueryResult<QuestionnaireSchema> {
  return useQuery({
    queryKey: queryKeys.questionnaire(documentType ?? ""),
    queryFn: ({ signal }) => fetchQuestionnaire(documentType!, signal),
    enabled: Boolean(documentType),
    staleTime: 1000 * 60 * 30,
  });
}

/** Список истории запросов (сайдбар). */
export function useHistory(
  limit = 50,
  offset = 0,
): UseQueryResult<HistoryListResult> {
  return useQuery({
    queryKey: queryKeys.history(limit, offset),
    queryFn: ({ signal }) => fetchHistory({ limit, offset }, signal),
    // Пока есть pending — чаще обновляем, чтобы видеть «Формируется…» → done.
    refetchInterval: (q) => {
      const items = q.state.data?.items ?? [];
      return items.some((it) => it.status === "pending" || it.status === "generating")
        ? 2500
        : false;
    },
  });
}

/** Детали одной записи истории (просмотр результата). */
export function useRequestDetail(
  id: string | undefined,
): UseQueryResult<HistoryDetail> {
  return useQuery({
    queryKey: queryKeys.requestDetail(id ?? ""),
    queryFn: ({ signal }) => fetchRequestDetail(id!, signal),
    enabled: Boolean(id),
    refetchInterval: (q) => {
      const d = q.state.data;
      if (!d) return false;
      if (d.status === "pending" || d.status === "generating") return 2000;
      if (d.children?.some((c) => c.status === "pending" || c.status === "generating")) {
        return 2000;
      }
      return false;
    },
  });
}

/**
 * Мутация генерации дневника. После успеха инвалидирует историю, чтобы новый
 * запрос сразу появился в сайдбаре (docs/08 §4.1).
 */
export function useGenerate() {
  const qc = useQueryClient();
  return useMutation<GenerateResult, unknown, GenerateRequest>({
    mutationFn: (body) => generate(body),
    onSuccess: (data) => {
      invalidateHistory(qc);
      void qc.invalidateQueries({
        queryKey: queryKeys.requestDetail(data.request_id),
      });
    },
  });
}

export function useCreatePending() {
  const qc = useQueryClient();
  return useMutation<PendingResult, unknown, PendingRequest>({
    mutationFn: (body) => createPending(body),
    onSuccess: () => invalidateHistory(qc),
  });
}

function applyTitlePatch(old: unknown, id: string, title: string): unknown {
  if (!old || typeof old !== "object") return old;
  if ("items" in old && Array.isArray((old as HistoryListResult).items)) {
    const list = old as HistoryListResult;
    return {
      ...list,
      items: list.items.map((it) =>
        it.request_id === id ? { ...it, title_safe: title } : it,
      ),
    };
  }
  if ("request_id" in old && (old as HistoryDetail).request_id === id) {
    return { ...(old as HistoryDetail), title_safe: title };
  }
  return old;
}

type PatchRequestContext = {
  snapshots: Array<readonly [QueryKey, unknown]>;
};

export function usePatchRequest() {
  const qc = useQueryClient();
  return useMutation<
    { request_id: string; title_safe: string; status: string },
    unknown,
    { id: string; body: PatchRequestBody },
    PatchRequestContext
  >({
    mutationFn: ({ id, body }) => patchRequest(id, body),
    onMutate: async ({ id, body }) => {
      if (!body.title_safe) return { snapshots: [] };
      await qc.cancelQueries({ queryKey: ["requests"] });
      const snapshots = qc.getQueriesData({ queryKey: ["requests"] });
      qc.setQueriesData({ queryKey: ["requests"] }, (old) =>
        applyTitlePatch(old, id, body.title_safe!),
      );
      return { snapshots };
    },
    onError: (_err, _vars, ctx) => {
      ctx?.snapshots.forEach(([key, data]) => {
        qc.setQueryData(key, data);
      });
    },
    onSettled: (_data, _err, vars) => {
      invalidateHistory(qc);
      void qc.invalidateQueries({
        queryKey: queryKeys.requestDetail(vars.id),
      });
    },
  });
}

export function useDeleteRequest() {
  const qc = useQueryClient();
  return useMutation<void, unknown, string>({
    mutationFn: (id) => deleteRequest(id),
    onSuccess: (_data, id) => {
      invalidateHistory(qc);
      qc.removeQueries({ queryKey: queryKeys.requestDetail(id) });
    },
  });
}

export function useSupportThread(
  enabled = true,
  refetchInterval: number | false = 8000,
): UseQueryResult<SupportThreadView> {
  return useQuery({
    queryKey: queryKeys.supportThread,
    queryFn: ({ signal }) => fetchSupportThread(signal),
    enabled,
    refetchInterval,
  });
}

export function useSendSupportMessage() {
  const qc = useQueryClient();
  return useMutation<SupportMessage, unknown, SupportDraft>({
    mutationFn: (draft) => sendSupportMessage(draft),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.supportThread });
    },
  });
}

/** Содержимое вложения. Файлы неизменяемы — кэшируем навсегда. */
export function useSupportAttachment(
  id: string,
  scope: SupportAttachmentScope,
  enabled = true,
): UseQueryResult<Blob> {
  return useQuery({
    queryKey: ["support", "attachment", scope, id] as const,
    queryFn: ({ signal }) => fetchSupportAttachment(id, scope, signal),
    enabled,
    staleTime: Infinity,
    gcTime: 10 * 60_000,
    retry: 1,
  });
}

export function useMarkSupportRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => markSupportRead(),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.supportThread });
    },
  });
}

export function useRequestFeedback(
  requestId: string | undefined,
): UseQueryResult<FeedbackGetResult> {
  return useQuery({
    queryKey: queryKeys.requestFeedback(requestId ?? ""),
    queryFn: ({ signal }) => fetchRequestFeedback(requestId!, signal),
    enabled: Boolean(requestId),
  });
}

export function useUpsertFeedback(requestId: string) {
  const qc = useQueryClient();
  return useMutation<GenerationFeedback, unknown, FeedbackUpsertBody>({
    mutationFn: (body) => upsertRequestFeedback(requestId, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.requestFeedback(requestId) });
      void qc.invalidateQueries({ queryKey: queryKeys.adminFeedback });
    },
  });
}

export function useAdminSupportSummary(enabled = true): UseQueryResult<SupportSummary> {
  return useQuery({
    queryKey: queryKeys.adminSupportSummary,
    queryFn: ({ signal }) => fetchAdminSupportSummary(signal),
    enabled,
    refetchInterval: 10000,
  });
}

export function useAdminSupportThreads(enabled = true): UseQueryResult<SupportThreadListResult> {
  return useQuery({
    queryKey: queryKeys.adminSupportThreads,
    queryFn: ({ signal }) => fetchAdminSupportThreads({ limit: 80 }, signal),
    enabled,
    refetchInterval: 6000,
  });
}

export function useAdminSupportThread(
  id: string | undefined,
): UseQueryResult<AdminSupportThreadDetail> {
  return useQuery({
    queryKey: queryKeys.adminSupportThread(id ?? ""),
    queryFn: ({ signal }) => fetchAdminSupportThread(id!, signal),
    enabled: Boolean(id),
    refetchInterval: 4000,
  });
}

export function useReplyAdminSupport(threadId: string) {
  const qc = useQueryClient();
  return useMutation<SupportMessage, unknown, SupportDraft>({
    mutationFn: (draft) => replyAdminSupport(threadId, draft),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.adminSupportThread(threadId) });
      void qc.invalidateQueries({ queryKey: queryKeys.adminSupportThreads });
      void qc.invalidateQueries({ queryKey: queryKeys.adminSupportSummary });
    },
  });
}

export function useMarkAdminSupportRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (threadId: string) => markAdminSupportRead(threadId),
    onSuccess: (_data, threadId) => {
      void qc.invalidateQueries({ queryKey: queryKeys.adminSupportThread(threadId) });
      void qc.invalidateQueries({ queryKey: queryKeys.adminSupportThreads });
      void qc.invalidateQueries({ queryKey: queryKeys.adminSupportSummary });
    },
  });
}

export function useAdminFeedback(enabled = true): UseQueryResult<AdminFeedbackListResult> {
  return useQuery({
    queryKey: queryKeys.adminFeedback,
    queryFn: ({ signal }) => fetchAdminFeedback({ limit: 80 }, signal),
    enabled,
  });
}

function invalidateNews(qc: ReturnType<typeof useQueryClient>) {
  void qc.invalidateQueries({ queryKey: ["news"] });
  void qc.invalidateQueries({ queryKey: queryKeys.adminNews });
}

export function useNews(
  type = "all",
  enabled = true,
): UseQueryResult<NewsListResult> {
  return useQuery({
    queryKey: queryKeys.news(type),
    queryFn: ({ signal }) =>
      fetchNews({ limit: 50, type: type === "all" ? undefined : type }, signal),
    enabled,
  });
}

export function useNewsPost(id: string | undefined): UseQueryResult<NewsPost> {
  return useQuery({
    queryKey: queryKeys.newsPost(id ?? ""),
    queryFn: ({ signal }) => fetchNewsPost(id!, signal),
    enabled: Boolean(id),
  });
}

export function useAdminNews(enabled = true): UseQueryResult<NewsListResult> {
  return useQuery({
    queryKey: queryKeys.adminNews,
    queryFn: ({ signal }) => fetchAdminNews({ limit: 80 }, signal),
    enabled,
  });
}

export function useCreateNewsPost() {
  const qc = useQueryClient();
  return useMutation<NewsPost, unknown, NewsWriteBody>({
    mutationFn: (body) => createNewsPost(body),
    onSuccess: () => invalidateNews(qc),
  });
}

export function usePatchNewsPost() {
  const qc = useQueryClient();
  return useMutation<NewsPost, unknown, { id: string; body: NewsWriteBody }>({
    mutationFn: ({ id, body }) => patchNewsPost(id, body),
    onSuccess: (data) => {
      invalidateNews(qc);
      void qc.invalidateQueries({ queryKey: queryKeys.newsPost(data.id) });
    },
  });
}

export function useDeleteNewsPost() {
  const qc = useQueryClient();
  return useMutation<{ ok: boolean }, unknown, string>({
    mutationFn: (id) => deleteNewsPost(id),
    onSuccess: (_data, id) => {
      invalidateNews(qc);
      qc.removeQueries({ queryKey: queryKeys.newsPost(id) });
    },
  });
}
