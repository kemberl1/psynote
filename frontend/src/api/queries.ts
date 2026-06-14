// Хуки TanStack Query поверх endpoints (docs/08 §3 — кэш истории/типов/схемы,
// единые статусы загрузки/ошибок). Ключи кэша централизованы здесь.
import {
    useMutation,
    useQuery,
    useQueryClient,
    type UseQueryResult,
} from "@tanstack/react-query";
import {
    fetchDocumentTypes,
    fetchHistory,
    fetchQuestionnaire,
    fetchRequestDetail,
    generate,
} from "./endpoints";
import type {
    DocumentType,
    GenerateRequest,
    GenerateResult,
    HistoryDetail,
    HistoryListResult,
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
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["requests"] });
    },
  });
}
