// Лёгкая разметка постов новостей: заголовки, списки, **жирный**, `код`.
// Без HTML — только безопасные текстовые узлы для React.

export type NewsInline =
  | { kind: "text"; text: string }
  | { kind: "bold"; text: string }
  | { kind: "code"; text: string };

export type NewsBlock =
  | { type: "h2"; text: string }
  | { type: "h3"; text: string }
  | { type: "p"; inlines: NewsInline[] }
  | { type: "ul"; items: NewsInline[][] };

const INLINE_RE = /(\*\*[^*]+?\*\*|`[^`]+`)/g;

export function parseNewsInlines(raw: string): NewsInline[] {
  const out: NewsInline[] = [];
  let last = 0;
  const re = new RegExp(INLINE_RE.source, "g");
  for (const m of raw.matchAll(re)) {
    const idx = m.index ?? 0;
    if (idx > last) out.push({ kind: "text", text: raw.slice(last, idx) });
    const token = m[0];
    if (token.startsWith("**")) {
      out.push({ kind: "bold", text: token.slice(2, -2) });
    } else {
      out.push({ kind: "code", text: token.slice(1, -1) });
    }
    last = idx + token.length;
  }
  if (last < raw.length) out.push({ kind: "text", text: raw.slice(last) });
  return out.length > 0 ? out : [{ kind: "text", text: raw }];
}

function headingText(line: string, prefix: string): string {
  return line.slice(prefix.length).trim();
}

function isListLine(line: string): boolean {
  return /^[-*]\s+\S/.test(line);
}

function listItemText(line: string): string {
  return line.replace(/^[-*]\s+/, "").trim();
}

/** Разбирает тело поста в блоки для безопасного рендера. */
export function parseNewsMarkup(source: string): NewsBlock[] {
  const lines = source.replace(/\r\n/g, "\n").split("\n");
  const blocks: NewsBlock[] = [];
  let para: string[] = [];
  let list: string[] = [];

  const flushPara = () => {
    if (para.length === 0) return;
    const text = para.join(" ").trim();
    para = [];
    if (text) blocks.push({ type: "p", inlines: parseNewsInlines(text) });
  };
  const flushList = () => {
    if (list.length === 0) return;
    blocks.push({
      type: "ul",
      items: list.map((item) => parseNewsInlines(item)),
    });
    list = [];
  };

  for (const raw of lines) {
    const line = raw.trimEnd();
    const trimmed = line.trim();
    if (!trimmed) {
      flushPara();
      flushList();
      continue;
    }
    if (trimmed.startsWith("### ")) {
      flushPara();
      flushList();
      blocks.push({ type: "h3", text: headingText(trimmed, "### ") });
      continue;
    }
    if (trimmed.startsWith("## ")) {
      flushPara();
      flushList();
      blocks.push({ type: "h2", text: headingText(trimmed, "## ") });
      continue;
    }
    if (isListLine(trimmed)) {
      flushPara();
      list.push(listItemText(trimmed));
      continue;
    }
    flushList();
    para.push(trimmed);
  }
  flushPara();
  flushList();
  return blocks;
}

export function newsTypeLabel(type: string): string {
  return type === "news" ? "Новость" : "Релиз";
}

export function isRecentNews(iso?: string, days = 14): boolean {
  if (!iso) return false;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return false;
  return Date.now() - d.getTime() < days * 24 * 60 * 60 * 1000;
}
