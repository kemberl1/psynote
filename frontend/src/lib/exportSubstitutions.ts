import { normalizeDailySpacing, omitEmptyDiarySections } from "./diaryMarkup";
import { fixObviousTypos } from "./typoFixes";

/** Плейсхолдеры для локальной подстановки при экспорте (docs/07 §7). */
export interface ExportSubstitutions {
  "[ДАТА]"?: string;
  "[ВРЕМЯ]"?: string;
  "[ФИО_ВРАЧА]"?: string;
  "[ДОЛЖНОСТЬ_ВРАЧА]"?: string;
  "[НОМЕР_ИБ]"?: string;
  "[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]"?: string;
  "[ДОЛЖНОСТЬ_ЗАВ_ОТДЕЛЕНИЕМ]"?: string;
  "[ЛУ]"?: string;
}

/** Поля подписи из профиля врача (настройки аккаунта). */
export type SignatureDoctor = {
  display_name?: string;
  full_name?: string;
  position?: string;
  head_full_name?: string;
  head_position?: string;
  head_institution?: string;
} | null;

/** Время осмотра в шапке дневника — 10 час. 00 мин. */
export const DIARY_EXAM_HOUR = 10;
export const DIARY_EXAM_MINUTE = 0;
export const DIARY_EXAM_TIME = "10 час. 00 мин.";

const ISO_DAY_RE = /^(\d{4})-(\d{2})-(\d{2})/;
const DMY_RE = /(\d{2})\.(\d{2})\.(\d{4})/;
const NUMERIC_HEADER_RE =
  /^(\d{2})\.(\d{2})\.(\d{4})\s+время:\s+(\d{1,2}):(\d{2})\s*$/;

const MONTHS_GENITIVE = [
  "",
  "января",
  "февраля",
  "марта",
  "апреля",
  "мая",
  "июня",
  "июля",
  "августа",
  "сентября",
  "октября",
  "ноября",
  "декабря",
];

function pad2(n: number | string): string {
  return String(n).padStart(2, "0");
}

/** «13» августа 2026 г. */
export function formatOfficialDate(day: number, month: number, year: number): string {
  return `«${pad2(day)}» ${MONTHS_GENITIVE[month]} ${year} г.`;
}

/** 10 час. 00 мин. */
export function formatOfficialTime(
  hour: number = DIARY_EXAM_HOUR,
  minute: number = DIARY_EXAM_MINUTE,
): string {
  return `${pad2(hour)} час. ${pad2(minute)} мин.`;
}

function partsFromIsoOrDmy(raw?: string): { d: number; m: number; y: number } | undefined {
  if (!raw) return undefined;
  const trimmed = raw.trim();
  const isoDay = ISO_DAY_RE.exec(trimmed);
  if (isoDay) {
    return { y: Number(isoDay[1]), m: Number(isoDay[2]), d: Number(isoDay[3]) };
  }
  const dmy = DMY_RE.exec(trimmed);
  if (dmy) {
    return { d: Number(dmy[1]), m: Number(dmy[2]), y: Number(dmy[3]) };
  }
  return undefined;
}

/** Дата осмотра из заголовка пакета: «День 8 · 27.07.2026 · …». */
export function diaryDateFromTitle(title?: string): string | undefined {
  const m = DMY_RE.exec(title ?? "");
  return m ? `${m[1]}.${m[2]}.${m[3]}` : undefined;
}

/** Форматирует дату записи для подстановки [ДАТА] без сдвига UTC. */
export function formatExportDate(iso?: string): string | undefined {
  const p = partsFromIsoOrDmy(iso);
  if (!p) return undefined;
  return formatOfficialDate(p.d, p.m, p.y);
}

/** Форматирует время записи для подстановки [ВРЕМЯ]. */
export function formatExportTime(_iso?: string): string | undefined {
  return formatOfficialTime();
}

