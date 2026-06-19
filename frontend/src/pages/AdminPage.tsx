// Админ-страница загрузки документов в RAG (Этап 10, docs/08 §5.4).
// Drag-and-drop зона + прогресс + история загрузок.
import { useCallback, useEffect, useRef, useState } from "react";
import { fetchAdminDocuments, uploadAdminDocument } from "../api/endpoints";
import { ApiError } from "../api/errors";
import type { AdminDocument, AdminUploadResult } from "../api/types";
import { Badge, Button, Spinner } from "../components/ui";
import "./admin.css";

const ACCEPT = ".docx,.odt,.doc";
const MAX_SIZE = 15 * 1024 * 1024; // 15 MB

type UploadStatus = "idle" | "uploading" | "done" | "error";

interface UploadState {
  status: UploadStatus;
  result?: AdminUploadResult;
  error?: string;
}

export function AdminPage() {
  const [files, setFiles] = useState<File[]>([]);
  const [dragOver, setDragOver] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [uploadState, setUploadState] = useState<UploadState>({ status: "idle" });
  const [history, setHistory] = useState<AdminDocument[]>([]);
  const [loadingHistory, setLoadingHistory] = useState(true);
  const inputRef = useRef<HTMLInputElement>(null);

  // Загрузка истории при монтировании.
  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const data = await fetchAdminDocuments();
        if (!cancelled) setHistory(data.items);
      } catch { /* ignore */ } finally {
        if (!cancelled) setLoadingHistory(false);
      }
    }
    void load();
    return () => { cancelled = true; };
  }, []);

  const refreshHistory = useCallback(async () => {
    try {
      const data = await fetchAdminDocuments();
      setHistory(data.items);
    } catch { /* ignore */ }
  }, []);

  // Обработка выбранных файлов.
  const handleFiles = useCallback((newFiles: FileList | File[]) => {
    const arr = Array.from(newFiles).filter((f) => {
      const ext = "." + f.name.split(".").pop()?.toLowerCase();
      return [".docx", ".odt", ".doc"].includes(ext) && f.size <= MAX_SIZE;
    });
    setFiles(arr);
    setUploadState({ status: "idle" });
  }, []);

  const onDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(true);
  }, []);

  const onDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
  }, []);

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    if (e.dataTransfer.files.length) {
      handleFiles(e.dataTransfer.files);
    }
  }, [handleFiles]);

  const onInputChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files?.length) {
      handleFiles(e.target.files);
    }
  }, [handleFiles]);

  // Загрузка файла на сервер.
  const handleUpload = useCallback(async () => {
    if (!files.length) return;
    const file = files[0]; // MVP: один файл за раз
    setUploading(true);
    setUploadState({ status: "uploading" });
    try {
      const result = await uploadAdminDocument(file);
      setUploadState({ status: "done", result });
      setFiles([]);
      await refreshHistory();
    } catch (err: unknown) {
      const msg = err instanceof ApiError ? err.message : "неизвестная ошибка";
      setUploadState({ status: "error", error: msg });
    } finally {
      setUploading(false);
    }
  }, [files, refreshHistory]);

  const statusLabel = (s: string) => {
    switch (s) {
      case "processing": return <Badge>обработка</Badge>;
      case "ingested": return <Badge tone="success">загружен</Badge>;
      case "pii_blocked": return <Badge tone="accent">заблокирован (ПДн)</Badge>;
      case "failed": return <Badge tone="accent">ошибка</Badge>;
      default: return <Badge>{s}</Badge>;
    }
  };

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} Б`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} КБ`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} МБ`;
  };

  const formatDate = (d: string) => {
    try {
      return new Date(d).toLocaleString("ru-RU", {
        day: "2-digit", month: "2-digit", year: "numeric",
        hour: "2-digit", minute: "2-digit",
      });
    } catch { return d; }
  };

  return (
    <div className="admin-page">
      <h2 className="admin-page__title">Управление корпусом документов</h2>
      <p className="admin-page__subtitle">
        Загрузка документов (.docx, .odt, .doc) для индексации в RAG.
        Файлы анонимизируются перед ingest — персональные данные не попадают в векторную базу.
      </p>

      {/* ─── Drag-and-drop зона ──────────────────────────────── */}
      <div
        className={`admin-dropzone${dragOver ? " admin-dropzone--active" : ""}`}
        onDragOver={onDragOver}
        onDragLeave={onDragLeave}
        onDrop={onDrop}
        onClick={() => inputRef.current?.click()}
        role="button"
        tabIndex={0}
        aria-label="Зона загрузки документов"
      >
        <input
          ref={inputRef}
          type="file"
          accept={ACCEPT}
          onChange={onInputChange}
          style={{ display: "none" }}
        />
        <div className="admin-dropzone__icon">📄</div>
        <div className="admin-dropzone__text">
          {files.length
            ? <><strong>{files[0].name}</strong> ({formatSize(files[0].size)})</>
            : <>Перетащите файл сюда или <span className="accent">выберите</span></>
          }
        </div>
        <div className="admin-dropzone__hint">
          .docx · .odt · .doc — до 15 МБ
        </div>
      </div>

      {/* Кнопка загрузки */}
      {files.length > 0 && uploadState.status !== "uploading" && (
        <div style={{ marginTop: 16 }}>
          <Button variant="primary" onClick={handleUpload} loading={uploading} disabled={uploading}>
            Загрузить и обработать
          </Button>
        </div>
      )}

      {/* ─── Прогресс / результат загрузки ────────────────────── */}
      {uploadState.status === "uploading" && (
        <div className="admin-upload-status">
          <Spinner size="lg" />
          <span>Файл обрабатывается… (извлечение → анонимизация → чанкинг → эмбеддинг)</span>
        </div>
      )}

      {uploadState.status === "done" && uploadState.result && (
        <div className="admin-upload-status admin-upload-status--success">
          <div className="admin-upload-status__title">✓ Документ загружен</div>
          <div className="admin-upload-status__details">
            <div>Статус: <strong>{uploadState.result.status}</strong></div>
            <div>Чанков создано: <strong>{uploadState.result.chunks_count}</strong></div>
            <div>Анонимизировано сущностей: <strong>{uploadState.result.anonymizer_removed_count}</strong></div>
            {uploadState.result.removed_by_type && Object.keys(uploadState.result.removed_by_type).length > 0 && (
              <div>По типам: {Object.entries(uploadState.result.removed_by_type).map(([k, v]) => (
                <Badge key={k} mono>{k}: {String(v)}</Badge>
              ))}</div>
            )}
            {uploadState.result.qdrant_ids?.length > 0 && (
              <div>
                ID в Qdrant:{" "}
                <code className="admin-mono">{uploadState.result.qdrant_ids.slice(0, 3).join(", ")}{uploadState.result.qdrant_ids.length > 3 ? "…" : ""}</code>
              </div>
            )}
            {uploadState.result.error_message && (
              <div className="admin-error">Ошибка: {uploadState.result.error_message}</div>
            )}
          </div>
        </div>
      )}

      {uploadState.status === "error" && (
        <div className="admin-upload-status admin-upload-status--error">
          <div className="admin-upload-status__title">⚠ Ошибка загрузки</div>
          <div>{uploadState.error}</div>
        </div>
      )}

      {/* ─── История загрузок ──────────────────────────────────── */}
      <div className="admin-history">
        <h3 className="admin-history__title">История загрузок</h3>
        {loadingHistory ? (
          <div className="admin-history__loading"><Spinner /></div>
        ) : history.length === 0 ? (
          <div className="admin-history__empty">Пока нет загруженных документов</div>
        ) : (
          <div className="admin-history__list">
            {history.map((doc) => (
              <div key={doc.id} className="admin-history__item">
                <div className="admin-history__name">{doc.original_filename}</div>
                <div className="admin-history__meta">
                  <span>{statusLabel(doc.status)}</span>
                  <span className="admin-history__date">{formatDate(doc.created_at)}</span>
                  {doc.chunks_count > 0 && (
                    <span className="admin-history__chunks">{doc.chunks_count} чанков</span>
                  )}
                  {doc.anonymizer_removed_count > 0 && (
                    <span className="admin-history__anon">−{doc.anonymizer_removed_count} ПДн</span>
                  )}
                </div>
                {doc.error_message && (
                  <div className="admin-history__error">{doc.error_message}</div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
