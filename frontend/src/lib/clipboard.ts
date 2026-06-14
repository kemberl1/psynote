// Копирование текста в буфер обмена с фолбэком (docs/08 §5.3).
// Экспорт в Word/PDF — Этап 8; здесь только копирование текста.

/** Копирует текст; возвращает true при успехе. */
export async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // переходим к фолбэку
  }
  // Фолбэк для не-secure контекста.
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}
