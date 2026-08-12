// Раскладка пакетного нарратива врача в дневные брифы.
// Врач пишет эпикриз периода; реальные дневники корпуса — короткие срезы
// с 1–2 новыми наблюдениями. Этот модуль «допетривает» дугу без копипаста.

import type { Answers } from "../api/types";

export interface ArcDayPlan {
  isoDate: string;
  dayNumber: number;
  documentType: "daily" | "exam_10d";
}

export type DayRole = "quiet" | "event" | "exam";
export type DayPhase = "admission" | "field" | "titration" | "improving" | "residual";

export interface DayBrief {
  isoDate: string;
  dayNumber: number;
  periodIndex: number;
  periodLength: number;
  periodPct: number;
  role: DayRole;
  phase: DayPhase;
  mood: string;
  moodDetail: string[];
  behavior: string;
  contact: string[];
  sleep: string;
  appetite: string;
  observations: string[];
  forbidden: string[];
  therapyToday: string | null;
  includeFinalState: boolean;
  lengthHint: "short" | "medium" | "exam";
}

export interface CompileArcInput {
  days: ArcDayPlan[];
  directorContext: string;
  batchAnswers: Answers;
  estimatedDischargeDate: string;
}

interface ExtractedFacts {
  early: string[];
  laterBehaviors: string[];
  traits: string[];
  therapy: string[];
  improvement: string[];
  residual: string[];
  timed: string[];
}

const SYNDROME_LABEL: Record<string, string> = {
  behavioral: "поведенческих нарушений",
  anxious: "тревожный",
  depressive: "депрессивный",
  psychomotor_aggression: "психомоторной расторможенности с агрессией",
  psychomotor_autoaggression: "психомоторной расторможенности с аутоагрессией",
  affective_volitional: "аффективно-волевой неустойчивости",
  psychopathic: "психопатоподобный",
  asthenic: "астенический",
};

function splitSentences(text: string): string[] {
  return text
    .split(/(?<=\.)\s+|(?<=\n)/)
    .map((s) => s.trim())
    .filter(Boolean);
}

function splitListish(chunk: string): string[] {
  return chunk
    .split(/,\s+(?=[а-яё])/i)
    .map((s) => s.replace(/^[а-яё]?\s*/, "").trim())
    .map((s) => s.replace(/[.;]+$/, "").trim())
    .filter((s) => s.length > 12);
}

function classifySentence(s: string): keyof ExtractedFacts | "contrast" {
  const lower = s.toLowerCase();
  if (/выходн|понедельник|вторник|сред[ауы]|четверг|пятниц|суббот|воскресень/.test(lower)) {
    return "timed";
  }
  if (/в первые дни|при поступлении/.test(lower) && /в дальнейшем|затем|позднее/.test(lower)) {
    return "contrast";
  }
  if (/в первые дни|при поступлении|с первых дней/.test(lower)) return "early";
  if (/мг\/сут|левомепромазин|рисперидон|риперидон|вальпро|бипериден|терапи|дозировк|отмен/.test(lower)) {
    return "therapy";
  }
  if (/положительн\w+ динамик|стал более|снизилась частота|приблизилось к ровному/.test(lower)) {
    return "improvement";
  }
  if (/остается трудн|нуждается в длительн|нуждается в индивидуальном|альтернативн/.test(lower)) {
    return "residual";
  }
  if (
    /когнитивн|надзор|гигиеническ|ест самостоятельно|не взаимодейств|аппетит и сон|режимные моменты/.test(
      lower,
    )
  ) {
    return "traits";
  }
  return "laterBehaviors";
}