/** Собирает подстановки из даты осмотра, метаданных записи и профиля врача. */
export function buildExportSubstitutions(opts: {
  diaryDate?: string;
  title?: string;
  createdAt?: string;
  doctorName?: string;
  headName?: string;
  caseNumber?: string;
  doctor?: SignatureDoctor;
}): Record<string, string> {
  const subs: Record<string, string> = {};
  const date =
    formatExportDate(opts.diaryDate) ||
    formatExportDate(diaryDateFromTitle(opts.title)) ||
    formatExportDate(opts.createdAt);
  if (date) subs["[ДАТА]"] = date;
  subs["[ВРЕМЯ]"] = formatOfficialTime();
  const fromProfile = signatureFields(opts.doctor);
  const doctorName = opts.doctorName?.trim() || fromProfile.doctorName;
  if (doctorName) subs["[ФИО_ВРАЧА]"] = doctorName;
  if (fromProfile.doctorPosition) subs["[ДОЛЖНОСТЬ_ВРАЧА]"] = fromProfile.doctorPosition;
  const headFio = opts.headName?.trim() || opts.doctor?.head_full_name?.trim();
  if (headFio) subs["[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]"] = headFio;
  if (opts.doctor?.head_position?.trim()) {
    subs["[ДОЛЖНОСТЬ_ЗАВ_ОТДЕЛЕНИЕМ]"] = opts.doctor.head_position.trim();
  }
  if (opts.doctor?.head_institution?.trim()) {
    subs["[ЛУ]"] = opts.doctor.head_institution.trim();
  }
  if (opts.caseNumber?.trim()) subs["[НОМЕР_ИБ]"] = opts.caseNumber.trim();
  return subs;
}

const ATTENDING_PREFIX_RE = /^\s*лечащий\s+врач\s*[,.\-–—:]?\s*/i;

/** Должность без повтора «Лечащий врач» — он уже есть в строке подписи. */
export function doctorPositionLabel(raw?: string): string {
  return (raw ?? "").replace(ATTENDING_PREFIX_RE, "").trim();
}

/** ФИО врача, должность и строка заведующего для бланка. */
export function signatureFields(doctor?: SignatureDoctor): {
  doctorName?: string;
  doctorPosition?: string;
  headName?: string;
} {
  const fio = doctor?.full_name?.trim() || "";
  const position = doctorPositionLabel(doctor?.position);
  const headName = composeHeadSignature(doctor) || undefined;
  return {
    doctorName: fio || undefined,
    doctorPosition: position || undefined,
    headName,
  };
}

const PH_DOCTOR_NAME = "[ФИО_ВРАЧА]";
const PH_DOCTOR_POS = "[ДОЛЖНОСТЬ_ВРАЧА]";
const PH_HEAD_NAME = "[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]";
const PH_HEAD_POS = "[ДОЛЖНОСТЬ_ЗАВ_ОТДЕЛЕНИЕМ]";
const PH_LU = "[ЛУ]";
const EXAM_10D_DOCTOR_LINE = `${PH_DOCTOR_NAME}, ${PH_DOCTOR_POS}`;
const EXAM_10D_HEAD_LINE = `${PH_HEAD_NAME}, ${PH_HEAD_POS}, ${PH_LU}`;
const DAILY_PLACEHOLDER_SIG = `${PH_DOCTOR_POS} ${PH_DOCTOR_NAME}`;

/** Должность и ФИО рядом — как в ежедневном бланке. */
export function composeDailyDoctorLine(doctor?: SignatureDoctor): string {
  const { doctorName, doctorPosition } = signatureFields(doctor);
  return [doctorPosition || PH_DOCTOR_POS, doctorName || PH_DOCTOR_NAME].join(" ");
}

/**
 * Подпись заведующего в осмотре за 10 дней:
 * «ФИО. Должность (код). Лечебное учреждение».
 * Если в учреждении есть запятая, часть до неё идёт в скобки.
 */
export function composeHeadSignature(doctor?: SignatureDoctor): string {
  return [
    doctor?.head_full_name?.trim() || PH_HEAD_NAME,
    doctor?.head_position?.trim() || PH_HEAD_POS,
    doctor?.head_institution?.trim() || PH_LU,
  ].join(", ");
}

