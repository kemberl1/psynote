// Разбор текста дневника для карточки: плейсхолдеры [ДАТА], жирный **текст**
// и метки разделов шаблона (Жалобы:, Психический статус:).

export type DiaryRun =
  | { kind: "text"; text: string }
  | { kind: "bold"; text: string }
  | { kind: "placeholder"; text: string };

export const PLACEHOLDER_SPLIT_RE = /(\[[A-ZА-ЯЁ0-9_]+\])/g;
export const PLACEHOLDER_TEST_RE = /^\[[A-ZА-ЯЁ0-9_]+\]$/;

const SECTION_LABELS = [
  "Анамнез заболевания (дополнения к анамнезу)",
  "Анамнез жизни (дополнения к анамнезу)",
  "Физикальное исследование, локальный статус (его изменение)",
  "Физикальное исследование",
  "Психический статус (его изменение)",
  "Психический статус",
  "Соматический статус",
  "Неврологический статус (его изменение)",
  "Неврологический статус",
  "Осложнение основного заболевания",
  "Сопутствующие заболевания",
  "Основное заболевание",
  "Дополнительные сведения о заболевании",
  "Дополнительные сведения",
  "Обоснование диагноза (при наличии дополнительных сведений)",
  "Выполнены медицинские вмешательства",
  "План обследования (дополнения к плану)",
  "План обследования",
  "План лечения (дополнения к плану)",
  "План лечения",
  "Этапный эпикриз",
  "Заведующий отделением",
  "Лечащий врач",
  "Назначения",
  "Диагноз",
  "Жалобы",
  "Синдром",
].sort((a, b) => b.length - a.length);

const HEADER_LINES = [
  "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ",
  "ОСМОТР лечащим врачом совместно с заведующим отделением",
  "ОСМОТР",
  "лечащим врачом совместно с заведующим отделением",
  "Фамилия, имя, отчество (при наличии) врача, должность, специальность, подпись",
  "Фамилия, имя, отчество (при наличии) заведующего отделением, подпись",
];

function stripWrapStars(s: string): string {
  return s.replace(/^\*\*/, "").replace(/\*\*$/, "");
}

function isHeaderLine(line: string): boolean {
  const plain = stripWrapStars(line.trim());
  const upper = plain.toUpperCase();
  return HEADER_LINES.some((h) => h.toUpperCase() === upper);
}

