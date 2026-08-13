// RequestDetailPage (/requests/:id) — просмотр результата из истории.
// Пакет: одна запись + раскрываемый список дней. Pending: «Формируется…».
import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useAuth } from "../auth/AuthContext";
import { ApiError, friendlyError } from "../api/errors";
import {
  useDeleteRequest,
  useDocumentTypes,
  useRequestDetail,
} from "../api/queries";
import type { ExportFormat, HistoryChild } from "../api/types";
import { GenerationFeedback } from "../components/feedback/GenerationFeedback";
import { GenerationResult } from "../components/result/GenerationResult";
import { DocumentView } from "../components/result/DocumentView";
import { Banner, Badge, Button, Skeleton, Spinner } from "../components/ui";
import { downloadBatchExport, downloadExport } from "../lib/download";
import { buildExportSubstitutions } from "../lib/exportSubstitutions";
import {
  documentTypeLabel,
  isPendingStatus,
  statusLabel,
} from "../lib/format";
import {
  isGenerationRunning,
  resumeBatchGeneration,
  startSingleGeneration,
} from "../lib/generationRunner";
import {
  type EditDiaryState,
  unpackBatchMeta,
} from "../lib/historyTitles";
import "./pages.css";
import "../components/result/result.css";

export function RequestDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { doctor } = useAuth();
  const docTypesQuery = useDocumentTypes();
  const { data, isPending, isError, error, refetch } = useRequestDetail(id);
  const deleteMutation = useDeleteRequest();
  const [exporting, setExporting] = useState<ExportFormat | null>(null);
  const [toast, setToast] = useState<string | null>(null);
  const [resuming, setResuming] = useState(false);
  const [jobTick, setJobTick] = useState(0);

  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 2600);
    return () => clearTimeout(t);
  }, [toast]);

  const handleEdit = () => {
    if (!data) return;
    if (data.document_type === "batch") {
      const { answers, meta } = unpackBatchMeta(data.answers_anonymized);
      const state: EditDiaryState = {
        requestId: data.request_id,
        documentType: "batch",
        answers,
        batchMeta: meta ?? undefined,
      };
      navigate("/diary/batch", { state });
      return;
    }
    const state: EditDiaryState = {
      requestId: data.request_id,
      documentType: data.document_type,
      answers: data.answers_anonymized ?? {},
    };
    navigate("/diary", { state });
  };

  const handleDelete = () => {
    if (!data) return;
    const label =
      data.document_type === "batch"
        ? "Удалить весь пакет дневников из истории?"
        : "Удалить запись из истории?";
    if (!window.confirm(label)) return;
    deleteMutation.mutate(data.request_id, {
      onSuccess: () => navigate("/diary"),
    });
  };

  const doneChildIds =
    data?.children
      ?.filter((c) => c.status === "done" && c.content)
      .map((c) => c.request_id) ?? [];

  const handleBatchExport = async (format: ExportFormat) => {
    if (doneChildIds.length === 0 || exporting) return;
    setExporting(format);
    try {
      await downloadBatchExport({
        format,
        request_ids: doneChildIds,
        substitutions: buildExportSubstitutions({
          createdAt: data?.created_at,
          doctorName: doctor?.display_name,
        }),
      });
      setToast("Файл сохранён");
    } catch (err) {
      const msg =
        err instanceof ApiError && err.code === "NOT_FOUND"
          ? "Записи не найдены — обновите страницу"
          : "Не удалось сформировать файл";
      setToast(msg);
    } finally {
      setExporting(null);
    }
  };

  useEffect(() => {
    if (!id || !data || !isPendingStatus(data.status)) return;
    const t = window.setInterval(() => setJobTick((n) => n + 1), 2000);
    return () => window.clearInterval(t);
  }, [id, data]);

  const pending = data ? isPendingStatus(data.status) : false;
  const jobAlive = Boolean(id && isGenerationRunning(id) && jobTick >= 0);
  const isBatch = data?.document_type === "batch";
  const children = data?.children ?? [];

  return (
    <>
      <button className="back-link" onClick={() => navigate("/diary")}>
        ← К новому дневнику
      </button>

      {isPending && (
        <div className="page-skeleton">
          <Skeleton width="50%" height="28px" />
          <Skeleton width="240px" height="20px" />
          <Skeleton height="220px" radius="12px" />
        </div>
      )}

      {isError && (
        <Banner
          tone={friendlyError(error).tone}
          title={friendlyError(error).title}
          text={friendlyError(error).detail}
          action={
            <Button size="sm" onClick={() => void refetch()}>
              Повторить
            </Button>
          }
        />
      )}

      {data && pending && (
        <div className="generating-inline">
          <Spinner size="lg" />
          <div className="generating-inline__title">{data.title_safe}</div>
          <div className="generating-inline__hint">
            {jobAlive
              ? "Генерация идёт в этой вкладке. Можно открыть другой раздел — запись обновится сама."
              : "Похоже, генерация оборвалась (обновили страницу или перезапустился сервер). Готовые дни сохранятся — можно продолжить."}
          </div>
          {!jobAlive && (
            <div className="generating-inline__actions">
              <Button
                variant="primary"
                loading={resuming}
                disabled={resuming}
                onClick={() => {
                  if (!id) return;
                  setResuming(true);
                  const run = isBatch
                    ? resumeBatchGeneration({ qc, detail: data })
                    : startSingleGeneration({
                        qc,
                        documentType: data.document_type,
                        answers: data.answers_anonymized ?? {},
                        requestId: data.request_id,
                      });
                  void run.finally(() => setResuming(false));
                }}
              >
                Продолжить генерацию
              </Button>
              <Button onClick={handleEdit}>Изменить данные</Button>
            </div>
          )}
          {isBatch && children.length > 0 && (
            <BatchDaysList children={children} />
          )}
        </div>
      )}

      {data && !pending && isBatch && (
        <div className="result">
          <div className="result__header">
            <h2>{data.title_safe}</h2>
            <div className="result__meta">
              <Badge tone="accent">Пакет дневников</Badge>
              <Badge tone="success">{statusLabel(data.status)}</Badge>
              <span>{children.length} дней</span>
            </div>
          </div>

          <BatchDaysList children={children} />

          <div className="result__actions">
            <Button
              variant="primary"
              onClick={() => void handleBatchExport("docx")}
              disabled={doneChildIds.length === 0 || exporting !== null}
              loading={exporting === "docx"}
            >
              Экспорт в Word
            </Button>
            <Button
              onClick={() => void handleBatchExport("pdf")}
              disabled={doneChildIds.length === 0 || exporting !== null}
              loading={exporting === "pdf"}
            >
              Экспорт в PDF
            </Button>
            <Button onClick={handleEdit}>Редактировать данные</Button>
            <Button
              onClick={handleDelete}
              loading={deleteMutation.isPending}
              disabled={deleteMutation.isPending}
            >
              Удалить
            </Button>
          </div>

          {toast && (
            <div className="toast" role="status">
              <span aria-hidden="true">✓</span>
              {toast}
            </div>
          )}

          {data.status === "done" && (
            <GenerationFeedback requestId={data.request_id} />
          )}
        </div>
      )}

      {data && !pending && !isBatch && (
        <>
          <GenerationResult
            requestId={data.request_id}
            title={data.title_safe}
            documentType={data.document_type}
            documentTypes={docTypesQuery.data}
            content={data.content}
            status={data.status}
            llmModelUsed={data.llm_model_used}
            createdAt={data.created_at}
            anonymization={{
              removed_count: data.anonymizer_removed_count,
              removed_by_type: {},
            }}
            onEdit={handleEdit}
          />
          <div style={{ marginTop: 16 }}>
            <Button
              onClick={handleDelete}
              loading={deleteMutation.isPending}
              disabled={deleteMutation.isPending}
            >
              Удалить из истории
            </Button>
          </div>
        </>
      )}
    </>
  );
}