export function extractFacts(directorContext: string): ExtractedFacts {
  const facts: ExtractedFacts = {
    early: [],
    laterBehaviors: [],
    traits: [],
    therapy: [],
    improvement: [],
    residual: [],
    timed: [],
  };
  if (!directorContext.trim()) return facts;

  for (const raw of splitSentences(directorContext)) {
    const kind = classifySentence(raw);
    if (kind === "contrast") {
      const parts = raw.split(/в дальнейшем|затем|позднее/i);
      const earlyPart = (parts[0] ?? "")
        .replace(/в первые дни|при поступлении|с первых дней/gi, "")
        .replace(/^[,.\s]+/, "")
        .trim();
      const laterPart = (parts.slice(1).join(" ") ?? "").replace(/^[,.\s]+/, "").trim();
      if (earlyPart) facts.early.push(earlyPart.replace(/[.;]+$/, ""));
      if (laterPart) facts.laterBehaviors.push(...splitListish(laterPart));
      continue;
    }
    if (kind === "laterBehaviors") {
      const bits = splitListish(raw);
      if (bits.length > 1) facts.laterBehaviors.push(...bits);
      else facts.laterBehaviors.push(raw.replace(/[.;]+$/, ""));
      continue;
    }
    facts[kind].push(raw.replace(/[.;]+$/, ""));
  }

  facts.laterBehaviors = unique(facts.laterBehaviors);
  facts.traits = unique(facts.traits);
  return facts;
}

function unique(items: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const item of items) {
    const key = item.toLowerCase().slice(0, 48);
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(item);
  }
  return out;
}

function phaseFor(pct: number, dayNumber: number): DayPhase {
  if (dayNumber <= 3) return "admission";
  if (pct < 35) return "field";
  if (pct < 55) return "titration";
  if (pct < 80) return "improving";
  return "residual";
}

function roleFor(day: ArcDayPlan, index: number, n: number): DayRole {
  if (day.documentType === "exam_10d") return "exam";
  if (index === n - 1) return "event";
  return index % 3 === 0 ? "event" : "quiet";
}

function interpolateMood(
  dynamics: unknown,
  phase: DayPhase,
): { mood: string; moodDetail: string[] } {
  if (dynamics === "negative") {
    return { mood: "unstable", moodDetail: ["irritability"] };
  }
  if (dynamics === "wavy") {
    return phase === "improving" || phase === "residual"
      ? { mood: "even", moodDetail: [] }
      : { mood: "unstable", moodDetail: ["lability"] };
  }
  if (dynamics === "stable") {
    return { mood: "even", moodDetail: [] };
  }
  switch (phase) {
    case "admission":
    case "field":
      return { mood: "unstable", moodDetail: ["irritability", "lability"] };
    case "titration":
      return { mood: "unstable", moodDetail: ["lability"] };
    case "improving":
      return { mood: "even", moodDetail: [] };
    case "residual":
      return { mood: "even", moodDetail: [] };
  }
}

function interpolateBehavior(phase: DayPhase, role: DayRole): string {
  if (phase === "admission") return "restless";
  if (phase === "field") return role === "event" ? "restless" : "minor_remarks";
  if (phase === "titration") return "minor_remarks";
  if (phase === "improving") return "minor_remarks";
  return "ordered";
}

function interpolateContact(facts: ExtractedFacts, role: DayRole, phase: DayPhase): string[] {
  const contact: string[] = [];
  const blob = [...facts.traits, ...facts.laterBehaviors].join(" ").toLowerCase();
  if (/не взаимодейств|обособлен/.test(blob)) contact.push("isolated");
  if (/коррекци|замечан|каприз|плак/.test(blob) && role !== "quiet") {
    contact.push("staff_remarks");
  }
  if (phase === "improving" || phase === "residual") {
    contact.push("calm_distance");
  }
  if (contact.length === 0) contact.push("does_not_disclose");
  return contact;
}

function pickRotated(pool: string[], index: number, count: number): string[] {
  if (pool.length === 0 || count <= 0) return [];
  const out: string[] = [];
  for (let i = 0; i < Math.min(count, pool.length); i++) {
    out.push(pool[(index + i) % pool.length]);
  }
  return out;
}

function parseIso(iso: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso.trim());
  if (!m) return null;
  return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
}

function timedForDay(
  directorContext: string,
  day: ArcDayPlan,
  periodLength: number,
): string[] {
  if (!directorContext.trim()) return [];
  const date = parseIso(day.isoDate);
  if (!date) return [];
  const dow = date.getDay();
  const isWeekend = dow === 0 || dow === 6;
  const out: string[] = [];
  for (const s of splitSentences(directorContext)) {
    const lower = s.toLowerCase();
    if (/выходн/.test(lower) && isWeekend) {
      out.push(s);
      continue;
    }
    const dowMap: Record<number, RegExp> = {
      0: /воскресень/,
      1: /понедельник/,
      2: /вторник/,
      3: /сред[ауы]/,
      4: /четверг/,
      5: /пятниц/,
      6: /суббот/,
    };
    const re = dowMap[dow];
    if (re && re.test(lower)) out.push(s);
    if (/перед выпиской|в последние дни/.test(lower) && day.dayNumber >= periodLength - 1) {
      out.push(s);
    }
  }
  return unique(out);
}

