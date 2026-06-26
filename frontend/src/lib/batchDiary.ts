// Логика пакетной генерации дневников за период (docs/08 §5, batch tab).
// Определяет тип документа по дню госпитализации и собирает answers для /generate.
import type { Answers } from "../api/types";

export type BatchDocType = "daily" | "exam_10d";

export const MAX_BATCH_DAYS = 31;

/** Параметры периода и сжатого опросника. */
export interface BatchParams {
  admissionDate: string;
  dateFrom: string;
  dateTo: string;
  answers: Answers;
  freeContext: string;
}

export interface BatchDayPlan {
  date: Date;
  isoDate: string;
  dayNumber: number;
  documentType: BatchDocType;
}

export interface BatchPlan {
  days: BatchDayPlan[];
  dailyCount: number;
  examCount: number;
}

/** Парсинг YYYY-MM-DD в локальную полночь (без сдвига UTC). */
export function parseLocalDate(iso: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso.trim());
  if (!m) return null;
  const y = Number(m[1]);
  const mo = Number(m[2]) - 1;
  const d = Number(m[3]);
  const dt = new Date(y, mo, d);
  if (dt.getFullYear() !== y || dt.getMonth() !== mo || dt.getDate() !== d) {
    return null;
  }
  return dt;
}

export function formatISODate(d: Date): string {
  const y = d.getFullYear();
  const mo = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${mo}-${day}`;
}

/** Номер дня госпитализации (1 = день поступления). */
export function dayNumberFromAdmission(admission: Date, day: Date): number {
  const a = Date.UTC(admission.getFullYear(), admission.getMonth(), admission.getDate());
  const b = Date.UTC(day.getFullYear(), day.getMonth(), day.getDate());
  return Math.floor((b - a) / 86_400_000) + 1;
}

/** День 10, 20, 30… от поступления → расширенный осмотр (exam_10d). */
export function isExam10Day(admission: Date, day: Date): boolean {
  const n = dayNumberFromAdmission(admission, day);
  return n > 0 && n % 10 === 0;
}

export function resolveDocType(admission: Date, day: Date): BatchDocType {
  return isExam10Day(admission, day) ? "exam_10d" : "daily";
}

export function eachDayInRange(from: Date, to: Date): Date[] {
  const days: Date[] = [];
  const cur = new Date(from.getFullYear(), from.getMonth(), from.getDate());
  const end = new Date(to.getFullYear(), to.getMonth(), to.getDate());
  while (cur <= end) {
    days.push(new Date(cur));
    cur.setDate(cur.getDate() + 1);
  }
  return days;
}

export interface BatchDateValidation {
  ok: boolean;
  message?: string;
}

/** Валидация дат периода и поступления. */
export function validateBatchDates(
  admissionIso: string,
  fromIso: string,
  toIso: string,
): BatchDateValidation {
  const admission = parseLocalDate(admissionIso);
  const from = parseLocalDate(fromIso);
  const to = parseLocalDate(toIso);
  if (!admission || !from || !to) {
    return { ok: false, message: "Укажите корректные даты в формате ДД.ММ.ГГГГ." };
  }
  if (from < admission) {
    return {
      ok: false,
      message: "Начало периода не может быть раньше даты поступления.",
    };
  }
  if (to < from) {
    return { ok: false, message: "Конец периода не может быть раньше начала." };
  }
  const count = eachDayInRange(from, to).length;
  if (count > MAX_BATCH_DAYS) {
    return {
      ok: false,
      message: `Период слишком длинный (${count} дн.). Максимум — ${MAX_BATCH_DAYS} дней за один запуск.`,
    };
  }
  if (count === 0) {
    return { ok: false, message: "Выберите хотя бы один день." };
  }
  return { ok: true };
}

export function buildBatchPlan(params: BatchParams): BatchPlan | null {
  const v = validateBatchDates(params.admissionDate, params.dateFrom, params.dateTo);
  if (!v.ok) return null;
  const admission = parseLocalDate(params.admissionDate)!;
  const from = parseLocalDate(params.dateFrom)!;
  const to = parseLocalDate(params.dateTo)!;
  const days = eachDayInRange(from, to).map((date) => {
    const dayNumber = dayNumberFromAdmission(admission, date);
    const documentType = resolveDocType(admission, date);
    return { date, isoDate: formatISODate(date), dayNumber, documentType };
  });
  const examCount = days.filter((d) => d.documentType === "exam_10d").length;
  return {
    days,
    dailyCount: days.length - examCount,
    examCount,
  };
}

function mapDynamicsToPeriod(dynamics: unknown): string {
  switch (dynamics) {
    case "positive":
    case "slight_improvement":
    case "stable_positive":
      return "improvement";
    case "worsening":
      return "no_improvement";
    default:
      return "no_change";
  }
}

/** Собрать answers для POST /generate из сжатого опросника и метаданных дня. */
export function buildGenerateAnswers(
  batchAnswers: Answers,
  dayNumber: number,
  freeContext: string,
  docType: BatchDocType,
): Answers {
  const eventsParts: string[] = [];
  const feeding = batchAnswers.feeding_weight;
  const events = batchAnswers.key_events;
  if (typeof feeding === "string" && feeding.trim()) {
    eventsParts.push(feeding.trim());
  }
  if (typeof events === "string" && events.trim()) {
    eventsParts.push(events.trim());
  }
  if (freeContext.trim()) {
    eventsParts.push(freeContext.trim());
  }
  eventsParts.push(`День госпитализации: ${dayNumber}.`);

  const base: Answers = {
    dynamics: batchAnswers.dynamics ?? "no_change",
    productive_symptoms: "not_detected",
    mood: batchAnswers.mood ?? "even",
    behavior: batchAnswers.behavior ?? "ordered",
    contact: ["productive", "polite_staff"],
    sleep: batchAnswers.sleep ?? "not_disturbed",
    appetite: batchAnswers.appetite ?? "preserved",
    tolerance: batchAnswers.tolerance ?? "good",
    complaints: batchAnswers.complaints ?? "none",
  };

  if (eventsParts.length > 0) {
    base.events = "present";
    base.events_detail = eventsParts.join(" ");
  }

  if (docType === "exam_10d") {
    const periodDynamics =
      batchAnswers.period_dynamics ?? mapDynamicsToPeriod(batchAnswers.dynamics);
    return {
      ...base,
      anamnesis_disease: "no_additions",
      physical_status: "unremarkable",
      neuro_status: "no_acute",
      criticism: "formal",
      thinking: "no_gross",
      attention_memory: "no_gross",
      intellect: "age_norm",
      suicidal: "not_detected",
      syndrome: batchAnswers.syndrome ?? "anxiety_depressive",
      comorbidities: ["none"],
      prescriptions: "see_list",
      period_dynamics: periodDynamics,
    };
  }

  return base;
}
