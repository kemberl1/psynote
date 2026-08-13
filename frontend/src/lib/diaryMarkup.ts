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
  "Дополнительные сведения",
  "Выполнены медицинские вмешательства",
  "План обследования (дополнения к плану)",
  "План обследования",
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

const SKIP_EXACT = new Set([
  "данных нет",
  "без дополнений",
  "не предъявляет",
  "жалоб не предъявляет",
  "нет",
  "-",
  "—",
  "без изменений",
  "без отрицательной динамики",
  "без особенностей",
]);

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

function isDiagnosisKeepLabel(label: string): boolean {
  const l = label.toLowerCase();
  return l === "диагноз" || l.includes("заболеван") || l === "синдром" || l.includes("осложнен");
}

function shouldDropSection(label: string, value: string): boolean {
  const l = label.toLowerCase();
  const v = value.toLowerCase();
  const n = normalizeSkipValue(value);
  if (l === "диагноз" && n === "") return false;
  if (isSkipValue(value)) {
    if (isDiagnosisKeepLabel(label) && (n === "не выявлено" || n === "-")) return false;
    if (l === "диагноз") return false;
    return true;
  }
  if (l.includes("жалоб") && (v.includes("не предъявля") || v.includes("жалоб нет"))) return true;
  if (l.includes("анамнез") && v.includes("без дополнен")) return true;
  if (
    l.includes("план обследования") &&
    (v.includes("без изменен") || v.includes("без дополнен"))
  ) {
    return true;
  }
  if (
    l.includes("физикальное") &&
    (v.includes("без изменен") || v.includes("без отрицательной динамики"))
  ) {
    return true;
  }
  if (
    l.includes("соматический") &&
    (v.includes("без изменен") || v.includes("без особенностей"))
  ) {
    return true;
  }
  if (
    l.includes("вмешательства") &&
    v.includes("осмотр лечащим врачом") &&
    n.length < 40
  ) {
    return true;
  }
  return false;
}

/**
 * Убрать пустые разделы («Без изменений», «не предъявляет», «без дополнений»)
 * по правилам оформления из docs/03 и промпта. Тот же текст уходит в Word.
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
      runs.push({ kind: "text", text: " " });
      runs.push(...parseInlineBold(section.rest));
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
