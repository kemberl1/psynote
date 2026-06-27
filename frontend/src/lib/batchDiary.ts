// Логика пакетной генерации дневников за период (docs/08 §5, batch tab).
// Определяет тип документа по дню госпитализации и собирает answers для /generate.
// Режиссёрский контекст передаётся как __arc_context__ — специальное поле,
// которое RAG-сервис инжектирует в системный промпт, а НЕ в текст дневника.
import type { Answers } from "../api/types";

export type BatchDocType = "daily" | "exam_10d";

export const MAX_BATCH_DAYS = 31;

/** Параметры периода и нарративного опросника. */
export interface BatchParams {
  admissionDate: string;
  dateFrom: string;
  dateTo: string;
  answers: Answers;
  /** Режиссёрский контекст — установка для LLM, не вставляется в текст дневника. */
  directorContext: string;
  /** Ориентировочная дата выписки (опционально). Калибрует темп улучшения. */
  estimatedDischargeDate: string;
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

// ── Маппинг новых полей нарративного опросника → значения daily/exam_10d ──

/** overall_dynamics → dynamics (ежедневный) */
function mapDynamicsToDaily(dynamics: unknown): string {
  switch (dynamics) {
    case "positive": return "positive";
    case "stable": return "no_change";
    case "wavy": return "no_change";
    case "negative": return "worsening";
    default: return "no_change";
  }
}

/** overall_dynamics → period_dynamics (осмотр 10 дней) */
function mapDynamicsToExam(dynamics: unknown): string {
  switch (dynamics) {
    case "positive": return "improvement";
    case "stable": return "no_change";
    case "wavy": return "slight_improvement";
    case "negative": return "no_improvement";
    default: return "no_change";
  }
}

/** Количество дней между двумя ISO-датами (отрицательное = в прошлом). */
function daysBetweenIso(fromIso: string, toIso: string): number | null {
  const from = parseLocalDate(fromIso);
  const to = parseLocalDate(toIso);
  if (!from || !to) return null;
  const a = Date.UTC(from.getFullYear(), from.getMonth(), from.getDate());
  const b = Date.UTC(to.getFullYear(), to.getMonth(), to.getDate());
  return Math.floor((b - a) / 86_400_000);
}

/** Человекочитаемое описание notable_events для событийного поля. */
function describeNotableEvents(events: unknown): string {
  if (!Array.isArray(events) || events.length === 0) return "";
  const labels: Record<string, string> = {
    specialist_consult: "консультации специалистов",
    therapy_change: "изменение терапии",
    exacerbation: "эпизоды ухудшения состояния",
    ecg_eeg: "инструментальное обследование (ЭКГ/ЭЭГ)",
  };
  return events
    .filter((e): e is string => typeof e === "string")
    .map((e) => labels[e] ?? e)
    .join(", ");
}

// ── Временна́я фильтрация событий из режиссёрского контекста ──────────────

/** День недели (0=вс, 1=пн, … 6=сб) → набор русских ключевых слов. */
const DOW_KEYWORDS: Record<number, string[]> = {
  0: ["в воскресенье", "воскресенье", "в вс"],
  1: ["в понедельник", "понедельник", "в пн"],
  2: ["во вторник", "вторник", "в вт"],
  3: ["в среду", "среду", "в ср"],
  4: ["в четверг", "четверг", "в чт"],
  5: ["в пятницу", "пятницу", "в пт"],
  6: ["в субботу", "субботу", "в сб"],
};

/** Ключевые слова, указывающие на выходной день. */
const WEEKEND_KEYWORDS = ["в выходные", "на выходных", "выходные", "в выходной"];

/** Ключевые слова для привязки к первым/последним дням. */
const EARLY_KEYWORDS = [
  "первые дни", "в первые дни", "в начале", "при поступлении",
  "в первый день", "на первый день", "с первых дней",
];
const LATE_KEYWORDS = [
  "в конце", "перед выпиской", "в последние дни", "накануне выписки",
  "ближе к выписке",
];

/**
 * Разбивает режиссёрский контекст на отдельные предложения/фрагменты
 * и определяет, является ли каждый фрагмент «событийным» (содержит
 * временну́ю привязку) или «фоновым» (описывает общее состояние).
 *
 * Возвращает { eventSentences, backgroundSentences }.
 */
function splitDirectorContext(context: string): {
  eventSentences: string[];
  backgroundSentences: string[];
} {
  // Все ключевые слова-триггеры временно́й привязки (кроме общих фоновых)
  const temporalTriggers = [
    ...WEEKEND_KEYWORDS,
    ...EARLY_KEYWORDS,
    ...LATE_KEYWORDS,
    ...Object.values(DOW_KEYWORDS).flat(),
    "вчера", "сегодня утром", "в этот день", "в тот день",
    "на прогулке", "во время", "после визита", "после встречи",
    "на этой неделе", "в начале недели", "в конце недели",
    "на второй день", "на третий день", "на четвёртый день",
    "на пятый день", "на шестой день", "на седьмой день",
    "на 2-й день", "на 3-й день",
  ];

  // Разбиваем по предложениям (по `. ` и по переносу строки)
  const sentences = context
    .split(/(?<=\.)\s+|(?<=\n)/)
    .map((s) => s.trim())
    .filter(Boolean);

  const eventSentences: string[] = [];
  const backgroundSentences: string[] = [];

  for (const s of sentences) {
    const lower = s.toLowerCase();
    const hasTemporalTrigger = temporalTriggers.some((kw) =>
      lower.includes(kw.toLowerCase())
    );
    if (hasTemporalTrigger) {
      eventSentences.push(s);
    } else {
      backgroundSentences.push(s);
    }
  }

  return { eventSentences, backgroundSentences };
}

/**
 * Определяет, относится ли событийное предложение к конкретному дню.
 *
 * @param sentence — одно предложение из режиссёрского контекста
 * @param date     — дата генерируемого дня
 * @param dayNumber — номер дня госпитализации (1-based)
 * @param totalDays — всего дней в периоде
 */
function isSentenceRelevantForDay(
  sentence: string,
  date: Date,
  dayNumber: number,
  totalDays: number,
): boolean {
  const lower = sentence.toLowerCase();
  const dow = date.getDay(); // 0=вс, 1=пн, … 6=сб
  const isWeekend = dow === 0 || dow === 6;

  // Выходные → только суббота или воскресенье
  if (WEEKEND_KEYWORDS.some((kw) => lower.includes(kw))) {
    return isWeekend;
  }

  // День недели → точное совпадение
  for (const [dowStr, keywords] of Object.entries(DOW_KEYWORDS)) {
    if (keywords.some((kw) => lower.includes(kw))) {
      return dow === Number(dowStr);
    }
  }

  // «Первые дни» / «при поступлении» / «в начале» → дни 1–3
  if (EARLY_KEYWORDS.some((kw) => lower.includes(kw))) {
    return dayNumber <= 3;
  }

  // «Перед выпиской» / «в конце» → последние 2 дня периода
  if (LATE_KEYWORDS.some((kw) => lower.includes(kw))) {
    return dayNumber >= totalDays - 1;
  }

  // «На второй/третий день» → конкретный номер дня
  const dayMatch = lower.match(/на\s+(\d+)[- ]?[йо]?\s*день/);
  if (dayMatch) {
    return dayNumber === Number(dayMatch[1]);
  }

  // «Вчера» → день 2+ (вчера = предыдущий день; нет смысла в день 1)
  if (lower.includes("вчера")) {
    return dayNumber >= 2;
  }

  // Если ни одно правило не сработало, включаем в текущий день
  // (безопасный фолбэк: лучше включить, чем потерять)
  return true;
}

/**
 * Фильтрует режиссёрский контекст для конкретного дня:
 * — все фоновые предложения включаются всегда;
 * — событийные предложения — только если относятся к этому дню.
 *
 * Это устраняет дублирование уникальных событий по всем дням.
 */
export function filterDirectorContextForDay(
  directorContext: string,
  date: Date,
  dayNumber: number,
  totalDays: number,
): string {
  if (!directorContext.trim()) return "";

  const { eventSentences, backgroundSentences } =
    splitDirectorContext(directorContext);

  const relevantEvents = eventSentences.filter((s) =>
    isSentenceRelevantForDay(s, date, dayNumber, totalDays)
  );

  return [...backgroundSentences, ...relevantEvents].join(" ").trim();
}

/**
 * Собрать answers для POST /generate из нарративного опросника + метаданных дня.
 *
 * Ключевые принципы:
 *  - Режиссёрский контекст (directorContext) + позиция в дуге → __arc_context__.
 *    RAG-сервис инжектирует это поле в системный промпт с пометкой «не цитировать».
 *  - Клиническая информация (препараты, события) → events_detail (в текст дневника).
 *  - overall_dynamics управляет как «dynamics» (daily), так и «period_dynamics» (exam_10d).
 */
export function buildGenerateAnswers(
  batchAnswers: Answers,
  dayNumber: number,
  totalDays: number,
  isoDate: string,
  directorContext: string,
  estimatedDischargeDate: string,
  docType: BatchDocType,
): Answers {
  const overallDynamics = batchAnswers.overall_dynamics;
  const admissionSeverity = batchAnswers.admission_severity ?? "moderate";
  const improvementPace = batchAnswers.improvement_pace;
  const finalState = batchAnswers.final_state;
  const keyMedications = batchAnswers.key_medications;
  const notableEvents = batchAnswers.notable_events;

  // ── Нарративная дуга (arc context) ──────────────────────────────────────────
  // Это метаданные для LLM-понимания, не для вставки в дневник.
  const arcParts: string[] = [];

  const pct = Math.round((dayNumber / totalDays) * 100);
  arcParts.push(`День госпитализации: ${dayNumber} из ${totalDays} (${pct}% периода пройдено).`);

  if (estimatedDischargeDate) {
    const daysLeft = daysBetweenIso(isoDate, estimatedDischargeDate);
    if (daysLeft !== null && daysLeft >= 0) {
      arcParts.push(`До ориентировочной выписки: ${daysLeft} дн.`);
    }
  }

  const severityLabel: Record<string, string> = {
    mild: "лёгкая",
    moderate: "средняя",
    severe: "тяжёлая",
  };
  arcParts.push(
    `Тяжесть при поступлении: ${severityLabel[String(admissionSeverity)] ?? String(admissionSeverity)}.`,
  );

  if (overallDynamics === "positive" && improvementPace) {
    const paceLabel: Record<string, string> = {
      fast: "быстрый (улучшение с первых дней)",
      moderate: "умеренный (постепенное улучшение)",
      slow: "медленный (старт медленный, ускорение к концу)",
    };
    arcParts.push(
      `Темп улучшения: ${paceLabel[String(improvementPace)] ?? String(improvementPace)}.`,
    );
  }

  if (typeof finalState === "string" && finalState.trim()) {
    arcParts.push(`Целевое состояние к последнему дню: ${finalState.trim()}.`);
  }

  // Фильтруем режиссёрский контекст: событийные предложения — только для
  // соответствующего дня, фоновые — для всех дней.
  const parsedDate = parseLocalDate(isoDate);
  const filteredContext = parsedDate
    ? filterDirectorContextForDay(directorContext, parsedDate, dayNumber, totalDays)
    : directorContext.trim();

  if (filteredContext) {
    arcParts.push(`Режиссёрская установка: ${filteredContext}`);
  }

  // ── Клиническая информация (для events_detail) ────────────────────────────
  const clinicalParts: string[] = [];

  const eventsDesc = describeNotableEvents(notableEvents);
  if (eventsDesc) {
    clinicalParts.push(`В данный период: ${eventsDesc}.`);
  }
  if (typeof keyMedications === "string" && keyMedications.trim()) {
    clinicalParts.push(`Терапия: ${keyMedications.trim()}.`);
  }

  // ── Базовые ответы (для daily-шаблона) ───────────────────────────────────
  const base: Answers = {
    dynamics: mapDynamicsToDaily(overallDynamics),
    productive_symptoms: "not_detected",
    mood: "even",
    behavior: "ordered",
    contact: ["productive", "polite_staff"],
    sleep: "not_disturbed",
    appetite: "preserved",
    tolerance: "good",
    complaints: "none",
  };

  // Диагноз прокидываем только для exam_10d (там есть freetext-поле «diagnosis»).
  const diagnosisValue = batchAnswers.diagnosis;
  const diagnosisStr =
    typeof diagnosisValue === "string" ? diagnosisValue.trim() : "";

  if (clinicalParts.length > 0) {
    base.events = ["consultation"];
    base.events_detail = clinicalParts.join(" ");
  }

  // Режиссёрский контекст + позиция → специальное поле для RAG-сервиса.
  // Это НЕ попадёт в текст дневника — RAG инжектирует его в системный промпт.
  if (arcParts.length > 0) {
    base.__arc_context__ = arcParts.join(" ");
  }

  if (docType === "exam_10d") {
    const interventions: string[] = [];
    if (Array.isArray(notableEvents)) {
      if (notableEvents.includes("ecg_eeg")) {
        interventions.push("ecg", "eeg");
      }
      if (notableEvents.includes("specialist_consult")) {
        interventions.push("psychologist");
      }
      if (notableEvents.includes("therapy_change")) {
        interventions.push("lab");
      }
    }

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
      syndrome: batchAnswers.leading_syndrome ?? "anxiety_depressive",
      comorbidities: ["none"],
      prescriptions: "see_list",
      period_dynamics: mapDynamicsToExam(overallDynamics),
      ...(diagnosisStr ? { diagnosis: diagnosisStr } : {}),
      ...(interventions.length > 0 ? { interventions } : {}),
    };
  }

  return base;
}