function rewriteNumericDateHeaders(content: string): string {
  return content
    .replace(/\r\n/g, "\n")
    .replace(/\r/g, "\n")
    .split("\n")
    .map((line) => {
      const m = NUMERIC_HEADER_RE.exec(line.trim());
      if (!m) return line;
      const date = formatOfficialDate(Number(m[1]), Number(m[2]), Number(m[3]));
      const time = formatOfficialTime(Number(m[4]), Number(m[5]));
      return `${date} время: ${time}`;
    })
    .join("\n");
}

/** Подставить дату/время осмотра и подпись в текст карточки (как в Word). */
export function applyDiaryStamp(
  content: string,
  opts: {
    title?: string;
    diaryDate?: string;
    createdAt?: string;
    doctor?: SignatureDoctor;
    doctorName?: string;
    headName?: string;
    caseNumber?: string;
  },
): string {
  const subs = buildExportSubstitutions(opts);
  let out = forceCanonicalSignatures(
    rewriteSignaturePlaceholders(
      rewriteHeadSignatureCaption(rewriteNumericDateHeaders(content)),
    ),
  );
  for (const [key, val] of Object.entries(subs)) {
    if (val) out = out.split(key).join(val);
  }
  if (opts.doctor) {
    out = tidySignatureCommas(out);
  }
  return ensureExam10dCaseNo(rewriteDailyForm(fixObviousTypos(out)), opts.caseNumber);
}

/** Предпросмотр / копирование: штамп, пустые разделы, пробелы ежедневника. */
export function prepareDiaryPreview(
  content: string,
  opts: {
    title?: string;
    diaryDate?: string;
    createdAt?: string;
    doctor?: SignatureDoctor;
  },
): string {
  return normalizeDailySpacing(omitEmptyDiarySections(applyDiaryStamp(content, opts)));
}

const DOCTOR_SIGNATURE_CAPTION =
  "Фамилия, имя, отчество (при наличии) врача, должность, специальность, подпись";
const HEAD_SIGNATURE_CAPTION =
  "Фамилия, имя, отчество (при наличии) заведующего отделением, подпись";
const DAILY_DOCTOR_LINE = "Лечащий врач, [ДОЛЖНОСТЬ_ВРАЧА], [ФИО_ВРАЧА]";
const DAILY_EXAM_TITLE = "Осмотр лечащим врачом";
const EXAM_10D_COMBINED =
  "ОСМОТР лечащим врачом совместно с заведующим отделением";
const EXAM_10D_SPLIT = "ОСМОТР\nлечащим врачом совместно с заведующим отделением";
const OFFICIAL_DATE_RE =
  /^«\s*(\d{1,2})\s*»\s+(\S+)\s+(\d{4})\s+г\.(?:\s+время:)?\s+(\d{1,2})\s+час\.\s+(\d{1,2})\s*мин\.?$/;
const DAILY_DATE_PREFIX_RE = /^дата:\s*/i;

function isDailyExam(content: string): boolean {
  return content
    .replace(/\r\n/g, "\n")
    .replace(/\r/g, "\n")
    .split("\n")
    .some((raw) => {
      const line = raw.trim();
      const upper = line.toUpperCase();
      return (
        upper === "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ" ||
        line.toLowerCase() === DAILY_EXAM_TITLE.toLowerCase()
      );
    });
}

function monthFromGenitive(name: string): number {
  return MONTHS_GENITIVE.findIndex((m) => m === name.toLowerCase());
}

function formatDailyDateLine(
  day: number,
  month: number,
  year: number,
  hour: number,
  minute: number,
): string {
  return `Дата: ${pad2(day)}.${pad2(month)}.${year} ${pad2(hour)}:${pad2(minute)}`;
}

function rewriteDailyDateLine(line: string): string | null {
  const trimmed = line.trim();
  const body = trimmed.replace(DAILY_DATE_PREFIX_RE, "");
  const official = OFFICIAL_DATE_RE.exec(body);
  if (official) {
    const month = monthFromGenitive(official[2]);
    if (month <= 0) return null;
    return formatDailyDateLine(
      Number(official[1]),
      month,
      Number(official[3]),
      Number(official[4]),
      Number(official[5]),
    );
  }
  const numeric = NUMERIC_HEADER_RE.exec(body);
  if (numeric) {
    return formatDailyDateLine(
      Number(numeric[1]),
      Number(numeric[2]),
      Number(numeric[3]),
      Number(numeric[4]),
      Number(numeric[5]),
    );
  }
  if (DAILY_DATE_PREFIX_RE.test(trimmed)) return trimmed;
  return null;
}

