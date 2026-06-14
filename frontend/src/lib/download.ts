// Скачивание бинарных файлов экспорта (docs/07 §7, docs/08 §5.3).
//
// Эндпоинт POST /requests/{id}/export отдаёт НЕ JSON, а бинарный файл, поэтому
// общий JSON-клиент (api/client.ts) тут не подходит — используем прямой fetch,
// читаем тело как Blob и инициируем скачивание через временную <a download>.
// Имя файла берём из Content-Disposition (его формирует gateway, без ПДн);
// при отсутствии — фолбэк на переданное имя.
import { API_BASE } from "../api/client";
import { ApiError } from "../api/errors";
import type { ExportFormat, ExportRequest } from "../api/types";

/** MIME-типы по формату (для фолбэка имени файла). */
const EXT_BY_FORMAT: Record<ExportFormat, string> = {
  docx: "docx",
  pdf: "pdf",
  txt: "txt",
};

/**
 * Запрашивает экспорт записи истории и сохраняет файл в браузере.
 * Бросает ApiError при сетевом сбое или не-2xx ответе (для тоста на UI).
 *
 * @param requestId — id записи истории (request_id из /generate или /requests).
 * @param body — формат и (опционально) подстановки плейсхолдеров.
 * @param fallbackName — имя файла без расширения, если сервер не прислал имя.
 */
export async function downloadExport(
  requestId: string,
  body: ExportRequest,
  fallbackName = "diary",
): Promise<void> {
  const url = `${API_BASE}/requests/${encodeURIComponent(requestId)}/export`;

  let res: Response;
  try {
    res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json; charset=utf-8" },
      body: JSON.stringify(body),
    });
  } catch {
    throw new ApiError("NETWORK", "сетевая ошибка", 0);
  }

  if (!res.ok) {
    // Тело ошибки — JSON-конверт {error:{code,message}} (docs/07 §1).
    let code = "UNKNOWN";
    let message = `HTTP ${res.status}`;
    try {
      const env = await res.json();
      code = env?.error?.code ?? code;
      message = env?.error?.message ?? message;
    } catch {
      // не-JSON — оставляем дефолты
    }
    throw new ApiError(
      code === "NOT_FOUND" || res.status === 404 ? "NOT_FOUND" : "UNKNOWN",
      message,
      res.status,
    );
  }

  const blob = await res.blob();
  const filename =
    filenameFromDisposition(res.headers.get("Content-Disposition")) ??
    `${fallbackName}.${EXT_BY_FORMAT[body.format]}`;

  triggerBlobDownload(blob, filename);
}

/** Извлекает filename из заголовка Content-Disposition (RFC 5987 / plain). */
export function filenameFromDisposition(header: string | null): string | null {
  if (!header) return null;
  // filename*=UTF-8''<encoded> имеет приоритет (RFC 5987).
  const star = /filename\*=UTF-8''([^;]+)/i.exec(header);
  if (star?.[1]) {
    try {
      return decodeURIComponent(star[1].trim());
    } catch {
      // падаем в plain-вариант
    }
  }
  const plain = /filename="?([^";]+)"?/i.exec(header);
  return plain?.[1]?.trim() ?? null;
}

/** Создаёт временную ссылку и кликает по ней, затем чистит ресурсы. */
function triggerBlobDownload(blob: Blob, filename: string): void {
  const objectUrl = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = objectUrl;
  a.download = filename;
  a.style.display = "none";
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  // Освобождаем objectURL чуть позже, чтобы скачивание успело стартовать.
  setTimeout(() => URL.revokeObjectURL(objectUrl), 1000);
}
