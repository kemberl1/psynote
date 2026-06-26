// BatchDiaryPage (/diary/batch) — пакетная генерация дневников за период.
// Сжатый опросник + дата поступления → для каждого дня POST /generate;
// дни 10/20/30… автоматически получают шаблон exam_10d.
import { useAuth } from "../auth/AuthContext";
import { useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { friendlyError } from "../api/errors";
import { generate } from "../api/endpoints";
import type { Answers, ExportFormat, GenerateResult } from "../api/types";
import { QuestionnaireRenderer } from "../components/questionnaire/QuestionnaireRenderer";
import { DocumentView } from "../components/result/DocumentView";
import { Banner, Button, EmptyState, Spinner } from "../components/ui";
import { downloadBatchExport } from "../lib/download";
import { buildExportSubstitutions } from "../lib/exportSubstitutions";
import {
  buildBatchPlan,
  buildGenerateAnswers,
  type BatchDayPlan,
  validateBatchDates,
} from "../lib/batchDiary";
import { BATCH_QUESTIONNAIRE } from "../lib/batchQuestionnaire";
import { documentTypeLabel } from "../lib/format";
import {
  buildDefaults,
  computeProgress,
  computeVisibleIds,
  prepareAnswers,
} from "../lib/questionnaire";
import { DiaryNav } from "./DiaryNav";
import "./pages.css";

type DayStatus = "pending" | "running" | "done" | "error";

interface DayResult {
  plan: BatchDayPlan;
  status: DayStatus;
  result?: GenerateResult;
  error?: string;
}

export function BatchDiaryPage() {
  const qc = useQueryClient();
  const schema = BATCH_QUESTIONNAIRE;

  const [admissionDate, setAdmissionDate] = useState("");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [answers, setAnswers] = useState<Answers>(() => buildDefaults(schema));
  const [freeContext, setFreeContext] = useState("");
  const [showInvalid, setShowInvalid] = useState(false);

  const [running, setRunning] = useState(false);
  const [dayResults, setDayResults] = useState<DayResult[] | null>(null);
  const [selectedIdx, setSelectedIdx] = useState(0);

  const visible = useMemo(
    () => computeVisibleIds(schema, answers),
    [schema, answers],
  );
  const progress = useMemo(
    () => computeProgress(schema, answers, visible),
    [schema, answers, visible],
  );

  const dateValidation = useMemo(
    () => validateBatchDates(admissionDate, dateFrom, dateTo),
    [admissionDate, dateFrom, dateTo],
  );

  const planPreview = useMemo(() => {
    if (!dateValidation.ok) return null;
    return buildBatchPlan({
      admissionDate,
      dateFrom,
      dateTo,
      answers,
      freeContext,
    });
  }, [admissionDate, dateFrom, dateTo, answers, freeContext, dateValidation.ok]);

  const invalidIds = useMemo(
    () =>
      showInvalid && progress
        ? new Set(progress.missingRequired)
        : new Set<string>(),
    [showInvalid, progress],
  );

  const datesComplete =
    Boolean(admissionDate) && Boolean(dateFrom) && Boolean(dateTo);

  const canGenerate =
    datesComplete &&
    dateValidation.ok &&
    progress !== null &&
    progress.missingRequired.length === 0 &&
    !running;

  const handleGenerate = async () => {
    if (!progress || !planPreview) return;
    if (progress.missingRequired.length > 0) {
      setShowInvalid(true);
      return;
    }
    if (!dateValidation.ok) return;

    setShowInvalid(false);
    setRunning(true);

    const payload = prepareAnswers(schema, answers, visible);
    const initial: DayResult[] = planPreview.days.map((plan) => ({
      plan,
      status: "pending",
    }));
    setDayResults(initial);
    setSelectedIdx(0);

    for (let i = 0; i < planPreview.days.length; i++) {
      const plan = planPreview.days[i];
      setDayResults((prev) =>
        prev?.map((r, idx) =>
          idx === i ? { ...r, status: "running" } : r,
        ) ?? null,
      );

      try {
        const genAnswers = buildGenerateAnswers(
          payload,
          plan.dayNumber,
          freeContext,
          plan.documentType,
        );
        const result = await generate({
          document_type: plan.documentType,
          answers: genAnswers,
        });
        setDayResults((prev) =>
          prev?.map((r, idx) =>
            idx === i ? { ...r, status: "done", result } : r,
          ) ?? null,
        );
      } catch (err) {
        const f = friendlyError(err);
        setDayResults((prev) =>
          prev?.map((r, idx) =>
            idx === i
              ? { ...r, status: "error", error: f.detail || f.title }
              : r,
          ) ?? null,
        );
      }
    }

    setRunning(false);
    void qc.invalidateQueries({ queryKey: ["requests"] });
  };

  const handleReset = () => {
    setDayResults(null);
    setSelectedIdx(0);
  };

  const doneCount =
    dayResults?.filter((r) => r.status === "done").length ?? 0;
  const errorCount =
    dayResults?.filter((r) => r.status === "error").length ?? 0;

  if (dayResults && !running && doneCount + errorCount === dayResults.length) {
    return (
      <>
        <DiaryNav />
        <BatchResultView
          results={dayResults}
          selectedIdx={selectedIdx}
          onSelect={setSelectedIdx}
          onReset={handleReset}
        />
      </>
    );
  }

  if (running && dayResults) {
    const current = dayResults.find((r) => r.status === "running");
    const completed = dayResults.filter(
      (r) => r.status === "done" || r.status === "error",
    ).length;
    return (
      <>
        <DiaryNav />
        <div className="generating">
          <Spinner size="lg" />
          <div className="generating__title">
            Генерируем пакет… {completed} / {dayResults.length}
          </div>
          <div className="generating__hint">
            {current
              ? `День ${current.plan.dayNumber} — ${documentTypeLabel(current.plan.documentType)}`
              : "Завершаем последний дневник…"}
          </div>
          <BatchProgressList results={dayResults} compact />
        </div>
      </>
    );
  }

  return (
    <>
      <DiaryNav />

      <div className="page-head">
        <h1 className="page-head__title">Пакетная генерация</h1>
        <p className="page-head__subtitle">
          Заполните сжатый опросник один раз — система создаст дневники за каждый
          день периода. Дни 10, 20, 30… автоматически оформятся как осмотр раз в
          10 дней.
        </p>
      </div>

      <div className="section">
        <span className="section__label">Период и поступление</span>
        <div className="batch-dates">
          <DateField
            id="admission"
            label="Дата поступления"
            required
            value={admissionDate}
            onChange={setAdmissionDate}
            help="Обязательно — от неё считаются дни 10, 20, 30…"
          />
          <DateField
            id="from"
            label="С"
            required
            value={dateFrom}
            onChange={setDateFrom}
            min={admissionDate || undefined}
          />
          <DateField
            id="to"
            label="По"
            required
            value={dateTo}
            onChange={setDateTo}
            min={dateFrom || admissionDate || undefined}
          />
        </div>
        {datesComplete && !dateValidation.ok && (
          <Banner tone="warning" title="Проверьте даты" text={dateValidation.message} />
        )}
        {planPreview && (
          <p className="batch-preview">
            Будет сгенерировано <b>{planPreview.days.length}</b> записей:{" "}
            <b>{planPreview.dailyCount}</b> ежедневных,{" "}
            <b>{planPreview.examCount}</b> осмотров (10 дней).
          </p>
        )}
      </div>

      <div className="section">
        <span className="section__label">Сжатый опросник</span>
        <QuestionnaireRenderer
          schema={schema}
          answers={answers}
          onChange={(id, value) =>
            setAnswers((prev) => ({ ...prev, [id]: value }))
          }
          invalidIds={invalidIds}
        />
      </div>

      <div className="section">
        <span className="section__label">Дополнительный контекст</span>
        <label className="batch-context">
          <span className="batch-context__label">
            Свободный текст{" "}
            <span className="field__optional">необязательно</span>
          </span>
          <textarea
            className="field__textarea batch-context__area"
            rows={4}
            placeholder="Общий контекст периода: особенности наблюдения, семейная ситуация, договорённости… Без персональных данных."
            value={freeContext}
            onChange={(e) => setFreeContext(e.target.value)}
          />
          <span className="field__help">
            Будет добавлен к каждому дневнику. Не указывайте ФИО, точные даты и
            адреса.
          </span>
        </label>
      </div>

      {progress && (
        <div className="form-footer">
          <span className="form-footer__progress">
            Отвечено <b>{progress.answered}</b> из {progress.total} обязательных
          </span>
          <Button
            variant="primary"
            size="lg"
            disabled={!canGenerate}
            onClick={() => void handleGenerate()}
          >
            Сгенерировать пакет
          </Button>
        </div>
      )}
    </>
  );
}

function DateField({
  id,
  label,
  required,
  value,
  onChange,
  min,
  help,
}: {
  id: string;
  label: string;
  required?: boolean;
  value: string;
  onChange: (v: string) => void;
  min?: string;
  help?: string;
}) {
  return (
    <label className="batch-date-field" htmlFor={id}>
      <span className="batch-date-field__label">
        {label}
        {required && (
          <span className="field__required" aria-hidden="true">
            *
          </span>
        )}
      </span>
      <input
        id={id}
        type="date"
        className="field__input batch-date-field__input"
        value={value}
        min={min}
        onChange={(e) => onChange(e.target.value)}
        required={required}
      />
      {help && <span className="field__help">{help}</span>}
    </label>
  );
}

function BatchProgressList({
  results,
  compact = false,
}: {
  results: DayResult[];
  compact?: boolean;
}) {
  return (
    <ul className={`batch-progress${compact ? " batch-progress--compact" : ""}`}>
      {results.map((r) => (
        <li
          key={r.plan.isoDate}
          className={`batch-progress__item batch-progress__item--${r.status}`}
        >
          <span className="batch-progress__day">День {r.plan.dayNumber}</span>
          <span className="batch-progress__type">
            {documentTypeLabel(r.plan.documentType)}
          </span>
          <span className="batch-progress__status">
            {r.status === "pending" && "ожидание"}
            {r.status === "running" && "…"}
            {r.status === "done" && "✓"}
            {r.status === "error" && "✗"}
          </span>
        </li>
      ))}
    </ul>
  );
}

function BatchResultView({
  results,
  selectedIdx,
  onSelect,
  onReset,
}: {
  results: DayResult[];
  selectedIdx: number;
  onSelect: (idx: number) => void;
  onReset: () => void;
}) {
  const { doctor } = useAuth();
  const done = results.filter((r) => r.status === "done").length;
  const failed = results.filter((r) => r.status === "error").length;
  const selected = results[selectedIdx];
  const [exporting, setExporting] = useState<ExportFormat | null>(null);
  const [toast, setToast] = useState<string | null>(null);

  const doneIds = results
    .filter((r) => r.status === "done" && r.result?.request_id)
    .map((r) => r.result!.request_id);

  const canExport = doneIds.length > 0;

  const handleBatchExport = async (format: ExportFormat) => {
    if (!canExport || exporting) return;
    setExporting(format);
    try {
      await downloadBatchExport({
        format,
        request_ids: doneIds,
        substitutions: buildExportSubstitutions({ doctorName: doctor?.display_name }),
      });
      setToast("Файл сохранён");
    } catch {
      setToast("Не удалось сформировать файл");
    } finally {
      setExporting(null);
    }
  };

  return (
    <>
      <div className="page-head">
        <h1 className="page-head__title">Пакет готов</h1>
        <p className="page-head__subtitle">
          Сгенерировано {done} из {results.length}
          {failed > 0 ? `, ошибок: ${failed}` : ""}. Записи сохранены в истории.
        </p>
      </div>

      <div className="batch-result">
        <aside className="batch-result__list" aria-label="Список дней">
          {results.map((r, idx) => (
            <button
              key={r.plan.isoDate}
              type="button"
              className={`batch-result__item${
                idx === selectedIdx ? " batch-result__item--active" : ""
              } batch-result__item--${r.status}`}
              onClick={() => onSelect(idx)}
            >
              <span>
                День {r.plan.dayNumber} · {documentTypeLabel(r.plan.documentType)}
              </span>
              {r.status === "error" && (
                <span className="batch-result__err">ошибка</span>
              )}
            </button>
          ))}
        </aside>

        <div className="batch-result__content">
          {selected?.status === "done" && selected.result && (
            <DocumentView content={selected.result.content} />
          )}
          {selected?.status === "error" && (
            <EmptyState
              icon="⚠"
              title="Не удалось сгенерировать"
              text={selected.error ?? "Попробуйте повторить для этого дня вручную."}
            />
          )}
          {selected?.status === "pending" && (
            <EmptyState title="Не обработано" text="День не был сгенерирован." />
          )}
        </div>
      </div>

      <div style={{ marginTop: 24, display: "flex", gap: 12, flexWrap: "wrap", alignItems: "center" }}>
        <Button
          onClick={() => void handleBatchExport("docx")}
          disabled={!canExport || exporting !== null}
          loading={exporting === "docx"}
        >
          Экспорт в Word
        </Button>
        <Button
          onClick={() => void handleBatchExport("pdf")}
          disabled={!canExport || exporting !== null}
          loading={exporting === "pdf"}
        >
          Экспорт в PDF
        </Button>
        <Button onClick={onReset}>← Новый пакет</Button>
      </div>

      {toast && (
        <div className="toast" role="status" style={{ marginTop: 12 }}>
          <span aria-hidden="true">✓</span>
          {toast}
        </div>
      )}
    </>
  );
}