/** Собрать брифы на каждый день пакета. */
export function compileArc(input: CompileArcInput): DayBrief[] {
  const { days, directorContext, batchAnswers } = input;
  const n = days.length;
  const facts = extractFacts(directorContext);
  const dynamics = batchAnswers.overall_dynamics;
  const examIndex = days.findIndex((d) => d.documentType === "exam_10d");
  const titrationIndex =
    examIndex >= 0 ? examIndex : Math.min(Math.floor(n * 0.4), Math.max(0, n - 1));

  const briefs: DayBrief[] = days.map((day, index) => {
    const periodPct = n <= 1 ? 100 : Math.round((index / (n - 1)) * 100);
    const phase = phaseFor(periodPct, day.dayNumber);
    const role = roleFor(day, index, n);
    const { mood, moodDetail } = interpolateMood(dynamics, phase);
    const behavior = interpolateBehavior(phase, role);
    const contact = interpolateContact(facts, role, phase);
    const includeFinalState =
      role === "exam" || index >= n - 2;
    const therapyToday =
      role === "exam" || index === titrationIndex
        ? facts.therapy.join(" ") || null
        : null;

    const obsCount = role === "quiet" ? 1 : role === "exam" ? 3 : 2;
    const observations: string[] = [];
    if (phase === "admission") observations.push(...pickRotated(facts.early, index, 1));
    observations.push(...pickRotated(facts.laterBehaviors, index, obsCount));
    if (role !== "quiet") {
      observations.push(...pickRotated(facts.traits, index, 1));
    } else {
      observations.push(...pickRotated(facts.traits, index + 3, 1));
    }
    if (phase === "improving" || phase === "residual") {
      observations.push(...pickRotated(facts.improvement, 0, 1));
    }
    if (phase === "residual") {
      observations.push(...pickRotated(facts.residual, 0, 1));
    }
    observations.push(...timedForDay(directorContext, day, n));

    return {
      isoDate: day.isoDate,
      dayNumber: day.dayNumber,
      periodIndex: index,
      periodLength: n,
      periodPct,
      role,
      phase,
      mood,
      moodDetail,
      behavior,
      contact,
      sleep: "not_disturbed",
      appetite: "preserved",
      observations: unique(observations).slice(0, role === "exam" ? 6 : 4),
      forbidden: [],
      therapyToday,
      includeFinalState,
      lengthHint: role === "exam" ? "exam" : role === "quiet" ? "short" : "medium",
    };
  });

  for (let i = 0; i < briefs.length; i++) {
    const others = briefs
      .filter((_, j) => j !== i)
      .flatMap((b) => b.observations);
    const extra: string[] = [];
    if (!briefs[i].includeFinalState) {
      const fs = batchAnswers.final_state;
      if (typeof fs === "string" && fs.trim()) extra.push(fs.trim());
    }
    if (!briefs[i].therapyToday) extra.push(...facts.therapy);
    if (briefs[i].phase === "admission" || briefs[i].phase === "field") {
      extra.push(...facts.improvement, ...facts.residual);
    }
    briefs[i].forbidden = unique([...extra, ...others]).slice(0, 16);
  }

  return briefs;
}