function matchSectionLabel(line: string): { label: string; rest: string } | null {
  const trimmed = line.replace(/^\s+/, "");
  for (const label of SECTION_LABELS) {
    const re = new RegExp(
      `^(?:\\*\\*)?${escapeRegExp(label)}(?:\\*\\*)?\\s*:\\s*(?:\\*\\*)?`,
      "i",
    );
    const m = trimmed.match(re);
    if (m) {
      return { label, rest: trimmed.slice(m[0].length) };
    }
  }
  return null;
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

const SKIP_EXACT = new Set(["данных нет"]);

function normalizeSkipValue(v: string): string {
  return v
    .toLowerCase()
    .replace(/[.,;:!?«»"'()]/g, "")
    .replace(/[—–]/g, "-")
    .replace(/\s+/g, " ")
    .trim();
}

function isSkipValue(v: string): boolean {
  const n = normalizeSkipValue(v);
  return n === "" || SKIP_EXACT.has(n);
}

function shouldDropSection(_label: string, value: string): boolean {
  return normalizeSkipValue(value) === "данных нет";
}

/**
 * Убрать мусор генерации («Данных нет»). Строки бланка МИС
 * («не предъявляет», «без дополнений») оставляем.
 */
export function omitEmptyDiarySections(content: string): string {
  const lines = content.replace(/\r\n/g, "\n").replace(/\r/g, "\n").split("\n");
  const out: string[] = [];
  let secLabel = "";
  let secHead = "";
  let secRest = "";
  let secBody: string[] = [];
  let hasSec = false;

  const flush = () => {
    if (!hasSec) return;
    const body = secBody.filter((raw) => {
      const trim = raw.trim();
      if (!trim) return true;
      if (isSkipValue(trim) && !matchSectionLabel(trim)) return false;
      return true;
    });
    while (body.length && !body[body.length - 1].trim()) body.pop();
    const value = [secRest, ...body.map((l) => l.trim()).filter(Boolean)].join(" ");
    if (!shouldDropSection(secLabel, value)) {
      out.push(secHead);
      out.push(...body);
    }
    hasSec = false;
    secBody = [];
  };

  for (const raw of lines) {
    const line = stripWrapStars(raw).replace(/\*\*/g, "").trimEnd();
    const matched = matchSectionLabel(line.trim());
    if (matched) {
      flush();
      hasSec = true;
      secLabel = matched.label;
      secRest = matched.rest;
      secHead = line.trim();
      secBody = [];
      continue;
    }
    if (hasSec) {
      secBody.push(line.trim() === "" ? "" : line);
      continue;
    }
    const trim = line.trim();
    if (!trim) {
      if (out.length && out[out.length - 1] !== "") out.push("");
      continue;
    }
    if (isSkipValue(trim) && !matchSectionLabel(trim)) continue;
    out.push(line);
  }
  flush();
  while (out.length && !out[0]) out.shift();
  while (out.length && !out[out.length - 1]) out.pop();
  return out.join("\n");
}

const DAILY_EXAM_TITLE = "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ";

function isDailyExam(content: string): boolean {
  return content.toUpperCase().includes(DAILY_EXAM_TITLE);
}

function isSectionStart(line: string): boolean {
  return matchSectionLabel(line.trim()) !== null;
}

function nextNonEmpty(lines: string[], from: number): string | undefined {
  for (let i = from; i < lines.length; i++) {
    if (lines[i].trim()) return lines[i];
  }
  return undefined;
}

function isDailyBlankAfter(line: string): boolean {
  const matched = matchSectionLabel(line.trim());
  if (!matched) return false;
  return (
    matched.label === "Анамнез жизни (дополнения к анамнезу)" ||
    matched.label === "Неврологический статус"
  );
}

/**
 * В ежедневнике — ровно одна пустая строка после анамнеза жизни
 * и после неврологического статуса. Между остальными разделами
 * лишние пустые строки убираем. Осмотр за 10 дней не трогаем.
 */
export function normalizeDailySpacing(content: string): string {
  if (!isDailyExam(content)) return content;
  const lines = content.replace(/\r\n/g, "\n").replace(/\r/g, "\n").split("\n");
  const compacted: string[] = [];
  for (let i = 0; i < lines.length; i++) {
    if (lines[i].trim() !== "") {
      compacted.push(lines[i]);
      continue;
    }
    const prev = compacted.length ? compacted[compacted.length - 1] : "";
    const next = nextNonEmpty(lines, i + 1);
    if (prev && next && isSectionStart(prev) && isSectionStart(next)) {
      continue;
    }
    if (compacted.length && compacted[compacted.length - 1] === "") continue;
    compacted.push("");
  }

  const out: string[] = [];
  for (let i = 0; i < compacted.length; i++) {
    out.push(compacted[i]);
    if (!isDailyBlankAfter(compacted[i])) continue;
    const next = compacted[i + 1];
    if (next !== undefined && next.trim() === "") continue;
    out.push("");
  }
  return out.join("\n");
}

function parseInlineBold(text: string): DiaryRun[] {
  if (!text) return [];
  const runs: DiaryRun[] = [];
  const re = /\*\*(.+?)\*\*/g;
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) {
      runs.push({ kind: "text", text: text.slice(last, m.index) });
    }
    runs.push({ kind: "bold", text: m[1] });
    last = m.index + m[0].length;
  }
  if (last < text.length) {
    runs.push({ kind: "text", text: text.slice(last) });
  }
  return runs;
}

function parseLine(line: string): DiaryRun[] {
  if (!line) return [];
  if (isHeaderLine(line)) {
    return [{ kind: "bold", text: stripWrapStars(line.trim()) }];
  }
  const section = matchSectionLabel(line);
  if (section) {
    const leadingWs = line.match(/^\s*/)?.[0] ?? "";
    const runs: DiaryRun[] = [];
    if (leadingWs) runs.push({ kind: "text", text: leadingWs });
    runs.push({ kind: "bold", text: `${section.label}:` });
    if (section.rest) {
      const value = section.rest.replace(/\*\*/g, "");
      if (value) runs.push({ kind: "text", text: ` ${value}` });
    }
    return runs;
  }
  return parseInlineBold(line);
}

function parseTextBlock(text: string): DiaryRun[] {
  const runs: DiaryRun[] = [];
  const parts = text.split(/(\n)/);
  for (const part of parts) {
    if (part === "\n") {
      runs.push({ kind: "text", text: "\n" });
      continue;
    }
    runs.push(...parseLine(part));
  }
  return runs;
}

/** Разобрать дневник на текстовые/жирные/плейсхолдерные фрагменты. */
export function parseDiaryMarkup(content: string): DiaryRun[] {
  const parts = content.split(PLACEHOLDER_SPLIT_RE);
  const runs: DiaryRun[] = [];
  for (const part of parts) {
    if (!part) continue;
    if (PLACEHOLDER_TEST_RE.test(part)) {
      runs.push({ kind: "placeholder", text: part });
    } else {
      runs.push(...parseTextBlock(part));
    }
  }
  return mergeAdjacent(runs);
}

function mergeAdjacent(runs: DiaryRun[]): DiaryRun[] {
  const out: DiaryRun[] = [];
  for (const run of runs) {
    const prev = out[out.length - 1];
    if (prev && prev.kind === run.kind && run.kind !== "placeholder") {
      prev.text += run.text;
    } else {
      out.push({ ...run });
    }
  }
  return out;
}
