// GenerationResult — экран результата генерации (docs/08 §5.3).
// Показывает: метаданные (модель/дата), плашку анонимизации, документ в
// моношрифте с подсветкой плейсхолдеров и панель действий.
// Этап 6: только «Копировать текст» (экспорт Word/PDF — Этап 8, заглушки).
import { useEffect, useState } from "react";
import type {
    AnonymizationSummary,
    DocumentType,
} from "../../api/types";
import { copyText } from "../../lib/clipboard";
import { documentTypeLabel, formatDateTime } from "../../lib/format";
import { Badge, Button } from "../ui";
import { AnonymizationNotice } from "./AnonymizationNotice";
import { DocumentView } from "./DocumentView";
import "./result.css";

interface GenerationResultProps {
  title?: string;
  documentType: string;
  documentTypes?: DocumentType[];
  content: string;
  status: string;
  llmModelUsed?: string;
  createdAt?: string;
  anonymization?: AnonymizationSummary;
}

export function GenerationResult({
  title,
  documentType,
  documentTypes,
  content,
  status,
  llmModelUsed,
  createdAt,
  anonymization,
}: GenerationResultProps) {
  const [toast, setToast] = useState<string | null>(null);

  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 2200);
    return () => clearTimeout(t);
  }, [toast]);

  const handleCopy = async () => {
    const ok = await copyText(content);
    setToast(ok ? "Текст скопирован" : "Не удалось скопировать");
  };

  return (
    <div className="result">
      <div className="result__header">
        <h2>{title ?? documentTypeLabel(documentType, documentTypes)}</h2>
        <div className="result__meta">
          <Badge tone="accent">
            {documentTypeLabel(documentType, documentTypes)}
          </Badge>
          {status && <Badge tone={status === "done" ? "success" : "default"}>{status}</Badge>}
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
        {/* Экспорт — Этап 8. Кнопки-заглушки disabled, чтобы показать UX. */}
        <Button disabled title="Экспорт в Word — на этапе 8">
          Скачать Word
        </Button>
        <Button disabled title="Экспорт в PDF — на этапе 8">
          Скачать PDF
        </Button>
      </div>

      {toast && (
        <div className="toast" role="status">
          <span aria-hidden="true">✓</span>
          {toast}
        </div>
      )}
    </div>
  );
}
