// BatchDiaryPage (/diary/batch) — пакетная генерация дневников за период.
// Одна запись в истории (пакет) + дочерние дни. Генерация в фоне.
import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { friendlyError } from "../api/errors";
import { deleteRequest } from "../api/endpoints";
import type { Answers } from "../api/types";
import { QuestionnaireRenderer } from "../components/questionnaire/QuestionnaireRenderer";
import { Banner, Button } from "../components/ui";
import {
  buildBatchPlan,
  buildGenerateAnswers,
  validateBatchDates,
} from "../lib/batchDiary";
import { compileArc } from "../lib/arcCompiler";
import { BATCH_QUESTIONNAIRE } from "../lib/batchQuestionnaire";
import { startBatchGeneration } from "../lib/generationRunner";
import type { EditDiaryState } from "../lib/historyTitles";
import {
  buildDefaults,
  computeProgress,
  computeVisibleIds,
  prepareAnswers,
} from "../lib/questionnaire";
import { DiaryNav } from "./DiaryNav";
import "./pages.css";

export function BatchDiaryPage() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const editState = (location.state as EditDiaryState | null) ?? null;
  const schema = BATCH_QUESTIONNAIRE;

  const [admissionDate, setAdmissionDate] = useState(
    editState?.batchMeta?.admission_date ?? "",
  );
  const [dateFrom, setDateFrom] = useState(editState?.batchMeta?.date_from ?? "");
  const [dateTo, setDateTo] = useState(editState?.batchMeta?.date_to ?? "");
  const [estimatedDischargeDate, setEstimatedDischargeDate] = useState(
    editState?.batchMeta?.estimated_discharge ?? "",
  );
  const [answers, setAnswers] = useState<Answers>(
    () => editState?.answers ?? buildDefaults(schema),
  );
  const [directorContext, setDirectorContext] = useState(
    editState?.batchMeta?.director_context ?? "",
  );
  const [replaceRequestId, setReplaceRequestId] = useState<string | undefined>(
    editState?.documentType === "batch" ? editState.requestId : undefined,
  );
  const [showInvalid, setShowInvalid] = useState(false);
  const [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState<unknown>(null);

  useEffect(() => {
    if (editState) {
      navigate(location.pathname, { replace: true, state: null });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

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
      directorContext,
      estimatedDischargeDate,
    });
  }, [
    admissionDate,
    dateFrom,
    dateTo,
    answers,
    directorContext,
    estimatedDischargeDate,
    dateValidation.ok,
  ]);

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
    !starting;

  const handleGenerate = async () => {
    if (!progress || !planPreview) return;
    if (progress.missingRequired.length > 0) {
      setShowInvalid(true);
      return;
    }
    if (!dateValidation.ok) return;

    setShowInvalid(false);
    setStarting(true);
    setStartError(null);

    const payload = prepareAnswers(schema, answers, visible);
    const totalDays = planPreview.days.length;
    const briefs = compileArc({
      days: planPreview.days,
      directorContext,
      batchAnswers: payload,
      estimatedDischargeDate,
    });
    const days = planPreview.days.map((plan, i) => ({
      dayNumber: plan.dayNumber,
      isoDate: plan.isoDate,
      documentType: plan.documentType,
      answers: buildGenerateAnswers(
        payload,
        plan.dayNumber,
        totalDays,
        plan.isoDate,
        directorContext,
        estimatedDischargeDate,
        plan.documentType,
        briefs[i],
      ),
    }));

    try {
      // При повторной генерации пакета удаляем старую запись (дети cascade),
      // чтобы не копились дубликаты дней.
      if (replaceRequestId) {
        try {
          await deleteRequest(replaceRequestId);
        } catch {
          /* если уже удалена — продолжаем */
        }
        setReplaceRequestId(undefined);
      }

      const parentId = await startBatchGeneration({
        qc,
        meta: {
          admission_date: admissionDate,
          date_from: dateFrom,
          date_to: dateTo,
          estimated_discharge: estimatedDischargeDate,
          director_context: directorContext,
        },
        narrativeAnswers: payload,
        days,
      });
      navigate(`/requests/${parentId}`);
    } catch (err) {
      setStartError(err);
      setStarting(false);
    }
  };

  return (
    <>
      <DiaryNav />

      <div className="page-head">
        <h1 className="page-head__title">
          {replaceRequestId
            ? "Редактирование пакета"
            : "Сформировать дневники за выбранный период"}
        </h1>
        <p className="page-head__subtitle">
          Опишите нарратив периода один раз — AI построит полный ряд дневников
          с правильной клинической динамикой. В истории появится одна запись
          пакета; внутри можно раскрыть каждый день.
        </p>
      </div>

      {startError != null && (
        <div className="section">
          <Banner
            tone={friendlyError(startError).tone}
            title={friendlyError(startError).title}
            text={friendlyError(startError).detail}
          />
        </div>
      )}

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
            label="Начало периода"
            required
            value={dateFrom}
            onChange={setDateFrom}
            min={admissionDate || undefined}
          />
          <DateField
            id="to"
            label="Конец периода"
            required
            value={dateTo}
            onChange={setDateTo}
            min={dateFrom || admissionDate || undefined}
          />
          <DateField
            id="discharge"
            label="Ориентировочная выписка"
            value={estimatedDischargeDate}
            onChange={setEstimatedDischargeDate}
            min={dateTo || dateFrom || admissionDate || undefined}
            help="Необязательно — задаёт темп улучшения"
          />
        </div>
        {datesComplete && !dateValidation.ok && (
          <Banner
            tone="warning"
            title="Проверьте даты"
            text={dateValidation.message}
          />
        )}
        {planPreview && (
          <p className="batch-preview">
            Будет сгенерировано <b>{planPreview.days.length}</b> записей:{" "}
            <b>{planPreview.dailyCount}</b> ежедневных,{" "}
            <b>{planPreview.examCount}</b>{" "}
            {planPreview.examCount === 1 ? "осмотр" : "осмотров"} за 10 дней.
          </p>
        )}
      </div>

      <div className="section">
        <span className="section__label">Дополнительный контекст</span>
        <label className="batch-context">
          <span className="batch-context__label">
            Установка для ИИ{" "}
            <span className="field__optional">необязательно</span>
          </span>
          <textarea
            className="field__textarea batch-context__area"
            rows={4}
            placeholder="Например: пациент поступил в тяжёлом состоянии, постепенно стабилизировался, к концу периода — устойчивая ремиссия."
            value={directorContext}
            onChange={(e) => setDirectorContext(e.target.value)}
          />
          <span className="field__help">
            Окно для ввода дополнительного контекста. Помогает ИИ лучше
            понимать индивидуальную историю пациента.{" "}
            <b>Не попадёт в текст дневников</b> — нужно только для формирования
            контекста.
          </span>
        </label>
      </div>

      <div className="section">
        <span className="section__label">Клинические параметры</span>
        <QuestionnaireRenderer
          schema={schema}
          answers={answers}
          onChange={(id, value) =>
            setAnswers((prev) => ({ ...prev, [id]: value }))
          }
          invalidIds={invalidIds}
        />
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
            loading={starting}
            onClick={() => void handleGenerate()}
          >
            {replaceRequestId ? "Сгенерировать пакет заново" : "Сгенерировать пакет"}
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
      {help ? (
        <span className="field__help">{help}</span>
      ) : (
        <span className="field__help" aria-hidden="true">
          &nbsp;
        </span>
      )}
    </label>
  );
}