function splitPositionAndName(rest: string): { pos: string; name: string } {
  const trimmed = rest.trim();
  if (!trimmed) return { pos: "", name: "" };
  const parts = trimmed.split(", ");
  if (parts.length >= 2) {
    const last = parts[parts.length - 1].trim();
    if (!last.toLowerCase().includes("врач") && last.split(/\s+/).length >= 2) {
      return { pos: parts.slice(0, -1).join(", ").trim(), name: last };
    }
  }
  if (trimmed.toLowerCase().includes("врач")) return { pos: trimmed, name: "" };
  return { pos: "", name: trimmed };
}

function rewriteDailyDoctorLine(line: string): string | null {
  const trimmed = line.trim();
  if (!trimmed.startsWith("Лечащий врач")) return null;
  let rest = trimmed.slice("Лечащий врач".length).replace(/^[:,.\-–—]\s*/, "").trim();
  let real = rest.split(PH_DOCTOR_POS).join("").split(PH_DOCTOR_NAME).join("");
  while (real.includes(",,")) real = real.replace(/,,/g, ",");
  real = real.replace(/^[,\s;]+|[,\s;]+$/g, "");
  const { pos, name } = splitPositionAndName(real);
  return `${pos || PH_DOCTOR_POS} ${name || PH_DOCTOR_NAME}`;
}

function rewriteExam10dHeader(content: string): string {
  if (isDailyExam(content)) return content;
  return content.split(EXAM_10D_COMBINED).join(EXAM_10D_SPLIT);
}

/** Ежедневный бланк: заголовок, «Дата: ДД.ММ.ГГГГ ЧЧ:ММ», подпись без шапки ИБ. */
export function rewriteDailyForm(content: string): string {
  if (!isDailyExam(content)) return rewriteExam10dHeader(content);
  const out: string[] = [];
  for (const raw of content.replace(/\r\n/g, "\n").replace(/\r/g, "\n").split("\n")) {
    const line = raw.trim();
    if (!line) continue;
    const upper = line.toUpperCase();
    if (line.startsWith("ИБ №") || upper.startsWith("ИБ N")) continue;
    if (upper === "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ" || line.toLowerCase() === DAILY_EXAM_TITLE.toLowerCase()) {
      out.push(DAILY_EXAM_TITLE);
      continue;
    }
    if (line === DOCTOR_SIGNATURE_CAPTION) continue;
    const date = rewriteDailyDateLine(line);
    if (date) {
      out.push(date);
      continue;
    }
    const doctor = rewriteDailyDoctorLine(line);
    if (doctor) {
      out.push(doctor);
      continue;
    }
    out.push(line);
  }
  while (out.length && !out[0]) out.shift();
  while (out.length && !out[out.length - 1]) out.pop();
  return out.join("\n");
}

function formatCaseNo(raw?: string): string {
  let s = (raw ?? "").trim();
  s = s.replace(/^ИБ\s*№\s*/i, "").replace(/^ИБ\s*N\s*/i, "").trim();
  if (!s || (s.includes("[") && s.includes("]"))) return "ИБ №";
  return `ИБ №${s}`;
}

function isCaseNoLine(line: string): boolean {
  const t = line.trim();
  return t.startsWith("ИБ №") || t.toUpperCase().startsWith("ИБ N");
}

function caseNoFromContent(content: string, fallback?: string): string {
  if (fallback?.trim()) return formatCaseNo(fallback);
  for (const raw of content.split("\n")) {
    if (isCaseNoLine(raw) && !raw.includes("[")) return formatCaseNo(raw);
  }
  return "ИБ №";
}

