// Раскладка пакетного нарратива врача в дневные брифы.
// Врач пишет эпикриз периода; реальные дневники корпуса — короткие срезы
// с 1–2 новыми наблюдениями. Этот модуль «допетривает» дугу без копипаста.

import type { Answers } from "../api/types";
import { fixObviousTypos } from "./typoFixes";

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
  /** Анамнез поступления — только осмотр 10 дней, никогда как «сегодня». */
  historyNotes: string[];
  includeFinalState: boolean;
  lengthHint: "short" | "medium" | "exam";
  /** Понедельник после пропущенных сб/вс этого пакета. */
  weekendRecap: boolean;
  /**
   * Формула «Дополнительные сведения о заболевании» на понедельнике
   * после выходных (не первые 3 дня госпитализации). Иначе null.
   */
  weekendDutyNote: string | null;
  /** События сб–вс — только в доп. сведения понедельника, не как «сегодня». */
  weekendRecapNotes: string[];
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
  history: string[];
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

// «мед. персоналом», «таб. Сертралина», «ст. 29», «т. д.», инициалы —
// точка не конец предложения (иначе в бриф шли обрывки «С мед»).
const NO_BREAK_BEFORE_DOT =
  /(?:^|[\s(«"])(?:мед|таб|капс|р-ра|р-р|ст|им|др|см|кап|гр|т\.\s?[дпе]|[а-яё])$/i;

function splitSentences(text: string): string[] {
  const out: string[] = [];
  // «…претендовал.На фоне» — врач пропустил пробел после точки.
  const re = /\.\s+|\.(?=[А-ЯЁ][а-яё])|\n/g;
  let start = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) {
    const isDot = m[0] !== "\n";
    if (isDot && NO_BREAK_BEFORE_DOT.test(text.slice(start, m.index))) continue;
    out.push(text.slice(start, isDot ? m.index + 1 : m.index));
    start = m.index + m[0].length;
  }
  out.push(text.slice(start));
  return out.map((s) => s.trim()).filter(Boolean);
}

/** Абзацы нарратива → предложения (эпизод не выходит за свой абзац). */
function splitParagraphs(text: string): string[][] {
  return text
    .split(/\n+/)
    .map((p) => splitSentences(p))
    .filter((p) => p.length > 0);
}

// Короче этого кусок перечисления — не наблюдение дня («тихим голосом»,
// «волосы длинные»), а часть соседней фразы.
const MIN_LIST_BIT = 25;

function splitListish(chunk: string): string[] {
  const parts = chunk
    .split(/,\s+(?=[а-яё])/i)
    .map((s) => s.trim().replace(/[.;]+$/, "").trim())
    .filter(Boolean);
  const out: string[] = [];
  for (const part of parts) {
    if (out.length > 0 && part.length < MIN_LIST_BIT) out[out.length - 1] += `, ${part}`;
    else out.push(part);
  }
  if (out.length > 1 && out[0].length < MIN_LIST_BIT) {
    out.splice(0, 2, `${out[0]}, ${out[1]}`);
  }
  return out.filter((s) => s.length > 12);
}

/** Родительский день отделения — среда. */
export function isParentDaySentence(s: string): boolean {
  return /родительск[а-яё]* дн|на свидан/.test(s.toLowerCase());
}

export function isRelativeVisitSentence(s: string): boolean {
  const lower = s.toLowerCase();
  if (isParentDaySentence(s)) return true;
  return (
    /встреч|визит|приход|пришл[аи]|пришёл|пришел|приехала|приехал/.test(lower) &&
    /мам[аоыуе]|матер|мать|отц[аеу]|отец|родител|родствен/.test(lower)
  );
}

/** Заключение на день выписки: «может быть выписан…», требование выписки. */
export function isDischargeDaySentence(s: string): boolean {
  return isDischargeSentence(s) || /может быть выписан/i.test(s);
}

/**
 * Убрать отрицания возбуждения: «агрессию не проявлял», «без агрессии»,
 * «возбуждений сегодня не было». Иначе «агресс» в отрицании делал тихого
 * ребёнка «двигательно беспокойным» и включал запреты про возбуждение.
 */
export function stripNegatedAgitation(text: string): string {
  const agit = "(?:возбужд|агресс|растормож|суетлив|конфликт)\\S*";
  const absent =
    "(?:не\\s+(?:проявлял\\S*|было|был[аи]?|отмечал\\S*|наблюдал\\S*|выявл\\S*|провоцировал\\S*|вступал\\S*|обнаружива\\S*|определя\\S*)|нет|отсутств\\S*)";
  return text
    .toLowerCase()
    .replace(new RegExp(`(?<![а-яё])(?:не|без|отсутств\\S*)\\s+(?:\\S+\\s+){0,2}?${agit}`, "g"), " ")
    .replace(new RegExp(`${agit}[,\\s]+(?:\\S+[,\\s]+){0,8}?${absent}`, "g"), " ");
}

/** Требование выписки / заключение о выписке — событие дня выписки, не «фон». */
export function isDischargeSentence(s: string): boolean {
  return /по\s+(?:категорическому\s+)?требовани|настаива[а-яё]*\s+на\s+выписк|недобровольн|ст\.\s*29|выписан[а-яё]*\s+по\s+требовани|преждевременн[а-яё]*\s+выписк|желани[а-яё]*\s+выписаться/i.test(
    s,
  );
}

/** Поступление / соц.анамнез — не событие сегодняшнего дня в отделении. */
export function isAdmissionHistory(s: string): boolean {
  const lower = s.toLowerCase();
  if (
    /направлен[а-яё]*\s+на\s+госпитализац|направление\s+на\s+госпитализац|пакет документов/.test(
      lower,
    )
  ) {
    return true;
  }
  if (
    /интернат|из\s+интернат|забрал[аи]\w*\s+.{0,50}интернат|школ[аеуы]\s*[-–]?\s*интернат/.test(
      lower,
    )
  ) {
    return true;
  }
  if (/(?<![а-яё])дд[ие](?![а-яё])|детск[а-яё]+\s+дом\s+инвалид/.test(lower)) return true;
  if (/пребывани[а-яё]*.{0,40}\d+\s*дн.{0,30}недел/.test(lower)) return true;
  if (/до\s+госпитализац|до\s+поступлен/.test(lower) && /интернат|дд[ие]|направлен/.test(lower)) {
    return true;
  }
  if (/при[её]мн[а-яё]*\s+поко|в при[её]мн/.test(lower)) return true;
  if (/сантранспорт/.test(lower)) return true;
  if (/доставлен/.test(lower) && /при[её]мн|стационар|госпитализ/.test(lower)) return true;
  if (/после выписки/.test(lower)) return true;
  // Сообщение о суицидальных мыслях — анамнез; отрицание тенденций
  // остаётся наблюдением дня.
  if (
    /суицидальн[а-яё]*\s+мысл|суицидн[а-яё]*\s+мысл|мысл[а-яё]*\s+о\s+смерти/.test(lower) &&
    !/отрица|не высказыва|не обнаружива|не выявл|нет\b/.test(lower)
  ) {
    return true;
  }
  // Анамнез суицидального поведения и причины поступления — не событие дня.
  if (
    /попытк[а-яё]*\s+суицид|суицидн[а-яё]*\s+попытк|наносил[а-яё]*\s+(?:себе\s+)?порез|порезал|лезви|убить себя|эпизод[а-яё]*\s+самоповрежд|причин[а-яё]*\s+госпитализац|\d+\s*(?:-\s*\d+\s*)?(?:лет|год[а-яё]*)\s+назад|после смерти/.test(
      lower,
    )
  ) {
    return true;
  }
  if (/рекомендованн[а-яё]+\s+лечени[а-яё]+\s+принимал/.test(lower)) return true;
  return false;
}

function isTherapyLeakObservation(text: string): boolean {
  const t = text.toLowerCase();
  return /инъекц|мг\/сут|кап\/сут|мл\s*в\/м|левомепромазин|рисперидон|риперидон|хлорпромазин|перициазин|алимемазин|галоперидол|(?<![а-яё])терапи|назнач|психокоррекц|бесед[а-яё]*\s+направленн/.test(
    t,
  );
}

function classifySentence(s: string): keyof ExtractedFacts | "contrast" | "skip" {
  const lower = s.toLowerCase();
  if (isAdmissionHistory(s)) return "history";
  if (
    /выходн|понедельник|вторник|сред[ауы]|четверг|пятниц|суббот|воскресень/.test(lower) ||
    isRelativeVisitSentence(s)
  ) {
    return "timed";
  }
  if (
    /в первые дни|при поступлении|первое время/.test(lower) &&
    /в дальнейшем|затем|позднее/.test(lower)
  ) {
    return "contrast";
  }
  if (/в первые дни|при поступлении|с первых дней|первое время/.test(lower)) return "early";
  if (
    /мг\/сут|левомепромазин|рисперидон|риперидон|вальпро|бипериден|терапи|дозировк|отмен|инъекц|хлорпромазин|перициазин|неволептом|галоперидол|алимемазин|кветиапин/.test(
      lower,
    )
  ) {
    return "therapy";
  }
  if (/\d{1,2}[./]\d{1,2}/.test(s)) return "timed";
  if (/положительн[а-яё]+ динамик|стал более|снизилась частота|приблизилось к ровному/.test(lower)) {
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

const ADMISSION_BLOCK_RE =
  /(?:[^.:]{0,30}(?:статус|состояние)\s+)?при\s+поступлении\s*:/i;
const ADMISSION_BLOCK_END_RE =
  /первое время|в дальнейшем|постепенно|на фоне|в течение (?:госпитализации|пребывания)|за время/;

export function extractFacts(directorContext: string): ExtractedFacts {
  const facts: ExtractedFacts = {
    early: [],
    laterBehaviors: [],
    traits: [],
    therapy: [],
    improvement: [],
    residual: [],
    timed: [],
    history: [],
  };
  if (!directorContext.trim()) return facts;

  // «Статус при поступлении: …» — до «первое время / постепенно / в
  // дальнейшем» это исходное состояние: только первые дни, не пул периода.
  let inAdmissionBlock = false;
  for (const sentence of splitSentences(directorContext)) {
    let raw = sentence;
    const marker = ADMISSION_BLOCK_RE.exec(raw);
    if (marker) {
      inAdmissionBlock = true;
      raw = raw.slice(marker.index + marker[0].length).trim();
      if (!raw) continue;
    } else if (inAdmissionBlock && ADMISSION_BLOCK_END_RE.test(raw.toLowerCase())) {
      inAdmissionBlock = false;
    }
    if (inAdmissionBlock) {
      if (isAdmissionHistory(raw)) facts.history.push(raw.replace(/[.;]+$/, ""));
      // «Сознание ясное» — штамп статуса, не наблюдение дня.
      else if (raw.length > 24) facts.early.push(raw.replace(/[.;]+$/, ""));
      continue;
    }
    const kind = classifySentence(raw);
    if (kind === "skip") continue;
    if (kind === "contrast") {
      const parts = raw.split(/в дальнейшем|затем|позднее/i);
      const earlyPart = (parts[0] ?? "")
        .replace(/в первые дни|при поступлении|с первых дней|первое время/gi, "")
        .replace(/^[,.\s]+/, "")
        .trim();
      const laterPart = (parts.slice(1).join(" ") ?? "").replace(/^[,.\s]+/, "").trim();
      if (earlyPart) facts.early.push(earlyPart.replace(/[.;]+$/, ""));
      if (laterPart) {
        const laterBits = splitListish(laterPart).filter((b) => !isAdmissionHistory(b));
        facts.laterBehaviors.push(...laterBits);
      }
      continue;
    }
    if (kind === "therapy") {
      facts.therapy.push(raw.replace(/[.;]+$/, ""));
      const clinical = stripTherapyClauses(raw);
      if (
        clinical &&
        !isAdmissionHistory(clinical) &&
        !isTherapyLeakObservation(clinical)
      ) {
        facts.laterBehaviors.push(clinical.replace(/[.;]+$/, ""));
      }
      continue;
    }
    if (kind === "laterBehaviors") {
      // Заключение о выписке не режем на куски: оно уйдёт в день выписки целиком.
      if (isDischargeDaySentence(raw)) {
        facts.laterBehaviors.push(raw.replace(/[.;]+$/, ""));
        continue;
      }
      const bits = splitListish(raw).filter((b) => !isAdmissionHistory(b));
      if (bits.length > 1) facts.laterBehaviors.push(...bits);
      else facts.laterBehaviors.push(raw.replace(/[.;]+$/, ""));
      continue;
    }
    facts[kind].push(raw.replace(/[.;]+$/, ""));
  }

  facts.laterBehaviors = unique(
    facts.laterBehaviors.filter((s) => !isAdmissionHistory(s) && !isTherapyLeakObservation(s)),
  );
  facts.traits = unique(facts.traits);
  facts.history = unique(facts.history);
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
  return /простын|обув|хаотичн|возбужд|агресс|бьёт|бьет себя|царап|головой|растормож|суетлив|таска|обирает|стаскива|разбрасыв|фиксац/.test(
    stripNegatedAgitation(text),
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

function finalStateText(batchAnswers: Answers): string {
  const raw = batchAnswers.final_state;
  return typeof raw === "string" ? raw : "";
}

function diagnosisText(batchAnswers: Answers): string {
  const raw = batchAnswers.diagnosis;
  return typeof raw === "string" ? fixObviousTypos(raw.trim()) : "";
}

/** Возбуждение СЕГОДНЯ (крик, замах, не поддавался коррекции), не «с трудом поддаётся». */
export function observationsAgitated(observations: string[]): boolean {
  const blob = stripNegatedAgitation(observations.join(" "));
  if (!blob.trim()) return false;
  return /возбужд|агресс|не подда|замах|кричал|кричит|остро.{0,20}замечан/.test(blob);
}

/**
 * Мягкая фиксация — только если врач сам написал о ней в наблюдениях дня.
 * Мера стеснения в медицинском документе не выводится из «возбуждения».
 */
export function observationsNeedFixation(observations: string[]): boolean {
  return /фиксац/.test(observations.join(" ").toLowerCase());
}

const ID_DIAGNOSIS_RE = /F7\d|умственн[а-яё]*\s+отстал/i;

/**
 * Ориентировка и критика — константы пациента, а не переменная дня.
 * На проде они прыгали: «ориентирован верно» → «в дате затрудняется» →
 * снова «верно»; критика «формальная → достаточная → формируется».
 */
export function steadyStateLines(
  diagnosis: string,
  finalState: string,
  phase: DayPhase,
): string[] {
  const lines: string[] = [];
  if (!ID_DIAGNOSIS_RE.test(diagnosis)) {
    lines.push(
      "Ориентировка — константа всех дней: в месте, времени и собственной личности " +
        "верно. НЕ пиши «в дате затрудняется», «дату называет с ошибкой», " +
        "«ориентирован частично», если этого нет в наблюдениях СЕГОДНЯ.",
    );
  }
  const finalCriticism = /критик[а-яё]*[^.]*\./i.exec(finalState)?.[0]?.trim();
  const target =
    phase === "improving" || phase === "residual"
      ? finalCriticism ?? "критика к своему состоянию формируется"
      : "критика к своему состоянию формальная";
  lines.push(
    `Критика — обязательный элемент статуса, пиши её КАЖДЫЙ день одной ` +
      `формулировкой: «${target}». Не чередуй «формальная / частично сохранна / ` +
      "достаточная / формируется» от дня к дню.",
  );
  return lines;
}

function cognitiveLockLines(diagnosis: string): string[] {
  if (/F72|F73|тяжёл[а-яё]* умственн|выраженн[а-яё]* умственн/i.test(diagnosis)) {
    return [
      "Интеллект ЭТОГО пациента — умственная отсталость тяжёлой (выраженной) степени по диагнозу, словами полностью. Не пиши «лёгкую», «умеренную», аббревиатуру «УО», F70/F91 и «возрастную норму».",
      "КОГНИТИВНЫЙ ПРОФИЛЬ — константы ВСЕХ дней, не меняй от дня к дню:",
      "• Критика: отсутствует / не выявляется. Не чередуй со «снижена».",
      "• Эмоциональные реакции: недифференцированные (удовольствие/неудовольствие), лабильные. ЗАПРЕЩЕНО: «адекватны ситуации».",
      "• Волевой контроль: отсутствует или резко снижен. ЗАПРЕЩЕНО: «достаточен».",
      "• Мышление: наглядно-действенное / наглядно-образное. Не «абстрактное».",
    ];
  }
  if (/F71|умеренн[а-яё]* умственн/i.test(diagnosis)) {
    return [
      "Интеллект ЭТОГО пациента — умственная отсталость умеренной степени по диагнозу, словами полностью. Не пиши «лёгкую», не пиши аббревиатуру «УО», не подставляй F70/F91 и не пиши «возрастную норму».",
      "КОГНИТИВНЫЙ ПРОФИЛЬ — константы ВСЕХ дней, не меняй от дня к дню:",
      "• Критика: отсутствует / не выявляется. При умеренной умственной отсталости критики нет — не пиши «снижена» вперемешку с «отсутствует».",
      "• Эмоциональные реакции: недифференцированные (удовольствие/неудовольствие), лабильные, легко возбудим. ЗАПРЕЩЕНО: «адекватны ситуации».",
      "• Волевой контроль: резко снижен или отсутствует. ЗАПРЕЩЕНО: «достаточен».",
      "• Мышление: наглядно-действенное / наглядно-образное или сугубо конкретное. Не «соответствует возрасту».",
    ];
  }
  if (/F70|лёгк[а-яё]* умственн|легк[а-яё]* умственн/i.test(diagnosis)) {
    return [
      "Интеллект ЭТОГО пациента — умственная отсталость лёгкой степени по диагнозу, словами полностью. Не подставляй F71/F91, не пиши «возрастную норму», не пиши аббревиатуру «УО».",
    ];
  }
  if (/\bF7\d/i.test(diagnosis)) {
    return [
      "Интеллект — умственная отсталость по диагнозу (F7x), степень как в формулировке врача, словами полностью, не «УО», не «возрастная норма».",
    ];
  }
  return [
    "Интеллект ЭТОГО пациента — НЕ умственная отсталость: в диагнозе нет F70–F79. " +
      "Пиши «соответствует возрасту» / «на уровне возрастной нормы» (если бриф не задал иное). " +
      "ЗАПРЕЩЕНО копировать из образцов корпуса «умственную отсталость», «лёгкую/умеренную УО», " +
      "«снижен до уровня … отсталости» — это другой пациент.",
  ];
}

/** Уровень речи из эпикриза/диагноза — чтобы не приписывать словесные акты неговорящему. */
export function inferSpeechLevel(
  directorContext: string,
  diagnosis: string,
  finalState = "",
): SpeechLevel {
  const blob = `${directorContext} ${finalState} ${diagnosis}`.toLowerCase();
  if (
    /звукокомплекс|не говорит|безречев|невербальн|речь не сформир|отдельн[а-яё]* звук|собственная речь представлена/.test(
      blob,
    )
  ) {
    return "sounds";
  }
  if (/отдельн[а-яё]* слов|слова-предложен|лепетн/.test(blob)) return "words";
  if (/короткие фраз|простые фраз|фразовая речь не/.test(blob)) return "short_phrases";
  if (
    /развернут[а-яё]* предложен|фразовая речь сформир|речь фразов|собственн[а-яё]* речь фразов|отвечает развернуто/.test(
      blob,
    )
  ) {
    return "expanded";
  }
  if (/F72|выраженн[а-яё]* умственн/.test(diagnosis)) return "sounds";
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

/**
 * Тихий портрет: депрессивный/тревожный/астенический регистр или «тих,
 * малозаметен» без возбуждений. Шаблон «первые дни = двигательно беспокоен,
 * раздражителен» таким детям приписывал чужую клинику.
 */
export function isQuietPortrait(syndrome: unknown, directorContext: string): boolean {
  const lower = stripNegatedAgitation(directorContext);
  const agitated = /возбужд|агресс|растормож|суетлив|хаотичн|двигательно беспокоен/.test(lower);
  if (agitated) return false;
  if (syndrome === "depressive" || syndrome === "anxious" || syndrome === "asthenic") return true;
  return /(?<![а-яё])тих|малозаметн|пассивн|вял|заторможен/.test(lower);
}

function interpolateMood(
  dynamics: unknown,
  phase: DayPhase,
  quiet = false,
  syndrome: unknown = "",
): { mood: string; moodDetail: string[] } {
  if (quiet) {
    const low = syndrome === "depressive" ? "lowered" : "unstable";
    if (dynamics === "stable") return { mood: "even", moodDetail: [] };
    if (dynamics === "negative") return { mood: low, moodDetail: ["anxiety"] };
    switch (phase) {
      case "admission":
        return { mood: low, moodDetail: ["anxiety", "tearfulness"] };
      case "field":
      case "titration":
        return { mood: low, moodDetail: ["anxiety"] };
      case "improving":
      case "residual":
        return { mood: "even", moodDetail: [] };
    }
  }
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

function interpolateBehavior(phase: DayPhase, role: DayRole, quiet = false): string {
  if (quiet) return "ordered";
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
  return /телевизор|(?<![а-яё])тв(?![а-яё])|рисова|конструктор|сюжетно|ролев|в игровой|смотрел тв/.test(
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

/** Где фраза стоит в тексте врача: 0 — начало госпитализации, 1 — конец. */
function narrativePosition(item: string, directorContext: string): number {
  const at = directorContext.indexOf(item.slice(0, 40));
  if (at < 0 || directorContext.length < 2) return 0.5;
  return at / (directorContext.length - 1);
}

/**
 * Фразы из середины эпикриза — в дни, соответствующие их месту в тексте:
 * «фон настроения был снижен» из начала не должен всплывать перед выпиской.
 * Повторно выданная фраза штрафуется, чтобы дни не копировали друг друга.
 */
function pickByPosition(
  pool: string[],
  directorContext: string,
  periodPct: number,
  count: number,
  used: Map<string, number>,
): string[] {
  if (pool.length === 0 || count <= 0) return [];
  const target = periodPct / 100;
  const picked = [...pool]
    .map((item, i) => ({
      item,
      i,
      cost:
        Math.abs(narrativePosition(item, directorContext) - target) +
        (used.get(item) ?? 0) * 0.35,
    }))
    .sort((a, b) => a.cost - b.cost || a.i - b.i)
    .slice(0, count)
    .map((x) => x.item);
  for (const item of picked) used.set(item, (used.get(item) ?? 0) + 1);
  return picked;
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
  if (/фиксац/.test(t)) return "fixation";
  if (/хаотичн/.test(t)) return "chaos";
  if (/каприз|плак/.test(t)) return "cry";
  if (/раздраж|вербальн[а-яё]* коррекц/.test(t)) return "irritable";
  if (/бездеятел/.test(t)) return "idle";
  if (/возбужд|агресс/.test(t)) return "agitation";
  return t.slice(0, 32);
}

function fieldActPriority(text: string): number {
  const k = fieldActKey(text);
  if (k === "sheets" || k === "shoes" || k === "fixation") return 0;
  if (k === "agitation" || k === "cry" || k === "irritable") return 1;
  return 2;
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
  for (let j = 0; j < index; j++) {
    const c = calendarFor(days[j].isoDate);
    if ((c === "saturday" || c === "sunday") && days[j].dayNumber > 3) {
      return true;
    }
  }
  return false;
}

function fieldStopsForDay(
  observations: string[],
  includeFinalState: boolean,
  portrait: string,
): string[] {
  const obs = observations.join(" ").toLowerCase();
  const stops: string[] = [];
  // Стоп только для того, что есть в портрете ЭТОГО пациента: чужие
  // «простыни» у тихого ребёнка лишь подсказывают модели выдумку.
  if (/простын/.test(portrait) && !/простын/.test(obs)) {
    stops.push("простыни / стаскивание белья — не сегодня");
  }
  if (/обув/.test(portrait) && !/обув/.test(obs)) {
    stops.push("обувь / обирание обуви — не сегодня");
  }
  if (/двер|за руку/.test(portrait) && !includeFinalState && !/двер|за руку/.test(obs)) {
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

/** «12-13.09» — последние сб–вс на дату осмотра (для плейсхолдера [ВЫХОДНЫЕ]). */
export function weekendSpanLabel(date: Date): string {
  const sat = saturdayOf(date);
  const sun = new Date(sat);
  sun.setDate(sat.getDate() + 1);
  return formatWeekendSpan(sat, sun);
}

/**
 * Формула бланка после диагноза — на ПОНЕДЕЛЬНИКЕ после выходных.
 * Первые 3 дня госпитализации — без формулы. Суббота/воскресенье — null
 * (отдельный дневник за выходной после 3-го дня не пишется).
 */
export function weekendDutyNote(isoDate: string, dayNumber: number): string | null {
  if (dayNumber <= 3) return null;
  const date = parseIso(isoDate);
  if (!date) return null;
  if (date.getDay() !== 1) return null;
  const sat = saturdayOf(date);
  const sun = new Date(sat);
  sun.setDate(sat.getDate() + 1);
  return (
    `за период выходных дней с ${formatWeekendSpan(sat, sun)} ` +
    "под наблюдением дежурного мед персонала."
  );
}

function isTherapyText(text: string): boolean {
  return (
    isTherapyLeakObservation(text) ||
    /\d\s*(?:мг|мл)(?![а-яё])|кап\/|таб\.|р-р|инъекц|терапи|дозиров|препарат|антидепрессант/i.test(text)
  );
}

/** «НЕ повторяй»: узнаваемое начало эпизода, не его полный текст. */
function forbidFragment(text: string): string {
  const clean = text
    .replace(/^За период \(не новый эпизод сегодня\):\s*/, "")
    .replace(/[.;]+$/, "")
    .trim();
  const words = clean.split(/\s+/);
  let out = words.slice(0, 8).join(" ");
  if (out.length > 70) out = out.slice(0, 70).replace(/\s+\S*$/, "");
  return out.length < clean.length ? `${out}…` : out;
}

/** Убрать из текста препараты/инъекции — оставить наблюдаемое поведение. */
export function stripTherapyClauses(text: string): string {
  let s = text
    .replace(/\b\d{1,2}[./]\d{1,2}(?:[./]\d{2,4})?\.?/g, " ")
    .replace(
      /(?:р-?ра?\.?|таб\.?|раствор)\s[^.]{0,90}?(?:мг\/сут|кап\/сут|мл(?:\s*в\/м)?)/gi,
      " ",
    )
    .replace(/с седативной целью[^.]*?(?:\.|$)/gi, " ")
    .replace(/в связи с (?:этим|возбуждением)[^.]*инъекц[^.]*?(?:\.|$)/gi, " ")
    .replace(
      /(?:дежурным врачом\s+)?(?:была |было )?(?:назначена |выполнена |сделана )?инъекц[^.]*?(?:\.|$)/gi,
      " ",
    )
    .replace(/дозировка[^.]*(?:\.|$)/gi, " ")
    .replace(/под контролем\s*АД,?\s*ЧСС/gi, " ")
    .replace(/\s+/g, " ")
    .replace(/^[,.\s]+|[.,\s]+$/g, "")
    .trim();
  if (s.length < 28) return "";
  if (s && !/[.!?]$/.test(s)) s += ".";
  return s;
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

  const consumed = new Set<string>();
  for (const para of splitParagraphs(directorContext)) {
    for (let i = 0; i < para.length; i++) {
      const raw = para[i];
      if (consumed.has(normSentence(raw))) continue;
      const kind = classifySentence(raw);
      if (kind === "therapy" || kind === "history" || kind === "skip") continue;
      let s = raw.replace(/[.;]+$/, "");
      if (isAdmissionHistory(s) || isTherapyLeakObservation(s)) continue;
      const lower = s.toLowerCase();
      const dated = extractDatedSnippets(s, packetYear)
        .map((d) => d.iso)
        .filter((iso) => inPacket.has(iso));
      // Недатированная выписка уходит в день выписки (compileArc).
      if (dated.length === 0 && isDischargeDaySentence(s)) continue;
      if (dated.length > 0) {
        // Эпизод целиком: «пришла мать» + требование выписки + беседа.
        const cont = episodeContinuation(para, i, packetYear);
        for (const c of cont) consumed.add(normSentence(c));
        if (cont.length > 0) s = [s, ...cont].join(". ");
      }
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
  }
  return out;
}

function normSentence(s: string): string {
  return s.replace(/[.;]+$/, "").trim().toLowerCase();
}

// Продолжение эпизода ссылается на его участников/ход: «мать и пациент…»,
// «с мамой проведена беседа», «решения не поменяли». Фон периода («фон
// настроения был снижен») к эпизоду не относится.
const EPISODE_LINK_RE =
  /мам|мат(?:ь|ер)|отец|отц|пап|родител|родствен|(?<![а-яё])они(?![а-яё])|(?<![а-яё])их(?![а-яё])|(?<![а-яё])им(?![а-яё])|решени|беседа|выписк|требовани|после (?:этого|визита|встречи|ухода|беседы)|в ответ|(?<![а-яё])затем(?![а-яё])/;

/** Следующие предложения абзаца, продолжающие эпизод с датой. */
function episodeContinuation(para: string[], i: number, packetYear: number): string[] {
  const out: string[] = [];
  for (let j = i + 1; j < para.length && out.length < 4; j++) {
    const t = para[j];
    const lower = t.toLowerCase();
    if (!EPISODE_LINK_RE.test(lower)) break;
    if (extractDatedSnippets(t, packetYear).length > 0) break;
    if (WEEKDAY_RES.some((x) => x.re.test(lower))) break;
    if (/в первые дни|при поступлении|в дальнейшем|в течение госпитализ|на фоне|в целом/.test(lower)) break;
    if (isAdmissionHistory(t) || isTherapyLeakObservation(t) || classifySentence(t) === "therapy") {
      break;
    }
    out.push(t.replace(/[.;]+$/, ""));
  }
  return out;
}

/** Предложения, ушедшие в эпизоды с датой: в общие пулы дней их не кладём. */
export function datedEpisodeSentences(
  directorContext: string,
  days: ArcDayPlan[],
): Set<string> {
  const out = new Set<string>();
  if (!directorContext.trim() || days.length === 0) return out;
  const packetYear = Number(days[0].isoDate.slice(0, 4)) || new Date().getFullYear();
  const inPacket = new Set(days.map((d) => d.isoDate));
  for (const para of splitParagraphs(directorContext)) {
    para.forEach((s, i) => {
      const dated = extractDatedSnippets(s, packetYear).some((d) => inPacket.has(d.iso));
      if (!dated) return;
      out.add(normSentence(s));
      for (const c of episodeContinuation(para, i, packetYear)) out.add(normSentence(c));
    });
  }
  return out;
}

function recapHostIndex(briefs: DayBrief[], weekendIndex: number): number {
  for (let i = weekendIndex + 1; i < briefs.length; i++) {
    if (briefs[i].calendar === "monday") return i;
  }
  for (let i = weekendIndex - 1; i >= 0; i--) {
    if (briefs[i].calendar !== "saturday" && briefs[i].calendar !== "sunday") {
      return i;
    }
  }
  return -1;
}

function dutyFormulaFromWeekend(weekendIso: string): string | null {
  const date = parseIso(weekendIso);
  if (!date) return null;
  const sat = saturdayOf(date);
  const sun = new Date(sat);
  sun.setDate(sat.getDate() + 1);
  return (
    `за период выходных дней с ${formatWeekendSpan(sat, sun)} ` +
    "под наблюдением дежурного мед персонала."
  );
}

function attachWeekendRecaps(briefs: DayBrief[]): void {
  for (let i = 0; i < briefs.length; i++) {
    const b = briefs[i];
    if (b.dayNumber <= 3) continue;
    if (b.calendar !== "saturday" && b.calendar !== "sunday") continue;
    const host = recapHostIndex(briefs, i);
    if (host < 0) continue;
    const target = briefs[host];
    target.weekendRecap = true;
    const formula =
      weekendDutyNote(target.isoDate, target.dayNumber) ??
      dutyFormulaFromWeekend(b.isoDate);
    if (!formula) continue;
    const extras = b.observations.filter(
      (o) =>
        o.length > 24 &&
        !/^За период/.test(o) &&
        !isAdmissionHistory(o) &&
        !isTherapyLeakObservation(o),
    );
    if (!target.weekendDutyNote) target.weekendDutyNote = formula;
    for (const o of extras.slice(0, 2)) {
      if (!target.weekendRecapNotes.some((x) => x.slice(0, 24) === o.slice(0, 24))) {
        target.weekendRecapNotes.push(o);
      }
    }
  }
  for (const b of briefs) {
    if (b.calendar === "saturday" || b.calendar === "sunday") {
      b.weekendDutyNote = null;
      b.weekendRecapNotes = [];
    }
  }
}

/** Собрать брифы на каждый день пакета. */
export function compileArc(input: CompileArcInput): DayBrief[] {
  const { days, directorContext, batchAnswers } = input;
  const n = days.length;
  const facts = extractFacts(directorContext);
  // Эпизоды с датой и выписка живут только в своём дне — не в общих пулах.
  const episodeParts = datedEpisodeSentences(directorContext, days);
  // «на момент осмотра» в статусе при поступлении — это поступление, не выписка.
  const admissionParts = new Set([...facts.early, ...facts.history].map(normSentence));
  // Последний абзац врача с выводом о выписке («может быть выписан…»)
  // целиком относится ко дню выписки, вместе со статусом «на момент осмотра».
  const paragraphs = splitParagraphs(directorContext);
  const lastParagraph = paragraphs.at(-1) ?? [];
  const firstDischarge = lastParagraph.findIndex(isDischargeDaySentence);
  const closingParagraph =
    firstDischarge < 0
      ? []
      : // Отдельный абзац-вывод — целиком; сплошной текст — только хвост
        // начиная с фразы о выписке, иначе туда уедет весь эпикриз.
        lastParagraph.slice(paragraphs.length > 1 ? 0 : firstDischarge);
  const closingParts = new Set(closingParagraph.map(normSentence));
  const isDischargeDayNote = (s: string): boolean =>
    (isDischargeDaySentence(s) || closingParts.has(normSentence(s))) &&
    !admissionParts.has(normSentence(s));
  const dischargeNotes = unique(
    [...splitSentences(directorContext), ...closingParagraph]
      .filter((s) => isDischargeDayNote(s) && !episodeParts.has(normSentence(s)))
      .map((s) => s.replace(/[.;]+$/, "")),
  );
  const ownDaySentence = (item: string): boolean => {
    const k = normSentence(item);
    if (!k) return false;
    if (isDischargeDayNote(item)) return true;
    for (const part of closingParts) if (part.includes(k)) return true;
    for (const part of episodeParts) if (part.includes(k)) return true;
    return false;
  };
  for (const key of ["laterBehaviors", "traits", "improvement", "residual", "timed"] as const) {
    facts[key] = facts[key].filter((x) => !ownDaySentence(x));
  }
  const dischargeIso =
    input.estimatedDischargeDate && days.some((d) => d.isoDate === input.estimatedDischargeDate)
      ? input.estimatedDischargeDate
      : input.estimatedDischargeDate
        ? null
        : (days[n - 1]?.isoDate ?? null);
  const quiet = isQuietPortrait(batchAnswers.leading_syndrome, directorContext);
  const dynamics = batchAnswers.overall_dynamics;
  const diagnosis = diagnosisText(batchAnswers);
  const finalState =
    typeof batchAnswers.final_state === "string" ? batchAnswers.final_state : "";
  const speechLevel = inferSpeechLevel(directorContext, diagnosis, finalState);
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
    [...facts.timed, ...splitSentences(directorContext)].filter(
      (s) =>
        isRelativeVisitSentence(s) && !isAdmissionHistory(s) && !ownDaySentence(s),
    ),
  );

  const usedFieldKeys = new Set<string>();
  const fieldPoolAll = facts.laterBehaviors
    .filter(isFieldAct)
    .sort((a, b) => fieldActPriority(a) - fieldActPriority(b));
  const calmPoolAll = facts.laterBehaviors.filter((s) => !isFieldAct(s) && !isOccupation(s));
  const calmUse = new Map<string, number>();
  const pickCalm = (pct: number, count: number) =>
    pickByPosition(calmPoolAll, directorContext, pct, count, calmUse);

  let lastOccupation = "";
  const briefs: DayBrief[] = days.map((day, index) => {
    const periodPct = n <= 1 ? 100 : Math.round((index / (n - 1)) * 100);
    const phase = phaseFor(periodPct, day.dayNumber);
    const role = roleFor(day, index, n);
    const { mood, moodDetail } = interpolateMood(
      dynamics,
      phase,
      quiet,
      batchAnswers.leading_syndrome,
    );
    const behavior = interpolateBehavior(phase, role, quiet);
    const contact = interpolateContact(facts, role, phase, speechLevel);
    const includeFinalState = index >= n - 2 || (role === "exam" && periodPct >= 80);
    const medsToday = datedMeds.filter((m) => m.iso === day.isoDate).map((m) => m.snippet);
    let therapyToday: string | null = null;
    if (medsToday.length > 0) {
      therapyToday = medsToday.join(" ");
    }

    const obsCount = role === "quiet" ? 1 : role === "exam" ? 3 : 2;
    const canTakeField =
      role === "event" &&
      (phase === "admission" || phase === "field" || phase === "titration");
    const timedToday = [
      ...(timedByDay.get(day.isoDate) ?? []),
      ...(day.isoDate === dischargeIso ? dischargeNotes : []),
    ];
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
      observations.push(...pickCalm(periodPct, 1));
    } else if (phase === "admission") {
      observations.push(...pickRotated(facts.early, index, 1));
      if (canTakeField) observations.push(...takeUnused(fieldPoolAll, usedFieldKeys, 1));
    } else if (phase === "field") {
      if (canTakeField) observations.push(...takeUnused(fieldPoolAll, usedFieldKeys, 1));
      else observations.push(...pickCalm(periodPct, 1));
    } else if (phase === "titration") {
      if (canTakeField) observations.push(...takeUnused(fieldPoolAll, usedFieldKeys, 1));
      observations.push(...pickCalm(periodPct, Math.max(1, obsCount - 1)));
    } else {
      observations.push(...pickCalm(periodPct, obsCount));
    }
    if (phase !== "admission") {
      // Одно и то же занятие («в игровой рисовал, читал») в соседних днях
      // читается как копипаст — через день пропускаем.
      const occupation = pickRotated(occupationPool, index, 1);
      if (occupation[0] && occupation[0] !== lastOccupation) {
        observations.push(...occupation);
        lastOccupation = occupation[0];
      } else {
        lastOccupation = "";
      }
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
    const dayObservations = unique([...reserved, ...rest.slice(0, restCap)]).filter(
      (o) => !isAdmissionHistory(o) && !isTherapyLeakObservation(o),
    );
    if (therapyToday) {
      const clinical = stripTherapyClauses(therapyToday);
      if (clinical && !isAdmissionHistory(clinical) && !isTherapyLeakObservation(clinical)) {
        dayObservations.unshift(clinical);
      }
    }
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
      observations: unique(dayObservations),
      forbidden: [],
      therapyToday,
      historyNotes: [],
      includeFinalState,
      lengthHint: role === "exam" ? "exam" : role === "quiet" ? "short" : "medium",
      weekendRecap: priorWeekendInPacket(days, index),
      weekendDutyNote: priorWeekendInPacket(days, index)
        ? weekendDutyNote(day.isoDate, day.dayNumber)
        : null,
      weekendRecapNotes: [],
    };
  });

  attachWeekendRecaps(briefs);

  const portraitBlob = `${directorContext} ${finalState}`.toLowerCase();
  const hasDischargeEvent =
    dischargeNotes.some((p) => isDischargeSentence(p)) ||
    [...episodeParts].some((p) => isDischargeSentence(p));
  for (let i = 0; i < briefs.length; i++) {
    const b = briefs[i];
    const others = briefs.filter((_, j) => j !== i).flatMap((x) => x.observations);
    const today = b.observations.join(" ");
    const labels: string[] = [];
    const timedHere = new Set(timedByDay.get(b.isoDate) ?? []);
    if (
      b.role !== "exam" &&
      visitPool.some((v) => !timedHere.has(v)) &&
      !isRelativeVisitSentence(today)
    ) {
      labels.push("визиты родственников — не сегодня");
    }
    if (hasDischargeEvent && b.isoDate !== dischargeIso) {
      labels.push("требование выписки / беседа о выписке — не сегодня");
    }
    const extra: string[] = [];
    if (b.phase === "admission" || b.phase === "field") {
      extra.push(...facts.improvement, ...facts.residual);
    }
    if (b.phase === "improving" || b.phase === "residual") {
      extra.push(...facts.laterBehaviors.filter(isFieldAct), ...facts.early);
    }
    // Только короткие ярлыки: полный текст чужих эпизодов и схемы терапии
    // в «НЕ повторяй» модель как раз и переносила в сегодняшний дневник.
    // Терапию и целевое состояние запрещают отдельные строки брифа.
    const fragments = unique([...extra, ...others])
      .filter(
        (t) =>
          !isTherapyText(t) &&
          !isAdmissionHistory(t) &&
          !isDischargeDaySentence(t) &&
          !isRelativeVisitSentence(t) &&
          !today.includes(t),
      )
      .map(forbidFragment);
    const stops = fieldStopsForDay(b.observations, b.includeFinalState, portraitBlob);
    b.forbidden = unique([...stops, ...labels, ...fragments]).slice(0, 8);
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
    if (brief.dayNumber <= 3) {
      lines.push(
        "Выходной, но это один из первых трёх дней госпитализации: пиши обычный ежедневный осмотр. " +
          "«Дополнительные сведения о заболевании» оставь «нет». " +
          "НЕ пиши формулу «за период выходных дней» и «под наблюдением дежурного мед персонала».",
      );
    } else {
      lines.push(
        "Выходной после 3-го дня госпитализации: отдельный дневник за сегодня НЕ пишется. " +
          "Если этот текст всё же генерируется — сделай его максимально коротким и без формулы дежурного персонала; " +
          "пересказ выходных уйдёт в понедельник.",
      );
    }
  } else if (brief.weekendRecap || brief.weekendDutyNote) {
    const formula = brief.weekendDutyNote
      ? `«${brief.weekendDutyNote}»`
      : "формулу из брифа про дежурный персонал";
    lines.push(
      "Понедельник после пропущенных сб/вс ЭТОГО пакета: отдельный дневник за субботу и воскресенье не пишется. " +
        `После диагноза в «Дополнительные сведения о заболевании» напиши ${formula} ` +
        "— формула про дежурный персонал и кратко (1–2 предложения) что было за выходные, если это есть в наблюдениях. " +
        "Эту формулу не ставь в психический статус и не выдумывай прогулки и инциденты.",
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
    lines.push(...cognitiveLockLines(diagnosis));
  } else {
    lines.push(
      "Код МКБ не дан — в диагнозе плейсхолдер [ОСНОВНОЙ_ДИАГНОЗ], не выдумывай F-код. " +
        "Интеллект и речь — только из ответов/брифа ЭТОГО пациента, не из образцов корпуса.",
    );
  }
  lines.push(
    ...steadyStateLines(diagnosis, finalStateText(batchAnswers), brief.phase),
  );
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
    const injection = /инъекц/i.test(brief.therapyToday);
    const fixated = observationsNeedFixation(brief.observations);
    lines.push(
      `Сегодня коррекция/инъекция — пиши ТОЛЬКО в «План лечения (дополнения к плану)». ` +
        `«Назначения» всегда: «см. лист назначений». ` +
        `В психический статус — эпизод поведения БЕЗ препаратов, доз и инъекций. ` +
        `Текст для плана лечения: ${brief.therapyToday}`,
    );
    if (injection && fixated) {
      lines.push(
        "Инъекция на сегодня — только если после мягкой фиксации НЕ успокоился (или успокоился непродолжительно). Если после фиксации успокоился — «План лечения: без дополнений», инъекцию не пиши.",
      );
    }
  } else {
    lines.push(
      "Терапию и дозировки сегодня НЕ перечисляй — «Назначения: см. лист назначений», «План лечения: без дополнений». Не переноси смены схемы с других дат. Не копируй препараты из образцов корпуса (перициазин, хлорпромазин и любые другие, которых нет в брифе).",
    );
  }
  if (observationsNeedFixation(brief.observations)) {
    lines.push(
      "ЗАПРЕЩЕНО: «физическое удержание», «удержание персоналом», «при попытке удержания», иммобилизация. " +
        "Врач сегодня указал мягкую фиксацию — в статусе каскад: поведение → замечания/вербальная коррекция → «в связи с этим применена мягкая фиксация конечностей под контролем медперсонала» (срок — только если он есть в наблюдениях) → после фиксации успокоился или не успокоился. " +
        "Если не успокоился и в брифе сегодня есть инъекция — это коррекция в «Плане лечения», препарат только из брифа. " +
        "«с помощью персонала» — только одевание, мытьё, еда.",
    );
  } else if (observationsAgitated(brief.observations)) {
    lines.push(
      "ЗАПРЕЩЕНО: «физическое удержание», «при попытке удержания». Сегодня возбуждение — опиши поведение и реакцию на замечания / вербальную коррекцию. " +
        "Мягкую фиксацию и другие меры стеснения не выдумывай: врач их сегодня не указал, а в медицинском документе они только со слов врача. «с помощью персонала» — только одевание, мытьё, еда.",
    );
  } else {
    lines.push(
      "ЗАПРЕЩЕНО: «физическое удержание», «при попытке удержания». Сегодня мягкую фиксацию не выдумывай — врач её не указал, даже если она есть в образцах корпуса. «с помощью персонала» — только одевание, мытьё, еда.",
    );
  }
  lines.push(
    "Интернат, ДДИ, направление на госпитализацию, «мать забрала до поступления», приёмный покой, сантранспорт, «после выписки» — не событие этого дня и не дополнение анамнеза этого бланка. Ребёнок сейчас в стационаре. Анамнез жизни и заболевания: «без дополнений». Не пиши «сегодня выдан пакет документов» и «сегодня мать забрала из интерната». Не оставляй [УЧРЕЖДЕНИЕ] / [НОМЕР_ДОКУМЕНТА].",
  );
  if (brief.role === "exam") {
    lines.push(
      "Осмотр 10 дней: «Анамнез жизни» и «Анамнез заболевания» оставь «без дополнений». Не копируй интернат/ДДИ/направление даже «для полноты».",
    );
  }
  if (brief.weekendRecapNotes.length > 0) {
    lines.push(
      "За выходные (ТОЛЬКО строка «Дополнительные сведения о заболевании», не статус сегодня и не анамнез). Перескажи 1–2 предложениями, не копируй сырые заметки:",
    );
    for (const n of brief.weekendRecapNotes) lines.push(`• ${n}`);
  }
  if (brief.includeFinalState) {
    const fs = batchAnswers.final_state;
    if (typeof fs === "string" && fs.trim()) {
      const usable = splitSentences(fs).filter((s) => !isAdmissionHistory(s));
      if (usable.length > 0) {
        lines.push(
          `Целевое/текущее состояние (можно опереться, не копировать дословно): ${usable.join(" ")}`,
        );
      }
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
  } else {
    out.additional_info = "none";
  }
  out.anamnesis_disease = "no_additions";
  out.anamnesis_life = "no_additions";
  out.prescriptions = "see_list";
  if (brief.therapyToday) {
    out.treatment_plan = "adjusted";
    out.treatment_plan_detail = brief.therapyToday;
  } else {
    out.treatment_plan = "no_change";
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
