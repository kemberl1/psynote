// Скачивание бинарных файлов экспорта (docs/07 §7, docs/08 §5.3).
//
// Эндпоинты POST /requests/{id}/export и POST /export/batch отдают НЕ JSON,
// а бинарный файл — используем прямой fetch с Bearer-токеном.
import { API_BASE } from "../api/client";
import { ApiError } from "../api/errors";
import { getAccessToken } from "../api/session";
import type { BatchExportRequest, ExportFormat, ExportRequest } from "../api/types";

/** MIME-типы по формату (для фолбэка имени файла). */
const EXT_BY_FORMAT: Record<ExportFormat, string> = {
  docx: "docx",
  txt: "txt",
};

async function downloadBinary(
  url: string,
  body: unknown,
  fallbackName: string,
  format: ExportFormat,
): Promise<void> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json; charset=utf-8",
  };
  const access = getAccessToken();
  if (access) headers["Authorization"] = `Bearer ${access}`;

  let res: Response;
  try {
    res = await fetch(url, {
      method: "POST",
      headers,
      body: JSON.stringify(body),
    });
  } catch {
    throw new ApiError("NETWORK", "сетевая ошибка", 0);
  }

  if (!res.ok) {
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
    `${fallbackName}.${EXT_BY_FORMAT[format]}`;

  triggerBlobDownload(blob, filename);
}

/**
 * Запрашивает экспорт одной записи истории и сохраняет файл в браузере.
 */
export async function downloadExport(
  requestId: string,
  body: ExportRequest,
  fallbackName = "diary",
): Promise<void> {
  const url = `${API_BASE}/requests/${encodeURIComponent(requestId)}/export`;
  await downloadBinary(url, body, fallbackName, body.format);
}

/**
 * Запрашивает пакетный экспорт нескольких записей в один файл.
 */
export async function downloadBatchExport(
  body: BatchExportRequest,
  fallbackName = "diaries_batch",
): Promise<void> {
  const url = `${API_BASE}/export/batch`;
  await downloadBinary(url, body, fallbackName, body.format);
}

/** Извлекает filename из заголовка Content-Disposition (RFC 5987 / plain). */
export function filenameFromDisposition(header: string | null): string | null {
  if (!header) return null;
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
  setTimeout(() => URL.revokeObjectURL(objectUrl), 1000);
}
