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
  /** Пользовательское название сессии в истории. */
  session_title?: string;
}

export function pendingTitle(documentType: string): string {
  return `${documentTypeLabel(documentType)} · Формируется…`;
}

export function batchAutoTitle(dateFrom: string, dateTo: string, dayCount: number): string {
  return `Период · ${formatDiaryDate(dateFrom)}–${formatDiaryDate(dateTo)} (${dayCount} дн.)`;
}

export function batchSessionTitle(
  dateFrom: string,
  dateTo: string,
  dayCount: number,
  custom?: string,
): string {
  const name = custom?.trim();
  return name || batchAutoTitle(dateFrom, dateTo, dayCount);
}

export function withPendingSuffix(title: string): string {
  return `${stripTitleStatus(title)} · Формируется…`;
}

export function batchPendingTitle(dateFrom: string, dateTo: string, dayCount: number): string {
  return withPendingSuffix(batchAutoTitle(dateFrom, dateTo, dayCount));
}

export function batchDoneTitle(dateFrom: string, dateTo: string, dayCount: number): string {
  return batchAutoTitle(dateFrom, dateTo, dayCount);
}

export function batchDayTitle(
  dayNumber: number,
  isoDate: string,
  documentType: string,
): string {
  return `День ${dayNumber} · ${formatDiaryDate(isoDate)} · ${documentTypeLabel(documentType)}`;
}

/** Старые записи хранили «Пакет» — в UI показываем «Период». */
export function displayHistoryTitle(title: string): string {
  return title.replaceAll("Пакет", "Период");
}

export function stripTitleStatus(title: string): string {
  return title
    .replace(/\s·\sФормируется…$/u, "")
    .replace(/\s·\sошибок:\s\d+$/u, "")
    .replace(/\s·\sОшибка$/u, "")
    .trim();
}

export function isAutoBatchTitle(title: string): boolean {
  const t = stripTitleStatus(displayHistoryTitle(title));
  return /^Период · \d{2}\.\d{2}\.\d{4}–\d{2}\.\d{2}\.\d{4} \(\d+ дн\.\)$/.test(t);
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
      session_title: typeof m.session_title === "string" ? m.session_title : "",
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
  sessionTitle?: string;
}