/** ИБ только в шапке осмотра заведующим, без нераскрытого [НОМЕР_ИБ]. */
function ensureExam10dCaseNo(content: string, caseNumber?: string): string {
  if (isDailyExam(content)) return content;
  if (!content.includes("заведующим") && !content.toUpperCase().includes("ОСМОТР")) {
    return content;
  }
  const ib = caseNoFromContent(content, caseNumber);
  const lines = content.replace(/\r\n/g, "\n").replace(/\r/g, "\n").split("\n");
  const out: string[] = [];
  let saw = false;
  for (const raw of lines) {
    if (isCaseNoLine(raw)) {
      if (!saw) {
        out.push(ib);
        saw = true;
      }
      continue;
    }
    out.push(raw);
  }
  if (!saw) out.unshift(ib);
  return out.join("\n");
}

function isFollowingSignatureValue(raw: string): boolean {
  const line = raw.trim();
  if (!line || line === DOCTOR_SIGNATURE_CAPTION || line === HEAD_SIGNATURE_CAPTION) {
    return false;
  }
  if (/^[А-ЯA-Z][^:]{2,80}:/.test(line) && !line.startsWith("[")) return false;
  const upper = line.toUpperCase();
  if (upper === "ОСМОТР" || line.toLowerCase() === DAILY_EXAM_TITLE.toLowerCase()) return false;
  if (line.startsWith("ИБ") || /^дата:/i.test(line)) return false;
  return true;
}

function skipFollowingSignatureValue(lines: string[], i: number): number {
  let j = i + 1;
  while (j < lines.length && !lines[j].trim()) j++;
  if (j < lines.length && isFollowingSignatureValue(lines[j])) return j;
  return i;
}

function forceCanonicalSignatures(content: string): string {
  const daily = isDailyExam(content);
  const lines = content.replace(/\r\n/g, "\n").replace(/\r/g, "\n").split("\n");
  const out: string[] = [];
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim();
    if (line === DOCTOR_SIGNATURE_CAPTION) {
      i = skipFollowingSignatureValue(lines, i);
      if (daily) out.push(DAILY_PLACEHOLDER_SIG);
      else out.push(DOCTOR_SIGNATURE_CAPTION, EXAM_10D_DOCTOR_LINE);
      continue;
    }
    if (line === HEAD_SIGNATURE_CAPTION) {
      i = skipFollowingSignatureValue(lines, i);
      out.push(HEAD_SIGNATURE_CAPTION, EXAM_10D_HEAD_LINE);
      continue;
    }
    out.push(lines[i]);
  }
  return out.join("\n");
}

function rewriteHeadSignatureCaption(content: string): string {
  return content.split(DOCTOR_SIGNATURE_CAPTION + "\n[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]").join(
    HEAD_SIGNATURE_CAPTION + "\n[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]",
  );
}

function rewriteSignaturePlaceholders(content: string): string {
  let out = content.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
  const isDaily = isDailyExam(out);
  const replacement = isDaily
    ? DAILY_DOCTOR_LINE
    : "[ФИО_ВРАЧА], [ДОЛЖНОСТЬ_ВРАЧА]";
  out = out.replace(/^Лечащий врач:?\s*\[ФИО_ВРАЧА\]\s*$/gm, replacement);
  const doctorWithPos = DOCTOR_SIGNATURE_CAPTION + "\n[ФИО_ВРАЧА], [ДОЛЖНОСТЬ_ВРАЧА]";
  const doctorOnly = DOCTOR_SIGNATURE_CAPTION + "\n[ФИО_ВРАЧА]";
  if (!out.includes("[ФИО_ВРАЧА], [ДОЛЖНОСТЬ_ВРАЧА]")) {
    out = out.split(doctorOnly).join(doctorWithPos);
  }
  return out;
}

function tidySignatureCommas(content: string): string {
  return content
    .split("\n")
    .map((line) => {
      let out = line;
      while (/,\s*,/.test(out)) out = out.replace(/,\s*,/g, ",");
      if (!out.includes(":") && !out.includes("\t")) {
        out = out.replace(/,\s*$/, "").replace(/^\s*,\s*/, "");
        out = out.replace(/\s{2,}/g, " ").trim();
      }
      return out;
    })
    .join("\n");
}
