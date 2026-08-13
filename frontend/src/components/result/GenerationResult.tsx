// GenerationResult — экран результата генерации (docs/08 §5.3).
// Показывает: метаданные (модель/дата), плашку анонимизации, документ в
// моношрифте с подсветкой плейсхолдеров и панель действий.
// Этап 8: «Копировать текст» + рабочие «Скачать Word/PDF» (экспорт через
// gateway, docs/07 §7). Кнопки экспорта активны при наличии requestId
// (свежий результат уже сохранён /generate; в истории — request_id из записи).
import { useEffect, useState } from "react";
import { ApiError } from "../../api/errors";
import type {
  AnonymizationSummary,
  DocumentType,
  ExportFormat,
} from "../../api/types";
import { copyText } from "../../lib/clipboard";
import { omitEmptyDiarySections } from "../../lib/diaryMarkup";
import { downloadExport } from "../../lib/download";
import { buildExportSubstitutions } from "../../lib/exportSubstitutions";
import { documentTypeLabel, formatDateTime, statusLabel } from "../../lib/format";
import { useAuth } from "../../auth/AuthContext";
import { Badge, Button } from "../ui";
import { GenerationFeedback } from "../feedback/GenerationFeedback";
import { AnonymizationNotice } from "./AnonymizationNotice";
import { DocumentView } from "./DocumentView";
import "./result.css";

interface GenerationResultProps {
  /** request_id записи — нужен для экспорта (docs/07 §7). */
  requestId?: string;
  title?: string;
  documentType: string;
  documentTypes?: DocumentType[];
  content: string;
  status: string;
  llmModelUsed?: string;
  createdAt?: string;
  anonymization?: AnonymizationSummary;
  /** Открыть форму с исходными данными для повторной генерации. */
  onEdit?: () => void;
}

export function GenerationResult({
  requestId,
  title,
  documentType,
  documentTypes,
  content,
  status,
  llmModelUsed,
  createdAt,
  anonymization,
  onEdit,
}: GenerationResultProps) {
  const { doctor } = useAuth();
  const [toast, setToast] = useState<string | null>(null);
  // Какой формат сейчас экспортируется (для лоадера на конкретной кнопке).
  const [exporting, setExporting] = useState<ExportFormat | null>(null);

  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 2600);
    return () => clearTimeout(t);
  }, [toast]);

  const handleCopy = async () => {
    const ok = await copyText(omitEmptyDiarySections(content));
    setToast(ok ? "Текст скопирован" : "Не удалось скопировать");
  };

  const handleExport = async (format: ExportFormat) => {
    if (!requestId || exporting) return;
    setExporting(format);
    try {
      await downloadExport(requestId, {
        format,
        substitutions: buildExportSubstitutions({
          createdAt,
          doctorName: doctor?.display_name,
        }),
      });
      setToast("Файл сохранён");
    } catch (err) {
      const msg =
        err instanceof ApiError && err.code === "NOT_FOUND"
          ? "Запись не найдена — обновите страницу"
          : "Не удалось сформировать файл";
      setToast(msg);
    } finally {
      setExporting(null);
    }
  };

  const exportDisabled = !requestId;
  const exportTitle = exportDisabled
    ? "Экспорт доступен после сохранения результата"
    : undefined;

  return (
    <div className="result">
      <div className="result__header">
        <h2>{title ?? documentTypeLabel(documentType, documentTypes)}</h2>
        <div className="result__meta">
          <Badge tone="accent">
            {documentTypeLabel(documentType, documentTypes)}
          </Badge>
          {status && (
            <Badge tone={status === "done" ? "success" : "default"}>
              {statusLabel(status)}
            </Badge>
          )}
          {llmModelUsed && <Badge mono>{llmModelUsed}</Badge>}
          {createdAt && <span>{formatDateTime(createdAt)}</span>}
        </div>
      </div>

      {anonymization && <AnonymizationNotice summary={anonymization} />}

      <DocumentView content={content} />

      <div className="result__actions">
        <Button variant="primary" onClick={handleCopy}>
          Копировать текст
        </Button>
        <Button
          onClick={() => void handleExport("docx")}
          disabled={exportDisabled || exporting !== null}
          loading={exporting === "docx"}
          title={exportTitle}
        >
          Скачать Word
        </Button>
        <Button
          onClick={() => void handleExport("pdf")}
          disabled={exportDisabled || exporting !== null}
          loading={exporting === "pdf"}
          title={exportTitle}
        >
          Скачать PDF
        </Button>
        {onEdit && (
          <Button onClick={onEdit}>Редактировать данные</Button>
        )}
      </div>

      {toast && (
        <div className="toast" role="status">
          <span aria-hidden="true">✓</span>
          {toast}
        </div>
      )}

      {requestId && status === "done" && content && (
        <GenerationFeedback requestId={requestId} />
      )}
    </div>
  );
}
