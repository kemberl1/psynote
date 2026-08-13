/** Плейсхолдеры для локальной подстановки при экспорте (docs/07 §7). */
export interface ExportSubstitutions {
  "[ДАТА]"?: string;
  "[ВРЕМЯ]"?: string;
  "[ФИО_ВРАЧА]"?: string;
  "[НОМЕР_ИБ]"?: string;
  "[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]"?: string;
}

/** Время осмотра в шапке дневника — всегда 10:00. */
export const DIARY_EXAM_TIME = "10:00";

const ISO_DAY_RE = /^(\d{4})-(\d{2})-(\d{2})/;
const DMY_RE = /(\d{2}\.\d{2}\.\d{4})/;

/** Дата осмотра из заголовка пакета: «День 8 · 27.07.2026 · …». */
export function diaryDateFromTitle(title?: string): string | undefined {
  const m = DMY_RE.exec(title ?? "");
  return m?.[1];
}

/** Форматирует дату записи для подстановки [ДАТА] без сдвига UTC. */
export function formatExportDate(iso?: string): string | undefined {
  if (!iso) return undefined;
  const trimmed = iso.trim();
  const isoDay = ISO_DAY_RE.exec(trimmed);
  if (isoDay) return `${isoDay[3]}.${isoDay[2]}.${isoDay[1]}`;
  const dmy = /^(\d{2}\.\d{2}\.\d{4})$/.exec(trimmed);
  if (dmy) return dmy[1];
  const d = new Date(trimmed);
  if (Number.isNaN(d.getTime())) return undefined;
  const dd = String(d.getUTCDate()).padStart(2, "0");
  const mm = String(d.getUTCMonth() + 1).padStart(2, "0");
  return `${dd}.${mm}.${d.getUTCFullYear()}`;
}

/** Форматирует время записи для подстановки [ВРЕМЯ]. */
export function formatExportTime(_iso?: string): string | undefined {
  return DIARY_EXAM_TIME;
}

/** Собирает подстановки из даты осмотра, метаданных записи и профиля врача. */
export function buildExportSubstitutions(opts: {
  diaryDate?: string;
  title?: string;
  createdAt?: string;
  doctorName?: string;
  caseNumber?: string;
}): Record<string, string> {
  const subs: Record<string, string> = {};
  const date =
    formatExportDate(opts.diaryDate) ||
    diaryDateFromTitle(opts.title) ||
    formatExportDate(opts.createdAt);
  if (date) subs["[ДАТА]"] = date;
  subs["[ВРЕМЯ]"] = DIARY_EXAM_TIME;
  if (opts.doctorName?.trim()) subs["[ФИО_ВРАЧА]"] = opts.doctorName.trim();
  if (opts.caseNumber?.trim()) subs["[НОМЕР_ИБ]"] = opts.caseNumber.trim();
  return subs;
}

/** Подставить дату/время осмотра в текст карточки (как в Word). */
export function applyDiaryStamp(
  content: string,
  opts: { title?: string; diaryDate?: string; createdAt?: string },
): string {
  const subs = buildExportSubstitutions(opts);
  let out = content;
  for (const key of ["[ДАТА]", "[ВРЕМЯ]"] as const) {
    const val = subs[key];
    if (val) out = out.split(key).join(val);
  }
  return out;
}
