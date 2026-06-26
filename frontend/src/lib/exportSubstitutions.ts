/** Плейсхолдеры для локальной подстановки при экспорте (docs/07 §7). */
export interface ExportSubstitutions {
  "[ДАТА]"?: string;
  "[ВРЕМЯ]"?: string;
  "[ФИО_ВРАЧА]"?: string;
  "[НОМЕР_ИБ]"?: string;
  "[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]"?: string;
}

/** Форматирует дату записи для подстановки [ДАТА]. */
export function formatExportDate(iso?: string): string | undefined {
  if (!iso) return undefined;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return undefined;
  const dd = String(d.getDate()).padStart(2, "0");
  const mm = String(d.getMonth() + 1).padStart(2, "0");
  return `${dd}.${mm}.${d.getFullYear()}`;
}

/** Форматирует время записи для подстановки [ВРЕМЯ]. */
export function formatExportTime(iso?: string): string | undefined {
  if (!iso) return undefined;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return undefined;
  const hh = String(d.getHours()).padStart(2, "0");
  const min = String(d.getMinutes()).padStart(2, "0");
  return `${hh}:${min}`;
}

/** Собирает подстановки из метаданных записи и профиля врача. */
export function buildExportSubstitutions(opts: {
  createdAt?: string;
  doctorName?: string;
  caseNumber?: string;
}): Record<string, string> {
  const subs: Record<string, string> = {};
  const date = formatExportDate(opts.createdAt);
  const time = formatExportTime(opts.createdAt);
  if (date) subs["[ДАТА]"] = date;
  if (time) subs["[ВРЕМЯ]"] = time;
  if (opts.doctorName?.trim()) subs["[ФИО_ВРАЧА]"] = opts.doctorName.trim();
  if (opts.caseNumber?.trim()) subs["[НОМЕР_ИБ]"] = opts.caseNumber.trim();
  return subs;
}
