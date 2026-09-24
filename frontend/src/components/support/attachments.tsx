// Вложения чата поддержки: скрепка, черновые файлы, картинки в переписке
// и просмотр на весь экран. Общие для виджета врача и админ-инбокса.
import { useCallback, useEffect, useRef, useState } from "react";
import type { ClipboardEvent, DragEvent } from "react";
import { createPortal } from "react-dom";
import { fetchSupportAttachment } from "../../api/endpoints";
import type { SupportAttachmentScope } from "../../api/endpoints";
import { useSupportAttachment } from "../../api/queries";
import type { SupportAttachment } from "../../api/types";
import { triggerBlobDownload } from "../../lib/download";
import { Spinner } from "../ui";

export const SUPPORT_MAX_FILES = 5;
export const SUPPORT_MAX_FILE_SIZE = 10 * 1024 * 1024;

/** Картинки, которые сервер отдаёт для показа (остальное — скачивание). */
const INLINE_IMAGE_TYPES = new Set(["image/png", "image/jpeg", "image/gif", "image/webp"]);

function isInlineImage(contentType: string): boolean {
  return INLINE_IMAGE_TYPES.has(contentType);
}

export function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} Б`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} КБ`;
  return `${(bytes / (1024 * 1024)).toFixed(1).replace(".", ",")} МБ`;
}

function useObjectUrl(blob: Blob | undefined | null): string | null {
  const [url, setUrl] = useState<string | null>(null);
  useEffect(() => {
    if (!blob) {
      setUrl(null);
      return;
    }
    const next = URL.createObjectURL(blob);
    setUrl(next);
    return () => URL.revokeObjectURL(next);
  }, [blob]);
  return url;
}

// ─── Черновик: выбранные, но ещё не отправленные файлы ─────────────────────