/** Текст брифа в системный промпт — срез ЭТОГО дня, не эпикриз периода. */
export function formatDayBrief(
  brief: DayBrief,
  batchAnswers: Answers,
  estimatedDischargeDate: string,
): string {
  const lines: string[] = [];
  lines.push(
    `День госпитализации: ${brief.dayNumber} (номер суток от поступления, не доля периода).`,
  );
  lines.push(
    `День в выбранном периоде: ${brief.periodIndex + 1} из ${brief.periodLength} (${brief.periodPct}% периода).`,
  );
  const phaseLabel: Record<DayPhase, string> = {
    admission: "поступление / первые сутки — адаптация, исходная острота",
    field: "полевое / неустойчивое поведение, коррекция ещё слабо держит",
    titration: "подбор терапии, поведение ещё трудное",
    improving: "на фоне коррекции — упорядоченнее, возбуждений меньше",
    residual: "остаточная трудность, ближе к целевому состоянию",
  };
  lines.push(`Фаза дуги: ${phaseLabel[brief.phase]}.`);
  const roleLabel: Record<DayRole, string> = {
    quiet: "тихий день — коротко, 1–2 наблюдения, без эпикриза",
    event: "событийный день — 2–3 конкретных наблюдения за СЕГОДНЯ",
    exam: "расширенный осмотр 10 дней — полный статус + этапный эпикриз периода",
  };
  lines.push(`Роль дня: ${roleLabel[brief.role]}.`);

  const syndrome = batchAnswers.leading_syndrome;
  if (typeof syndrome === "string" && syndrome) {
    lines.push(
      `Клинический регистр: ${SYNDROME_LABEL[syndrome] ?? syndrome}. ` +
        "Лексика регистра нужна, но не ставь острые проявления в каждый день — только если они в наблюдениях сегодня.",
    );
  }

  if (estimatedDischargeDate) {
    lines.push(`Ориентировочная выписка: ${estimatedDischargeDate}.`);
  }

  if (brief.observations.length > 0) {
    lines.push("СЕГОДНЯ опиши через наблюдения врача ТОЛЬКО это:");
    for (const o of brief.observations) lines.push(`• ${o}`);
  }
  if (brief.therapyToday) {
    lines.push(`Терапию сегодня можно кратко отразить: ${brief.therapyToday}`);
  } else {
    lines.push("Терапию и дозировки сегодня НЕ перечисляй — это не день коррекции схемы.");
  }
  if (brief.includeFinalState) {
    const fs = batchAnswers.final_state;
    if (typeof fs === "string" && fs.trim()) {
      lines.push(
        `Целевое/текущее состояние (можно опереться, не копировать дословно): ${fs.trim()}`,
      );
    }
  } else {
    lines.push(
      "Целевое состояние к концу периода сегодня НЕ пиши и НЕ цитируй — оно для последних дней и осмотра 10 дней.",
    );
  }
  if (brief.forbidden.length > 0) {
    lines.push("НЕ повторяй сегодня (уже/ещё не этот день):");
    for (const f of brief.forbidden.slice(0, 8)) {
      lines.push(`— ${f.slice(0, 180)}`);
    }
  }

  if (brief.lengthHint === "short") {
    lines.push(
      "Длина как в корпусе сборника: один плотный абзац психического статуса (не шаблон-эпикриз). Соматика/неврология — коротко, без копипаста полного осмотра.",
    );
  } else if (brief.lengthHint === "medium") {
    lines.push(
      "Ежедневная запись: конкретные наблюдения за сегодня + короткий соматический/неврологический блок. Не пересказывай весь период.",
    );
  } else {
    lines.push(
      "Осмотр 10 дней: развёрнутый статус и этапный эпикриз динамики ЗА ПЕРИОД. Ежедневные мелочи не копируй дословно из других дней — обобщи дугу.",
    );
  }
  lines.push(
    "Запрещено: одинаковые фразы с соседними днями; полный психический статус-копия финального абзаца врача во все дни; выдуманные прогулки/визиты.",
  );
  return lines.join("\n");
}

export function applyBriefToAnswers(
  base: Answers,
  brief: DayBrief,
  batchAnswers: Answers,
  estimatedDischargeDate: string,
): Answers {
  const out: Answers = {
    ...base,
    dynamics: base.dynamics,
    mood: brief.mood,
    behavior: brief.behavior,
    contact: brief.contact,
    sleep: brief.sleep,
    appetite: brief.appetite,
  };
  if (brief.moodDetail.length > 0) out.mood_detail = brief.moodDetail;
  if (brief.behavior === "violates" || brief.behavior === "restless") {
    const blob = brief.observations.join(" ").toLowerCase();
    const detail: string[] = [];
    if (/агресс/.test(blob)) detail.push("aggression");
    if (/каприз|протест|раздраж/.test(blob)) detail.push("protest");
    if (detail.length > 0) out.behavior_detail = detail;
  }
  out.__arc_context__ = formatDayBrief(brief, batchAnswers, estimatedDischargeDate);
  return out;
}
