// Утилиты форматирования для UI (без ПДн — только метаданные/даты/коды).
import type { DocumentType } from "../api/types";

/** Человекочитаемые названия типов документов (fallback, если справочник не загружен). */
const DOC_TYPE_LABELS: Record<string, string> = {
  daily: "Ежедневный дневник",
  exam_10d: "Осмотр (раз в 10 дней)",
};

/** Возвращает заголовок типа документа по коду. */
export function documentTypeLabel(
  code: string,
  types?: DocumentType[],
): string {
  const found = types?.find((t) => t.code === code);
  return found?.title ?? DOC_TYPE_LABELS[code] ?? code;
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

/** Склонение «N персональных данных» (1 — данное, 2-4 — данных, ...). */
export function pluralizePii(n: number): string {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return `${n} персональное данное`;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20))
    return `${n} персональных данных`;
  return `${n} персональных данных`;
}