export function useDraftFiles() {
  const [files, setFiles] = useState<File[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [dragging, setDragging] = useState(false);
  const dragDepth = useRef(0);

  const add = useCallback((incoming: Iterable<File>) => {
    const list = Array.from(incoming);
    if (list.length === 0) return;
    setFiles((prev) => {
      const next = [...prev];
      let problem: string | null = null;
      for (const f of list) {
        if (f.size === 0) {
          problem = `Файл «${f.name}» пустой`;
          continue;
        }
        if (f.size > SUPPORT_MAX_FILE_SIZE) {
          problem = `Файл «${f.name}» больше 10 МБ`;
          continue;
        }
        if (next.length >= SUPPORT_MAX_FILES) {
          problem = `Не больше ${SUPPORT_MAX_FILES} файлов за раз`;
          break;
        }
        next.push(f);
      }
      setError(problem);
      return next;
    });
  }, []);

  const remove = useCallback((index: number) => {
    setFiles((prev) => prev.filter((_, i) => i !== index));
    setError(null);
  }, []);

  const clear = useCallback(() => {
    setFiles([]);
    setError(null);
  }, []);

  /** Вставка скриншота из буфера (Ctrl/Cmd+V). Текст вставляется как обычно. */
  const onPaste = useCallback(
    (e: ClipboardEvent) => {
      const pasted = Array.from(e.clipboardData.files);
      if (pasted.length === 0 || e.clipboardData.getData("text/plain")) return;
      e.preventDefault();
      add(
        pasted.map((f) =>
          f.name && f.name !== "image.png"
            ? f
            : new File([f], `скриншот-${Date.now()}.${f.type.split("/")[1] || "png"}`, { type: f.type }),
        ),
      );
    },
    [add],
  );

  const hasFiles = (e: DragEvent) => Array.from(e.dataTransfer.types).includes("Files");

  const dropHandlers = {
    onDragEnter: (e: DragEvent) => {
      if (!hasFiles(e)) return;
      e.preventDefault();
      dragDepth.current += 1;
      setDragging(true);
    },
    onDragOver: (e: DragEvent) => {
      if (!hasFiles(e)) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = "copy";
    },
    onDragLeave: (e: DragEvent) => {
      if (!hasFiles(e)) return;
      dragDepth.current = Math.max(0, dragDepth.current - 1);
      if (dragDepth.current === 0) setDragging(false);
    },
    onDrop: (e: DragEvent) => {
      if (!hasFiles(e)) return;
      e.preventDefault();
      dragDepth.current = 0;
      setDragging(false);
      add(e.dataTransfer.files);
    },
  };

  return { files, error, dragging, add, remove, clear, onPaste, dropHandlers };
}

function PaperclipIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="m20.5 11.5-8.1 8.1a5 5 0 0 1-7.1-7.1l8.5-8.5a3.3 3.3 0 0 1 4.7 4.7l-8.5 8.5a1.7 1.7 0 0 1-2.4-2.4l7.8-7.8"
        stroke="currentColor"
        strokeWidth="1.7"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function AttachButton({
  onFiles,
  disabled,
}: {
  onFiles: (files: FileList) => void;
  disabled?: boolean;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  return (
    <>
      <button
        type="button"
        className="support-attach-btn"
        aria-label="Прикрепить файл или картинку"
        title="Прикрепить файл или картинку"
        disabled={disabled}
        onClick={() => inputRef.current?.click()}
      >
        <PaperclipIcon />
      </button>
      <input
        ref={inputRef}
        type="file"
        multiple
        hidden
        onChange={(e) => {
          if (e.target.files) onFiles(e.target.files);
          e.target.value = "";
        }}
      />
    </>
  );
}

function DraftFile({ file, onRemove }: { file: File; onRemove: () => void }) {
  const image = file.type.startsWith("image/");
  const url = useObjectUrl(image ? file : null);
  return (
    <div className={`support-draft__item${image ? " support-draft__item--image" : ""}`}>
      {image && url ? (
        <img src={url} alt={file.name} className="support-draft__thumb" />
      ) : (
        <div className="support-draft__file">
          <span className="support-draft__name">{file.name}</span>
          <span className="support-draft__size">{formatFileSize(file.size)}</span>
        </div>
      )}
      <button
        type="button"
        className="support-draft__remove"
        aria-label={`Убрать ${file.name}`}
        onClick={onRemove}
      >
        ×
      </button>
    </div>
  );
}

export function DraftFiles({
  files,
  error,
  onRemove,
}: {
  files: File[];
  error: string | null;
  onRemove: (index: number) => void;
}) {
  if (files.length === 0 && !error) return null;
  return (
    <div className="support-draft">
      {files.length > 0 && (
        <div className="support-draft__list">
          {files.map((f, i) => (
            <DraftFile key={`${f.name}-${f.size}-${f.lastModified}-${i}`} file={f} onRemove={() => onRemove(i)} />
          ))}
        </div>
      )}
      {error && <div className="support-draft__error">{error}</div>}
    </div>
  );
}

export function DropOverlay({ visible }: { visible: boolean }) {
  if (!visible) return null;
  return (
    <div className="support-drop" aria-hidden="true">
      <div className="support-drop__inner">Отпустите, чтобы прикрепить</div>
    </div>
  );
}

// ─── Вложения в отправленных сообщениях ────────────────────────────────────

function Lightbox({
  url,
  attachment,
  blob,
  onClose,
}: {
  url: string;
  attachment: SupportAttachment;
  blob: Blob;
  onClose: () => void;
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return createPortal(
    <div className="support-lightbox" role="dialog" aria-label={attachment.filename} onClick={onClose}>
      <div className="support-lightbox__bar" onClick={(e) => e.stopPropagation()}>
        <span className="support-lightbox__name">{attachment.filename}</span>
        <button
          type="button"
          className="support-lightbox__action"
          onClick={() => triggerBlobDownload(blob, attachment.filename)}
        >
          Скачать
        </button>
        <button type="button" className="support-lightbox__action" aria-label="Закрыть" onClick={onClose}>
          ×
        </button>
      </div>
      <img
        src={url}
        alt={attachment.filename}
        className="support-lightbox__img"
        onClick={(e) => e.stopPropagation()}
      />
    </div>,
    document.body,
  );
}

function AttachmentImage({
  attachment,
  scope,
  onLoad,
}: {
  attachment: SupportAttachment;
  scope: SupportAttachmentScope;
  onLoad?: () => void;
}) {
  const { data: blob, isError, refetch } = useSupportAttachment(attachment.id, scope);
  const url = useObjectUrl(blob);
  const [open, setOpen] = useState(false);
  const close = useCallback(() => setOpen(false), []);

  if (isError) {
    return (
      <button type="button" className="support-att-img support-att-img--error" onClick={() => void refetch()}>
        Не удалось загрузить картинку — повторить
      </button>
    );
  }
  if (!url || !blob) {
    return (
      <div className="support-att-img support-att-img--loading">
        <Spinner />
      </div>
    );
  }
  return (
    <>
      <button
        type="button"
        className="support-att-img"
        title={attachment.filename}
        onClick={() => setOpen(true)}
      >
        <img src={url} alt={attachment.filename} onLoad={onLoad} />
      </button>
      {open && <Lightbox url={url} attachment={attachment} blob={blob} onClose={close} />}
    </>
  );
}

function AttachmentFile({
  attachment,
  scope,
}: {
  attachment: SupportAttachment;
  scope: SupportAttachmentScope;
}) {
  const [state, setState] = useState<"idle" | "loading" | "error">("idle");
  const download = async () => {
    setState("loading");
    try {
      const blob = await fetchSupportAttachment(attachment.id, scope);
      triggerBlobDownload(blob, attachment.filename);
      setState("idle");
    } catch {
      setState("error");
    }
  };
  return (
    <button
      type="button"
      className="support-att-file"
      onClick={() => void download()}
      disabled={state === "loading"}
      title="Скачать"
    >
      <span className="support-att-file__icon" aria-hidden="true">
        {state === "loading" ? <Spinner /> : "📄"}
      </span>
      <span className="support-att-file__meta">
        <span className="support-att-file__name">{attachment.filename}</span>
        <span className="support-att-file__size">
          {state === "error" ? "не скачалось — ещё раз" : formatFileSize(attachment.size)}
        </span>
      </span>
    </button>
  );
}

export function MessageAttachments({
  attachments,
  scope,
  onMediaLoad,
}: {
  attachments: SupportAttachment[] | undefined;
  scope: SupportAttachmentScope;
  onMediaLoad?: () => void;
}) {
  if (!attachments || attachments.length === 0) return null;
  const images = attachments.filter((a) => isInlineImage(a.content_type));
  const files = attachments.filter((a) => !isInlineImage(a.content_type));
  return (
    <div className="support-msg__attachments">
      {images.length > 0 && (
        <div className={`support-att-grid${images.length === 1 ? " support-att-grid--single" : ""}`}>
          {images.map((a) => (
            <AttachmentImage key={a.id} attachment={a} scope={scope} onLoad={onMediaLoad} />
          ))}
        </div>
      )}
      {files.map((a) => (
        <AttachmentFile key={a.id} attachment={a} scope={scope} />
      ))}
    </div>
  );
}
