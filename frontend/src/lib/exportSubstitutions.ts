import { omitEmptyDiarySections, normalizeDailySpacing } from "./diaryMarkup";
import { fixObviousTypos } from "./typoFixes";

/** Плейсхолдеры для локальной подстановки при экспорте (docs/07 §7). */
export interface ExportSubstitutions {
  "[ДАТА]"?: string;
  "[ВРЕМЯ]"?: string;
  "[ФИО_ВРАЧА]"?: string;
  "[ДОЛЖНОСТЬ_ВРАЧА]"?: string;
  "[НОМЕР_ИБ]"?: string;
  "[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]"?: string;
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
  const headName = opts.headName?.trim() || fromProfile.headName;
  if (doctorName) subs["[ФИО_ВРАЧА]"] = doctorName;
  if (fromProfile.doctorPosition) subs["[ДОЛЖНОСТЬ_ВРАЧА]"] = fromProfile.doctorPosition;
  if (headName) subs["[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]"] = headName;
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
  const fio = doctor?.full_name?.trim() || doctor?.display_name?.trim() || "";
  const position = doctorPositionLabel(doctor?.position);
  const headName = composeHeadSignature(doctor) || undefined;
  return {
    doctorName: fio || undefined,
    doctorPosition: position || undefined,
    headName,
  };
}

/** «Лечащий врач, должность, ФИО» — как в ежедневном бланке. */
export function composeDailyDoctorLine(doctor?: SignatureDoctor): string {
  const { doctorName, doctorPosition } = signatureFields(doctor);
  return ["Лечащий врач", doctorPosition, doctorName].filter(Boolean).join(", ");
}

/**
 * Подпись заведующего в осмотре за 10 дней:
 * «ФИО. Должность (код). Лечебное учреждение».
 * Если в учреждении есть запятая, часть до неё идёт в скобки.
 */
export function composeHeadSignature(doctor?: SignatureDoctor): string {
  const fio = doctor?.head_full_name?.trim() || "";
  const position = doctor?.head_position?.trim() || "";
  const institution = doctor?.head_institution?.trim() || "";
  let code = "";
  let rest = "";
  const comma = institution.indexOf(",");
  if (comma >= 0) {
    code = institution.slice(0, comma).trim();
    rest = institution.slice(comma + 1).trim();
  } else {
    rest = institution;
  }
  const role = [position, code ? `(${code})` : ""].filter(Boolean).join(" ");
  const tail = [role, rest].filter(Boolean).join(". ");
  if (fio && tail) {
    const sep = /[.!?]$/.test(fio) ? " " : ". ";
    return `${fio}${sep}${tail}`;
  }
  return fio || tail;
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
  let out = rewriteSignaturePlaceholders(
    rewriteHeadSignatureCaption(rewriteNumericDateHeaders(content)),
  );
  for (const [key, val] of Object.entries(subs)) {
    if (val) out = out.split(key).join(val);
  }
  if (opts.doctor) {
    out = out.split("[ДОЛЖНОСТЬ_ВРАЧА]").join("");
    out = tidySignatureCommas(out);
  }
  return fixObviousTypos(out);
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

function rewriteHeadSignatureCaption(content: string): string {
  return content.split(DOCTOR_SIGNATURE_CAPTION + "\n[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]").join(
    HEAD_SIGNATURE_CAPTION + "\n[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]",
  );
}

function rewriteSignaturePlaceholders(content: string): string {
  let out = content.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
  const isDaily = out.toUpperCase().includes("ОСМОТР ЛЕЧАЩИМ ВРАЧОМ");
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
      if (!out.includes(":")) {
        out = out.replace(/,\s*$/, "").replace(/^\s*,\s*/, "");
        out = out.replace(/\s{2,}/g, " ").trim();
      }
      return out;
    })
    .join("\n");
}
