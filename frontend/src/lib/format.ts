// Утилиты форматирования для UI (без ПДн — только метаданные/даты/коды).
import type { DocumentType } from "../api/types";

/** Человекочитаемые названия типов документов (fallback, если справочник не загружен). */
const DOC_TYPE_LABELS: Record<string, string> = {
  daily: "Ежедневный осмотр",
  exam_10d: "Осмотр за 10 дней",
  batch: "Пакет дневников",
};

/** Возвращает заголовок типа документа по коду. */
export function documentTypeLabel(
  code: string,
  types?: DocumentType[],
): string {
  const found = types?.find((t) => t.code === code);
  if (found?.title) {
    // Каталог может ещё отдавать старые длинные названия — нормализуем.
    if (code === "daily") return "Ежедневный осмотр";
    if (code === "exam_10d") return "Осмотр за 10 дней";
    return found.title;
  }
  return DOC_TYPE_LABELS[code] ?? code;
}

/** Короткий бейдж статуса для истории. */
export function statusLabel(status: string): string {
  switch (status) {
    case "pending":
    case "generating":
    case "anonymizing":
    case "retrieving":
      return "Формируется…";
    case "done":
      return "Готово";
    case "failed":
    case "blocked_pii":
      return "Ошибка";
    default:
      return status;
  }
}

export function isPendingStatus(status: string): boolean {
  return (
    status === "pending" ||
    status === "generating" ||
    status === "anonymizing" ||
    status === "retrieving"
  );
}

/** Форматирует ISO-дату в локальную «14 июня 2026, 17:42» (ru-RU). */
export function formatDateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("ru-RU", {
    day: "numeric",
    month: "long",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/** Короткая дата «14 июня» / «14.06.26» для компактного сайдбара. */
export function formatDateShort(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("ru-RU", {
    day: "numeric",
    month: "short",
  });
}

/** YYYY-MM-DD → ДД.ММ.ГГГГ (для подписей пакетных дней). */
export function formatDiaryDate(isoDate: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(isoDate.trim());
  if (!m) return isoDate;
  return `${m[3]}.${m[2]}.${m[1]}`;
}

/** Склонение «N персональных данных» (1 — данное, 2-4 — данных, ...). */
export function pluralizePii(n: number): string {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return `${n} персональное данное`;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20))
    return `${n} персональных данных`;
  return `${n} персональных данных`;
}
