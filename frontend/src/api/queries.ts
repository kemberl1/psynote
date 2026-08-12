// Хуки TanStack Query поверх endpoints (docs/08 §3 — кэш истории/типов/схемы,
// единые статусы загрузки/ошибок). Ключи кэша централизованы здесь.
import {
    useMutation,
    useQuery,
    useQueryClient,
    type UseQueryResult,
} from "@tanstack/react-query";
import {
    createPending,
    deleteRequest,
    fetchDocumentTypes,
    fetchHistory,
    fetchQuestionnaire,
    fetchRequestDetail,
    generate,
    patchRequest,
} from "./endpoints";
import type {
    DocumentType,
    GenerateRequest,
    GenerateResult,
    HistoryDetail,
    HistoryListResult,
    PatchRequestBody,
    PendingRequest,
    PendingResult,
    QuestionnaireSchema,
} from "./types";

/** Централизованные query-ключи (предсказуемая инвалидация). */
export const queryKeys = {
  documentTypes: ["document-types"] as const,
  questionnaire: (docType: string) => ["questionnaire", docType] as const,
  history: (limit: number, offset: number) =>
    ["requests", { limit, offset }] as const,
  requestDetail: (id: string) => ["requests", id] as const,
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

export function usePatchRequest() {
  const qc = useQueryClient();
  return useMutation<
    { request_id: string; title_safe: string; status: string },
    unknown,
    { id: string; body: PatchRequestBody }
  >({
    mutationFn: ({ id, body }) => patchRequest(id, body),
    onSuccess: (data) => {
      invalidateHistory(qc);
      void qc.invalidateQueries({
        queryKey: queryKeys.requestDetail(data.request_id),
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