function BatchDaysList({ children }: { children: HistoryChild[] }) {
  const { doctor } = useAuth();
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const [exportingId, setExportingId] = useState<string | null>(null);
  const [toast, setToast] = useState<string | null>(null);

  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 2600);
    return () => clearTimeout(t);
  }, [toast]);

  const sorted = useMemo(
    () => [...children].sort((a, b) => a.created_at.localeCompare(b.created_at)),
    [children],
  );

  if (sorted.length === 0) return null;

  const handleDayExport = async (child: HistoryChild, format: ExportFormat) => {
    if (!child.content || exportingId) return;
    setExportingId(`${child.request_id}:${format}`);
    try {
      await downloadExport(child.request_id, {
        format,
        substitutions: buildExportSubstitutions({
          title: child.title_safe,
          createdAt: child.created_at,
          doctorName: doctor?.display_name,
        }),
      });
      setToast("Файл сохранён");
    } catch {
      setToast("Не удалось сформировать файл");
    } finally {
      setExportingId(null);
    }
  };

  return (
    <div className="batch-days" aria-label="Дни пакета">
      {sorted.map((child) => {
        const expanded = Boolean(open[child.request_id]);
        const childPending = isPendingStatus(child.status);
        const canExport = child.status === "done" && Boolean(child.content);
        return (
          <div key={child.request_id} className="batch-day">
            <button
              type="button"
              className="batch-day__head"
              aria-expanded={expanded}
              onClick={() =>
                setOpen((prev) => ({
                  ...prev,
                  [child.request_id]: !prev[child.request_id],
                }))
              }
            >
              <span className="batch-day__title">{child.title_safe}</span>
              <span className="batch-day__meta">
                <Badge>{documentTypeLabel(child.document_type)}</Badge>
                <span>{statusLabel(child.status)}</span>
                <span aria-hidden="true">{expanded ? "▾" : "▸"}</span>
              </span>
            </button>
            {expanded && (
              <div className="batch-day__body">
                {childPending && (
                  <p className="generating-inline__hint">Ещё формируется…</p>
                )}
                {!childPending && child.content && (
                  <>
                    <DocumentView
                      content={child.content}
                      title={child.title_safe}
                      createdAt={child.created_at}
                    />
                    <div className="result__actions" style={{ marginTop: 12 }}>
                      <Button
                        size="sm"
                        onClick={() => void handleDayExport(child, "docx")}
                        disabled={!canExport || exportingId !== null}
                        loading={exportingId === `${child.request_id}:docx`}
                      >
                        Word
                      </Button>
                      <Button
                        size="sm"
                        onClick={() => void handleDayExport(child, "pdf")}
                        disabled={!canExport || exportingId !== null}
                        loading={exportingId === `${child.request_id}:pdf`}
                      >
                        PDF
                      </Button>
                    </div>
                    {child.status === "done" && (
                      <GenerationFeedback requestId={child.request_id} compact />
                    )}
                  </>
                )}
                {!childPending && !child.content && (
                  <p className="generating-inline__hint">
                    Текст для этого дня отсутствует.
                  </p>
                )}
              </div>
            )}
          </div>
        );
      })}
      {toast && (
        <div className="toast" role="status" style={{ marginTop: 8 }}>
          <span aria-hidden="true">✓</span>
          {toast}
        </div>
      )}
    </div>
  );
}
