// Хелперы заголовков истории и метаданных пакетной генерации.
import type { Answers } from "../api/types";
import { documentTypeLabel, formatDiaryDate } from "./format";

export const BATCH_META_KEY = "__batch_meta__";

export interface BatchMeta {
  admission_date: string;
  date_from: string;
  date_to: string;
  estimated_discharge?: string;
  director_context?: string;
}

export function pendingTitle(documentType: string): string {
  return `${documentTypeLabel(documentType)} · Формируется…`;
}

export function batchPendingTitle(dateFrom: string, dateTo: string, dayCount: number): string {
  return `Пакет · ${formatDiaryDate(dateFrom)}–${formatDiaryDate(dateTo)} (${dayCount} дн.) · Формируется…`;
}

export function batchDoneTitle(dateFrom: string, dateTo: string, dayCount: number): string {
  return `Пакет · ${formatDiaryDate(dateFrom)}–${formatDiaryDate(dateTo)} (${dayCount} дн.)`;
}

export function batchDayTitle(
  dayNumber: number,
  isoDate: string,
  documentType: string,
): string {
  return `День ${dayNumber} · ${formatDiaryDate(isoDate)} · ${documentTypeLabel(documentType)}`;
}

export function packBatchAnswers(
  answers: Answers,
  meta: BatchMeta,
): Answers {
  return {
    ...answers,
    [BATCH_META_KEY]: meta as unknown as Answers[string],
  };
}

export function unpackBatchMeta(answers: Answers | undefined | null): {
  answers: Answers;
  meta: BatchMeta | null;
} {
  if (!answers) return { answers: {}, meta: null };
  const raw = answers[BATCH_META_KEY];
  const rest = { ...answers };
  delete rest[BATCH_META_KEY];
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) {
    return { answers: rest, meta: null };
  }
  const m = raw as unknown as Record<string, unknown>;
  if (typeof m.admission_date !== "string" || typeof m.date_from !== "string" || typeof m.date_to !== "string") {
    return { answers: rest, meta: null };
  }
  return {
    answers: rest,
    meta: {
      admission_date: m.admission_date,
      date_from: m.date_from,
      date_to: m.date_to,
      estimated_discharge:
        typeof m.estimated_discharge === "string" ? m.estimated_discharge : "",
      director_context:
        typeof m.director_context === "string" ? m.director_context : "",
    },
  };
}

/** Состояние навигации для «Редактировать данные». */
export interface EditDiaryState {
  requestId: string;
  documentType: string;
  answers: Answers;
  /** Для пакета. */
  batchMeta?: BatchMeta;
}
