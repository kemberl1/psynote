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

export type DayCalendar = "weekday" | "wednesday" | "saturday" | "sunday" | "monday";
export type SpeechLevel = "sounds" | "words" | "short_phrases" | "expanded" | "unknown";

export interface DayBrief {
  isoDate: string;
  dayNumber: number;
  periodIndex: number;
  periodLength: number;
  periodPct: number;
  role: DayRole;
  phase: DayPhase;
  calendar: DayCalendar;
  speechLevel: SpeechLevel;
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
  /** Понедельник после сб/вс, которые уже есть в этом пакете. */
  weekendRecap: boolean;
  /**
   * Формула «Дополнительные сведения о заболевании» на сб/вс
   * (не первые 3 дня госпитализации). Иначе null.
   */
  weekendDutyNote: string | null;
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

/** Родительский день отделения — среда. */
export function isParentDaySentence(s: string): boolean {
  return /родительск\w* дн|на свидан/.test(s.toLowerCase());
}

export function isRelativeVisitSentence(s: string): boolean {
  const lower = s.toLowerCase();
  if (isParentDaySentence(s)) return true;
  return (
    /встреч|визит|приход|приехала|приехал/.test(lower) &&
    /мам[аоыуе]|матер|отц[аеу]|родител|родствен/.test(lower)
  );
}

function classifySentence(s: string): keyof ExtractedFacts | "contrast" | "skip" {
  const lower = s.toLowerCase();
  if (
    /выходн|понедельник|вторник|сред[ауы]|четверг|пятниц|суббот|воскресень/.test(lower) ||
    isRelativeVisitSentence(s)
  ) {
    return "timed";
  }
  if (/в первые дни|при поступлении/.test(lower) && /в дальнейшем|затем|позднее/.test(lower)) {
    return "contrast";
  }
  if (/в первые дни|при поступлении|с первых дней/.test(lower)) return "early";
  if (/мг\/сут|левомепромазин|рисперидон|риперидон|вальпро|бипериден|терапи|дозировк|отмен/.test(lower)) {
    return "therapy";
  }
  if (/\d{1,2}[./]\d{1,2}/.test(s)) return "timed";
  if (/положительн\w+ динамик|стал более|снизилась частота|приблизилось к ровному/.test(lower)) {
    return "improvement";
  }
  if (/остается трудн|нуждается в длительн|нуждается в индивидуальном|альтернативн/.test(lower)) {
    return "residual";
  }
  if (/надзор|под наблюдением/.test(lower)) {
    return "skip";
  }
  if (
    /когнитивн|гигиеническ|ест самостоятельно|не взаимодейств|аппетит и сон|режимные моменты/.test(
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
    if (kind === "skip") continue;
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

/** Полевые акты (простыни, обувь, хаос) — только ранняя/средняя фаза, не финал. */
export function isFieldAct(text: string): boolean {
  return /простын|обув|хаотичн|возбужд|агресс|бьёт|бьет себя|царап|головой|растормож|суетлив|таска|обирает|стаскива|разбрасыв/.test(
    text.toLowerCase(),
  );
}

function calendarFor(iso: string): DayCalendar {
  const date = parseIso(iso);
  if (!date) return "weekday";
  const dow = date.getDay();
  if (dow === 6) return "saturday";
  if (dow === 0) return "sunday";
  if (dow === 1) return "monday";
  if (dow === 3) return "wednesday";
  return "weekday";
}

function diagnosisText(batchAnswers: Answers): string {
  const raw = batchAnswers.diagnosis;
  return typeof raw === "string" ? raw.trim() : "";
}

function intellectLock(diagnosis: string): string {
  if (/F71|F72|F73|умеренн\w* умственн|выраженн\w* умственн|тяжёл\w* умственн/i.test(diagnosis)) {
    return (
      "Интеллект ЭТОГО пациента — умственная отсталость умеренной (или выраженной) степени по диагнозу, " +
      "словами полностью. Не пиши «лёгкую», не пиши аббревиатуру «УО», не подставляй F70/F91 и не пиши «возрастную норму»."
    );
  }
  if (/F70|лёгк\w* умственн|легк\w* умственн/i.test(diagnosis)) {
    return (
      "Интеллект ЭТОГО пациента — умственная отсталость лёгкой степени по диагнозу, словами полностью. " +
      "Не подставляй F71/F91, не пиши «возрастную норму», не пиши аббревиатуру «УО»."
    );
  }
  if (/\bF7\d/i.test(diagnosis)) {
    return (
      "Интеллект — умственная отсталость по диагнозу (F7x), степень как в формулировке врача, словами полностью, не «УО»."
    );
  }
  return (
    "Интеллект ЭТОГО пациента — НЕ умственная отсталость: в диагнозе нет F70–F79. " +
    "Пиши «соответствует возрасту» / «на уровне возрастной нормы» (если бриф не задал иное). " +
    "ЗАПРЕЩЕНО копировать из образцов корпуса «умственную отсталость», «лёгкую/умеренную УО», " +
    "«снижен до уровня … отсталости» — это другой пациент."
  );
}

/** Уровень речи из эпикриза/диагноза — чтобы не приписывать словесные акты неговорящему. */
export function inferSpeechLevel(
  directorContext: string,
  diagnosis: string,
  finalState = "",
): SpeechLevel {
  const blob = `${directorContext} ${finalState} ${diagnosis}`.toLowerCase();
  if (
    /звукокомплекс|не говорит|безречев|невербальн|речь не сформир|отдельн\w* звук|собственная речь представлена/.test(
      blob,
    )
  ) {
    return "sounds";
  }
  if (/отдельн\w* слов|слова-предложен|лепетн/.test(blob)) return "words";
  if (/короткие фраз|простые фраз|фразовая речь не/.test(blob)) return "short_phrases";
  if (
    /развернут\w* предложен|фразовая речь сформир|речь фразов|собственн\w* речь фразов|отвечает развернуто/.test(
      blob,
    )
  ) {
    return "expanded";
  }
  if (/F72|выраженн\w* умственн/.test(diagnosis)) return "sounds";
  return "unknown";
}

function syndromeLockLines(syndrome: string): string[] {
  if (syndrome === "psychomotor_autoaggression") {
    return [
      "Клинический регистр: психомоторной расторможенности с аутоагрессией.",
      "НЕ пиши штамп «без агрессивных, аутоагрессивных и других опасных тенденций» — он спорит с синдромом.",
      "Острый аутоагрессивный акт — только если он в наблюдениях СЕГОДНЯ. Иначе не выдумывай самоповреждение и не снимай регистр этой формулой.",
      "Можно: критика снижена/не выявляется; воля; «возбуждений сегодня не было»; раздражение на коррекцию — если это в наблюдениях.",
    ];
  }
  if (syndrome === "psychomotor_aggression") {
    return [
      "Клинический регистр: психомоторной расторможенности с агрессией.",
      "НЕ пиши ежедневный штамп «без агрессивных тенденций», если регистр — с агрессией. Острый акт — только если он в наблюдениях сегодня.",
    ];
  }
  if (syndrome) {
    return [
      `Клинический регистр: ${SYNDROME_LABEL[syndrome] ?? syndrome}. ` +
        "Лексика регистра нужна, но не ставь острые проявления в каждый день — только если они в наблюдениях сегодня.",
    ];
  }
  return [];
}

function sexLockLines(sex: unknown): string[] {
  if (sex === "female") {
    return [
      "Пациент — девочка. Согласуй род во всём тексте: она, упорядочена, беспокойна, получала замечания, капризничала. Не пиши «он/упорядочен».",
    ];
  }
  if (sex === "male") {
    return [
      "Пациент — мальчик. Согласуй род во всём тексте: он, упорядочен, беспокоен, получал замечания, капризничал. Не пиши «она/упорядочена».",
    ];
  }
  return [];
}

function speechLockLines(level: SpeechLevel): string[] {
  const lines = [
    "Речь и контакт должны быть согласованы: если ребёнок не говорит словами — не приписывай словесные ответы, жалобы, пререкания, повышение голоса, «не раскрывает переживания».",
  ];
  if (level === "sounds") {
    lines.push(
      "Речевой профиль ЭТОГО ребёнка: обращённую речь понимает на уровне простых инструкций (если так в наблюдениях); собственная речь — звукокомплексы/звуки, не слова.",
    );
    lines.push(
      "ЗАПРЕЩЕНО сегодня: отвечает односложно, отвечает в плане заданного, переживаний не раскрывает, пререкается, спорит, повышает голос, предъявляет жалобы словами, продуктивный речевой контакт.",
    );
    lines.push(
      "МОЖНО: вокализации, звуки, жесты, берёт за руку, плач, двигательный протест, не удерживает дистанцию, зрительный/тактильный контакт.",
    );
  } else if (level === "words") {
    lines.push(
      "Речевой профиль: отдельные слова, не развёрнутые предложения. Не пиши «развёрнуто рассказывает» и не пиши «не раскрывает переживания в полном объёме» как у говорящего подростка.",
    );
  } else if (level === "short_phrases") {
    lines.push("Речевой профиль: короткие фразы. Не пиши развёрнутые предложения, если их нет в наблюдениях.");
  } else if (level === "expanded") {
    lines.push("Речевой профиль: доступна фразовая/развёрнутая речь — описывай её, не снижай до звукокомплексов.");
  }
  return lines;
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

function interpolateContact(
  facts: ExtractedFacts,
  role: DayRole,
  phase: DayPhase,
  speech: SpeechLevel,
): string[] {
  const nonverbal = speech === "sounds" || speech === "words";
  const contact: string[] = [];
  const blob = [...facts.traits, ...facts.laterBehaviors, ...facts.improvement].join(" ").toLowerCase();
  const isolated = /не взаимодейств|обособлен|практически не обща/.test(blob);
  const sociable = /активно обща|с детьми обща|сверстник|весел|шумн|подвижн/.test(blob);
  const later = phase === "titration" || phase === "improving" || phase === "residual";
  if (isolated && !(sociable && later)) contact.push("isolated");
  if (sociable && later && !nonverbal) contact.push("selective_children");
  if (
    !nonverbal &&
    /коррекци|замечан|каприз|плак/.test(blob) &&
    role !== "quiet"
  ) {
    contact.push("staff_remarks");
  }
  if (phase === "improving" || phase === "residual") {
    contact.push("calm_distance");
  }
  if (contact.length === 0) contact.push(nonverbal ? "isolated" : "calm_distance");
  return contact.filter((c) => !(nonverbal && (c === "does_not_disclose" || c === "productive")));
}

function isOccupation(text: string): boolean {
  return /телевизор|\bтв\b|рисова|конструктор|сюжетно|ролев|в игровой|смотрел тв/.test(
    text.toLowerCase(),
  );
}

/** Даты вида 21.07 / 21.07.2026 в тексте врача → фрагмент относится к этому календарному дню. */
export function extractDatedSnippets(
  text: string,
  year: number,
): { iso: string; snippet: string }[] {
  if (!text.trim()) return [];
  const re = /(?:с\s+|от\s+)?(\d{1,2})[./](\d{1,2})(?:[./](\d{2,4}))?/g;
  const marks: { index: number; iso: string }[] = [];
  let match: RegExpExecArray | null;
  while ((match = re.exec(text)) !== null) {
    const day = Number(match[1]);
    const month = Number(match[2]);
    let y = match[3] ? Number(match[3]) : year;
    if (y < 100) y += 2000;
    if (month < 1 || month > 12 || day < 1 || day > 31) continue;
    const iso = `${y}-${String(month).padStart(2, "0")}-${String(day).padStart(2, "0")}`;
    marks.push({ index: match.index, iso });
  }
  if (marks.length === 0) return [];
  const out: { iso: string; snippet: string }[] = [];
  for (let i = 0; i < marks.length; i++) {
    const start = marks[i].index;
    const end = i + 1 < marks.length ? marks[i + 1].index : text.length;
    const snippet = text.slice(start, end).replace(/[;,.\s]+$/, "").trim();
    if (snippet.length > 8) out.push({ iso: marks[i].iso, snippet });
  }
  return out;
}

function pickRotated(pool: string[], index: number, count: number): string[] {
  if (pool.length === 0 || count <= 0) return [];
  const out: string[] = [];
  for (let i = 0; i < Math.min(count, pool.length); i++) {
    out.push(pool[(index + i) % pool.length]);
  }
  return out;
}

/** Ключ полевого акта, чтобы простыни и обувь не жили в одном слоте и не копились по дням. */
export function fieldActKey(text: string): string {
  const t = text.toLowerCase();
  if (/простын/.test(t)) return "sheets";
  if (/обув/.test(t)) return "shoes";
  if (/хаотичн/.test(t)) return "chaos";
  if (/каприз|плак/.test(t)) return "cry";
  if (/раздраж|вербальн\w* коррекц/.test(t)) return "irritable";
  if (/бездеятел/.test(t)) return "idle";
  if (/возбужд|агресс/.test(t)) return "agitation";
  return t.slice(0, 32);
}

function takeUnused(pool: string[], used: Set<string>, count: number): string[] {
  const out: string[] = [];
  for (const item of pool) {
    if (out.length >= count) break;
    const k = fieldActKey(item);
    if (used.has(k)) continue;
    used.add(k);
    out.push(item);
  }
  return out;
}

function priorWeekendInPacket(days: ArcDayPlan[], index: number): boolean {
  if (calendarFor(days[index]?.isoDate ?? "") !== "monday") return false;
  let sawSat = false;
  let sawSun = false;
  for (let j = 0; j < index; j++) {
    const c = calendarFor(days[j].isoDate);
    if (c === "saturday") sawSat = true;
    if (c === "sunday") sawSun = true;
  }
  return sawSat && sawSun;
}

function fieldStopsForDay(observations: string[], includeFinalState: boolean): string[] {
  const obs = observations.join(" ").toLowerCase();
  const stops: string[] = [];
  if (!/простын/.test(obs)) stops.push("простыни / стаскивание белья — не сегодня");
  if (!/обув/.test(obs)) stops.push("обувь / обирание обуви — не сегодня");
  if (!includeFinalState && !/двер|за руку/.test(obs)) {
    stops.push("ведёт к двери / «гулять по отделению» — не сегодня");
  }
  return stops;
}

function parseIso(iso: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso.trim());
  if (!m) return null;
  return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
}

function formatIso(d: Date): string {
  const y = d.getFullYear();
  const mo = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${mo}-${day}`;
}

function dowOf(iso: string): number | null {
  const d = parseIso(iso);
  return d ? d.getDay() : null;
}

const WEEKDAY_RES: { dow: number; re: RegExp }[] = [
  { dow: 0, re: /воскресень/ },
  { dow: 1, re: /понедельник/ },
  { dow: 2, re: /вторник/ },
  { dow: 3, re: /сред[ауые]/ },
  { dow: 4, re: /четверг/ },
  { dow: 5, re: /пятниц/ },
  { dow: 6, re: /суббот/ },
];

function saturdayOf(date: Date): Date {
  const sat = new Date(date);
  // вс → вчера, сб → сегодня; иначе последняя суббота.
  sat.setDate(date.getDate() - ((date.getDay() + 1) % 7));
  return sat;
}

function formatWeekendSpan(sat: Date, sun: Date): string {
  const sD = sat.getDate();
  const eD = sun.getDate();
  const sM = sat.getMonth() + 1;
  const eM = sun.getMonth() + 1;
  if (sM === eM) return `${sD}-${eD}.${String(sM).padStart(2, "0")}`;
  return `${sD}.${String(sM).padStart(2, "0")}-${eD}.${String(eM).padStart(2, "0")}`;
}

/**
 * Формула бланка после диагноза на сб/вс.
 * Первые 3 дня госпитализации — без формулы (даже если выходной).
 */
export function weekendDutyNote(isoDate: string, dayNumber: number): string | null {
  if (dayNumber <= 3) return null;
  const date = parseIso(isoDate);
  if (!date) return null;
  const dow = date.getDay();
  if (dow !== 0 && dow !== 6) return null;
  const sat = saturdayOf(date);
  const sun = new Date(sat);
  sun.setDate(sat.getDate() + 1);
  return (
    `за период выходных дней с ${formatWeekendSpan(sat, sun)} ` +
    "под наблюдением дежурного мед персонала."
  );
}

function closestDays(
  days: ArcDayPlan[],
  pred: (d: ArcDayPlan) => boolean,
  anchorIso: string,
): ArcDayPlan[] {
  const candidates = days.filter(pred);
  if (candidates.length === 0) return [];
  const anchor = parseIso(anchorIso)?.getTime();
  if (anchor == null) return [candidates[0]];
  let best = candidates[0];
  let bestDist = Infinity;
  for (const c of candidates) {
    const t = parseIso(c.isoDate)?.getTime() ?? Infinity;
    const dist = Math.abs(t - anchor);
    if (dist < bestDist) {
      best = c;
      bestDist = dist;
    }
  }
  return [best];
}

function weekendPairInPacket(days: ArcDayPlan[], one: ArcDayPlan): ArcDayPlan[] {
  const date = parseIso(one.isoDate);
  if (!date) return [one];
  const sat = saturdayOf(date);
  const sun = new Date(sat);
  sun.setDate(sat.getDate() + 1);
  const satIso = formatIso(sat);
  const sunIso = formatIso(sun);
  const pair = days.filter((d) => d.isoDate === satIso || d.isoDate === sunIso);
  return pair.length > 0 ? pair : [one];
}

/** Опорная дата в пакете: явная дата, фаза повествования или позиция фразы. */
export function narrativeAnchorIso(
  sentence: string,
  directorContext: string,
  days: ArcDayPlan[],
  packetYear: number,
): string {
  if (days.length === 0) return "";
  const dated = extractDatedSnippets(sentence, packetYear);
  if (dated[0]) return dated[0].iso;
  const lower = sentence.toLowerCase();
  if (/в первые дни|при поступлении|с первых дней|в начале госпитал/.test(lower)) {
    return days[0].isoDate;
  }
  if (/перед выпиской|в последние дни|ближе к выписке|накануне выписки/.test(lower)) {
    return days[days.length - 1].isoDate;
  }
  const pos = directorContext.indexOf(sentence);
  if (pos >= 0) {
    const window = directorContext.slice(Math.max(0, pos - 140), pos + sentence.length + 80);
    const w = window.toLowerCase();
    if (/в первые дни|при поступлении|с первых дней|в начале госпитал/.test(w)) {
      return days[0].isoDate;
    }
    if (/перед выпиской|в последние дни|ближе к выписке|накануне выписки/.test(w)) {
      return days[days.length - 1].isoDate;
    }
    if (directorContext.length > 1 && days.length > 1) {
      const frac = pos / (directorContext.length - 1);
      return days[Math.round(frac * (days.length - 1))].isoDate;
    }
  }
  return days[Math.floor(days.length / 2)]?.isoDate ?? days[0].isoDate;
}

/**
 * Раскладывает фразы врача с датой / днём недели / родительским днём
 * на подходящие дни пакета (ближайшие к повествованию).
 */
export function assignTimedObservations(
  directorContext: string,
  days: ArcDayPlan[],
): Map<string, string[]> {
  const out = new Map<string, string[]>();
  const add = (iso: string, text: string) => {
    const list = out.get(iso) ?? [];
    list.push(text);
    out.set(iso, list);
  };
  if (!directorContext.trim() || days.length === 0) return out;
  const packetYear = Number(days[0].isoDate.slice(0, 4)) || new Date().getFullYear();
  const inPacket = new Set(days.map((d) => d.isoDate));

  for (const raw of splitSentences(directorContext)) {
    if (classifySentence(raw) === "therapy") continue;
    const s = raw.replace(/[.;]+$/, "");
    const lower = s.toLowerCase();
    const dated = extractDatedSnippets(s, packetYear)
      .map((d) => d.iso)
      .filter((iso) => inPacket.has(iso));
    const mentioned = WEEKDAY_RES.filter((x) => x.re.test(lower)).map((x) => x.dow);
    const parent = isParentDaySentence(s);
    const weekendKw = /выходн/.test(lower);
    const visit = isRelativeVisitSentence(s);
    const late = /перед выпиской|в последние дни|ближе к выписке|накануне выписки/.test(lower);
    const anchor = narrativeAnchorIso(s, directorContext, days, packetYear);

    let targets: ArcDayPlan[] = [];
    if (dated.length > 0) {
      targets = days.filter((d) => dated.includes(d.isoDate));
    } else if (mentioned.length === 1) {
      targets = closestDays(days, (d) => dowOf(d.isoDate) === mentioned[0], anchor);
    } else if (mentioned.length > 1) {
      const hit = closestDays(
        days,
        (d) => mentioned.includes(dowOf(d.isoDate) ?? -1),
        anchor,
      );
      targets =
        mentioned.includes(0) && mentioned.includes(6) && hit[0]
          ? weekendPairInPacket(days, hit[0])
          : hit;
    } else if (parent || (visit && !weekendKw)) {
      targets = closestDays(days, (d) => dowOf(d.isoDate) === 3, anchor);
    } else if (weekendKw) {
      const hit = closestDays(days, (d) => {
        const dow = dowOf(d.isoDate);
        return dow === 0 || dow === 6;
      }, anchor);
      targets = hit[0] ? weekendPairInPacket(days, hit[0]) : [];
    } else if (late) {
      targets = days.slice(-2);
    } else {
      continue;
    }

    for (const t of targets) add(t.isoDate, s);
  }
  return out;
}

/** Собрать брифы на каждый день пакета. */
export function compileArc(input: CompileArcInput): DayBrief[] {
  const { days, directorContext, batchAnswers } = input;
  const n = days.length;
  const facts = extractFacts(directorContext);
  const dynamics = batchAnswers.overall_dynamics;
  const diagnosis = diagnosisText(batchAnswers);
  const finalState =
    typeof batchAnswers.final_state === "string" ? batchAnswers.final_state : "";
  const speechLevel = inferSpeechLevel(directorContext, diagnosis, finalState);
  const examIndex = days.findIndex((d) => d.documentType === "exam_10d");
  const titrationIndex =
    examIndex >= 0 ? examIndex : Math.min(Math.floor(n * 0.4), Math.max(0, n - 1));
  const packetYear = Number((days[0]?.isoDate ?? "").slice(0, 4)) || new Date().getFullYear();
  const medSource = [
    typeof batchAnswers.key_medications === "string" ? batchAnswers.key_medications : "",
    ...facts.therapy,
  ]
    .filter(Boolean)
    .join("\n");
  const datedMeds = extractDatedSnippets(medSource, packetYear);
  const occupationPool = unique(
    [...facts.laterBehaviors, ...facts.traits, ...facts.improvement].filter(isOccupation),
  );
  const timedByDay = assignTimedObservations(directorContext, days);
  const visitPool = unique(
    [...facts.timed, ...splitSentences(directorContext)].filter(isRelativeVisitSentence),
  );

  const usedFieldKeys = new Set<string>();
  const fieldPoolAll = facts.laterBehaviors.filter(isFieldAct);
  const calmPoolAll = facts.laterBehaviors.filter((s) => !isFieldAct(s) && !isOccupation(s));

  const briefs: DayBrief[] = days.map((day, index) => {
    const periodPct = n <= 1 ? 100 : Math.round((index / (n - 1)) * 100);
    const phase = phaseFor(periodPct, day.dayNumber);
    const role = roleFor(day, index, n);
    const { mood, moodDetail } = interpolateMood(dynamics, phase);
    const behavior = interpolateBehavior(phase, role);
    const contact = interpolateContact(facts, role, phase, speechLevel);
    const includeFinalState = index >= n - 2 || (role === "exam" && periodPct >= 80);
    const medsToday = datedMeds.filter((m) => m.iso === day.isoDate).map((m) => m.snippet);
    const medsToDate = datedMeds.filter((m) => m.iso <= day.isoDate).map((m) => m.snippet);
    let therapyToday: string | null = null;
    if (medsToday.length > 0) {
      therapyToday = medsToday.join(" ");
    } else if (role === "exam" && medsToDate.length > 0) {
      therapyToday =
        "Схема к СЕГОДНЯШНЕЙ дате (события позже этой даты не включай): " +
        medsToDate.join("; ");
    } else if (datedMeds.length === 0 && (role === "exam" || index === titrationIndex)) {
      therapyToday = facts.therapy.join(" ") || null;
    }

    const obsCount = role === "quiet" ? 1 : role === "exam" ? 3 : 2;
    const canTakeField =
      role === "event" &&
      (phase === "admission" || phase === "field" || phase === "titration");
    const timedToday = timedByDay.get(day.isoDate) ?? [];
    const visitToday =
      role === "exam"
        ? visitPool
            .filter((s) => !timedToday.includes(s))
            .map((s) => `За период (не новый эпизод сегодня): ${s}`)
        : [];
    const observations: string[] = [...timedToday, ...visitToday];
    if (role === "exam") {
      observations.push(
        ...fieldPoolAll.slice(0, 2).map((s) => `За период (не новый эпизод сегодня): ${s}`),
      );
      observations.push(...pickRotated(calmPoolAll, index, 1));
    } else if (phase === "admission") {
      observations.push(...pickRotated(facts.early, index, 1));
      if (canTakeField) observations.push(...takeUnused(fieldPoolAll, usedFieldKeys, 1));
    } else if (phase === "field") {
      if (canTakeField) observations.push(...takeUnused(fieldPoolAll, usedFieldKeys, 1));
      else observations.push(...pickRotated(calmPoolAll, index, 1));
    } else if (phase === "titration") {
      if (canTakeField) observations.push(...takeUnused(fieldPoolAll, usedFieldKeys, 1));
      observations.push(...pickRotated(calmPoolAll, index, Math.max(1, obsCount - 1)));
    } else {
      observations.push(...pickRotated(calmPoolAll, index, obsCount));
    }
    if (phase !== "admission") {
      observations.push(...pickRotated(occupationPool, index, 1));
    }
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

    const reserved = unique(timedToday);
    const rest = unique(observations).filter((o) => !reserved.includes(o));
    const restCap =
      role === "exam" ? 7 : timedToday.length > 0 ? 3 : role === "quiet" ? 2 : 3;
    return {
      isoDate: day.isoDate,
      dayNumber: day.dayNumber,
      periodIndex: index,
      periodLength: n,
      periodPct,
      role,
      phase,
      calendar: calendarFor(day.isoDate),
      speechLevel,
      mood,
      moodDetail,
      behavior,
      contact,
      sleep: "not_disturbed",
      appetite: "preserved",
      observations: unique([...reserved, ...rest.slice(0, restCap)]),
      forbidden: [],
      therapyToday,
      includeFinalState,
      lengthHint: role === "exam" ? "exam" : role === "quiet" ? "short" : "medium",
      weekendRecap: priorWeekendInPacket(days, index),
      weekendDutyNote: weekendDutyNote(day.isoDate, day.dayNumber),
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
    extra.push(
      ...datedMeds
        .filter((m) => m.iso > briefs[i].isoDate)
        .map((m) => m.snippet),
    );
    const timedHere = new Set(timedByDay.get(briefs[i].isoDate) ?? []);
    if (briefs[i].role !== "exam") {
      extra.push(...visitPool.filter((s) => !timedHere.has(s)));
    }
    if (briefs[i].phase === "admission" || briefs[i].phase === "field") {
      extra.push(...facts.improvement, ...facts.residual);
    }
    if (briefs[i].phase === "improving" || briefs[i].phase === "residual") {
      extra.push(...facts.laterBehaviors.filter(isFieldAct), ...facts.early);
    }
    const stops = fieldStopsForDay(briefs[i].observations, briefs[i].includeFinalState);
    briefs[i].forbidden = unique([...stops, ...extra, ...others]).slice(0, 22);
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

  const calendarLabel: Record<DayCalendar, string> = {
    weekday: "будний день",
    wednesday: "среда, родительский день отделения",
    saturday: "суббота, выходной",
    sunday: "воскресенье, выходной",
    monday: "понедельник после выходных",
  };
  if (brief.calendar === "monday" && !brief.weekendRecap) {
    lines.push("Календарь: понедельник (не после сб/вс этого пакета).");
  } else {
    lines.push(`Календарь: ${calendarLabel[brief.calendar]}.`);
  }
  if (brief.calendar === "wednesday") {
    lines.push(
      "Родительский день отделения — СРЕДА. Встречи с родственниками, свидания и события «в родительский день» пиши СЕГОДНЯ, если они в наблюдениях. На другие дни не переноси.",
    );
  }
  if (brief.calendar === "saturday" || brief.calendar === "sunday") {
    if (brief.weekendDutyNote) {
      lines.push(
        "Выходной (не первые 3 дня госпитализации): наблюдения дня — в психическом статусе. " +
          `После диагноза в «Дополнительные сведения о заболевании» напиши РОВНО: «${brief.weekendDutyNote}» ` +
          "Эту формулу не ставь в психический статус и не меняй даты. " +
          "Не пиши построения/занятия как в будни. Визиты и прогулки — только если они в наблюдениях сегодня.",
      );
    } else {
      lines.push(
        "Выходной, но это один из первых трёх дней госпитализации: " +
          "«Дополнительные сведения о заболевании» оставь «нет». " +
          "НЕ пиши формулу «за период выходных дней» и «под наблюдением дежурного мед персонала». " +
          "Наблюдения дня — в психическом статусе. Визиты и прогулки — только если они в наблюдениях сегодня.",
      );
    }
  } else if (brief.weekendRecap) {
    lines.push(
      "Понедельник после сб/вс ЭТОГО пакета: в психическом статусе 1–2 фразы «за период выходных» — сон, фон, общение, если это следует из портрета. Не выдумывай прогулки, визиты и инциденты, которых нет в нарративе. «Дополнительные сведения о заболевании» — «нет», если нет сведений о болезни. Формулу про дежурный персонал на понедельнике НЕ пиши — она только на сб/вс.",
    );
  } else if (brief.calendar === "monday") {
    lines.push(
      "Сегодня понедельник, но субботы–воскресенья в этом пакете ещё не было — НЕ пиши «за период выходных дней».",
    );
  }

  const diagnosis = diagnosisText(batchAnswers);
  if (diagnosis) {
    lines.push(
      `Диагноз основного заболевания пиши РОВНО: ${diagnosis}. ` +
        "Не подставляй другой код МКБ (не F70 вместо F71, не F91, не выдуманный). " +
        "Сопутствующие — «не выявлено» / «—», если врач их не указал.",
    );
    lines.push(intellectLock(diagnosis));
  } else {
    lines.push(
      "Код МКБ не дан — в диагнозе плейсхолдер [ОСНОВНОЙ_ДИАГНОЗ], не выдумывай F-код. " +
        "Интеллект и речь — только из ответов/брифа ЭТОГО пациента, не из образцов корпуса.",
    );
  }
  lines.push(...speechLockLines(brief.speechLevel));
  lines.push(...sexLockLines(batchAnswers.patient_sex));

  const syndrome = batchAnswers.leading_syndrome;
  if (typeof syndrome === "string" && syndrome) {
    lines.push(...syndromeLockLines(syndrome));
  }

  if (estimatedDischargeDate) {
    lines.push(`Ориентировочная выписка: ${estimatedDischargeDate}.`);
  }

  if (brief.observations.length > 0) {
    lines.push("СЕГОДНЯ опиши через наблюдения врача ТОЛЬКО это:");
    for (const o of brief.observations) lines.push(`• ${o}`);
  }
  if (brief.therapyToday) {
    lines.push(
      `Сегодня день коррекции/фиксации схемы — отрази в «Назначения» и/или «План лечения», не прячь за «см. лист назначений»: ${brief.therapyToday}`,
    );
  } else {
    lines.push(
      "Терапию и дозировки сегодня НЕ перечисляй — «Назначения: см. лист назначений». Не переноси смены схемы с других дат.",
    );
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
      "Целевое состояние к выписке / финальный статус сегодня НЕ пиши и НЕ цитируй — оно только для последних дней периода и позднего осмотра 10 дней, не для середины госпитализации.",
    );
  }
  if (brief.forbidden.length > 0) {
    lines.push("НЕ повторяй сегодня (уже/ещё не этот день). Не перефразируй эти эпизоды:");
    for (const f of brief.forbidden.slice(0, 12)) {
      lines.push(`— ${f.slice(0, 180)}`);
    }
  }

  if (brief.lengthHint === "short") {
    lines.push(
      "Тихий день: полный бланк ежедневного осмотра (все строки шаблона). Психический статус ВСЕГДА с сознания и ориентировки, дальше — 1–2 наблюдения за сегодня. Соматический статус пиши кратко. Жалобы/анамнез без событий — «не предъявляет» / «без дополнений».",
    );
  } else if (brief.lengthHint === "medium") {
    lines.push(
      "Событийный день: полный бланк ежедневного осмотра, статус с сознания и ориентировки, 2–3 наблюдения за сегодня. Соматический статус пиши кратко.",
    );
  } else {
    lines.push(
      "Осмотр 10 дней: бланк совместного осмотра (ОСМОТР / лечащим врачом совместно с заведующим отделением), развёрнутый психический статус (его изменение) и этапный эпикриз ЗА ПЕРИОД. Соматику в психический статус не тащи.",
    );
  }
  lines.push(
    "Конкретика дня: можно дорисовать быт отделения ИЗ портрета (бездеятелен → в палате/игровой; не взаимодействует → обособлен; полевой → ходит по палате). Это перевод эпикриза, не новые факты.",
  );
  lines.push(
    "Нельзя выдумывать факты, которых нет в контексте: прогулки, визиты, консультации, процедуры, самоповреждение, смену схемы, «выходные» не в тот календарный день.",
  );
  lines.push(
    "Клинические константы ЭТОГО пациента (диагноз, интеллект, речь, контакт) держи. Образцы корпуса — чужие дети: не копируй их МКБ, умственную отсталость, звукокомплексы, простыни, обувь, если этого нет в брифе. Штампы статуса — тот же смысл, разные слова.",
  );
  lines.push(
    "Не пиши: одинаковые фразы с соседними днями; «вероятно»; аббревиатура «УО»; жалобы и «без дополнений» внутри статуса. " +
      "«под наблюдением/надзором персонала» в статусе запрещено" +
      (brief.weekendDutyNote
        ? " — исключение только строка «Дополнительные сведения о заболевании» с формулой дежурного персонала."
        : "."),
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
  if (brief.weekendDutyNote) {
    out.additional_info = "present";
    out.additional_info_detail = brief.weekendDutyNote;
  }
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
