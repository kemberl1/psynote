// DiaryPage (/diary) — поток генерации дневников (docs/08 §5).
// При старте сразу создаётся запись «Формируется…» в истории; генерация
// продолжается в фоне — можно уйти на другой экран.
import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { friendlyError } from "../api/errors";
import { useDocumentTypes, useQuestionnaire } from "../api/queries";
import type { AnswerValue, Answers } from "../api/types";
import { QuestionnaireRenderer } from "../components/questionnaire/QuestionnaireRenderer";
import { Banner, Button, EmptyState, Skeleton } from "../components/ui";
import { startSingleGeneration } from "../lib/generationRunner";
import type { EditDiaryState } from "../lib/historyTitles";
import {
  buildDefaults,
  clearHiddenAnswers,
  computeProgress,
  computeVisibleIds,
  prepareAnswers,
} from "../lib/questionnaire";
import { DiaryNav } from "./DiaryNav";
import "./pages.css";

const DEFAULT_DOC_TYPE = "daily";

export function DiaryPage() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const editState = (location.state as EditDiaryState | null) ?? null;

  const docTypesQuery = useDocumentTypes();
  const [docType, setDocType] = useState<string>(
    editState?.documentType && editState.documentType !== "batch"
      ? editState.documentType
      : DEFAULT_DOC_TYPE,
  );
  const [answers, setAnswers] = useState<Answers>(editState?.answers ?? {});
  const [editRequestId, setEditRequestId] = useState<string | undefined>(
    editState?.documentType !== "batch" ? editState?.requestId : undefined,
  );
  const [showInvalid, setShowInvalid] = useState(false);
  const [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState<unknown>(null);

  const schemaQuery = useQuestionnaire(docType);

  // Сброс edit-state из history после применения, чтобы refresh не залипал.
  useEffect(() => {
    if (editState) {
      navigate(location.pathname, { replace: true, state: null });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // При загрузке/смене схемы — дефолты, если нет edit-ответов.
  useEffect(() => {
    if (schemaQuery.data && !editRequestId) {
      setAnswers((prev) =>
        Object.keys(prev).length > 0 ? prev : buildDefaults(schemaQuery.data!),
      );
    }
  }, [schemaQuery.data, editRequestId]);

  const schema = schemaQuery.data;

  const { visible, progress } = useMemo(() => {
    if (!schema) {
      return { visible: new Set<string>(), progress: null };
    }
    const vis = computeVisibleIds(schema, answers);
    return { visible: vis, progress: computeProgress(schema, answers, vis) };
  }, [schema, answers]);

  const handleChange = (id: string, value: AnswerValue) => {
    setAnswers((prev) => {
      if (!schema) return { ...prev, [id]: value };
      const updated = { ...prev, [id]: value };
      const vis = computeVisibleIds(schema, updated);
      return clearHiddenAnswers(schema, updated, vis);
    });
  };

  const handleSelectType = (code: string) => {
    if (code === docType) return;
    setDocType(code);
    setAnswers({});
    setEditRequestId(undefined);
    setShowInvalid(false);
    setStartError(null);
  };

  const handleGenerate = async () => {
    if (!schema || progress === null) return;
    if (progress.missingRequired.length > 0) {
      setShowInvalid(true);
      return;
    }
    const payload = prepareAnswers(schema, answers, visible);
    setStarting(true);
    setStartError(null);
    try {
      const requestId = await startSingleGeneration({
        qc,
        documentType: docType,
        answers: payload,
        requestId: editRequestId,
      });
      navigate(`/requests/${requestId}`);
    } catch (err) {
      setStartError(err);
      setStarting(false);
    }
  };

  const invalidIds = useMemo(
    () =>
      showInvalid && progress
        ? new Set(progress.missingRequired)
        : new Set<string>(),
    [showInvalid, progress],
  );

  const canGenerate =
    Boolean(schema) && progress !== null && !starting;

  return (
    <>
      <DiaryNav />
      <div className="page-head">
        <h1 className="page-head__title">
          {editRequestId ? "Редактирование дневника" : "Новый дневник"}
        </h1>
        <p className="page-head__subtitle">
          Выберите тип документа и заполните короткий опросник — система
          сгенерирует обезличенный текст. После запуска запись сразу появится в
          истории со статусом «Формируется…».
        </p>
      </div>

      <DocumentTypeSwitcher
        active={docType}
        onSelect={handleSelectType}
        loading={docTypesQuery.isPending}
        types={docTypesQuery.data}
      />

      {startError != null && (
        <div className="section">
          <GenerateError error={startError} onRetry={() => void handleGenerate()} />
        </div>
      )}

      <div className="section">
        <span className="section__label">Опросник</span>

        {schemaQuery.isPending && <FormSkeleton />}

        {schemaQuery.isError && (
          <Banner
            tone="danger"
            title={friendlyError(schemaQuery.error).title}
            text={friendlyError(schemaQuery.error).detail}
            action={
              <Button size="sm" onClick={() => void schemaQuery.refetch()}>
                Повторить
              </Button>
            }
          />
        )}

        {schema && schema.questions.length === 0 && (
          <EmptyState
            title="Опросник пуст"
            text="Для этого типа документа схема пока не настроена."
          />
        )}

        {schema && schema.questions.length > 0 && (
          <QuestionnaireRenderer
            schema={schema}
            answers={answers}
            onChange={handleChange}
            invalidIds={invalidIds}
          />
        )}
      </div>

      {schema && schema.questions.length > 0 && progress && (
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
            {editRequestId ? "Сгенерировать заново" : "Сгенерировать"}
          </Button>
        </div>
      )}
    </>
  );
}

function DocumentTypeSwitcher({
  active,
  onSelect,
  loading,
  types,
}: {
  active: string;
  onSelect: (code: string) => void;
  loading: boolean;
  types?: { code: string; title: string; is_active: boolean }[];
}) {
  if (loading) {
    return <Skeleton width="280px" height="44px" radius="12px" />;
  }
  const list =
    types && types.length > 0
      ? types.filter((t) => t.code !== "batch")
      : [
          { code: "daily", title: "Ежедневный осмотр", is_active: true },
          { code: "exam_10d", title: "Осмотр за 10 дней", is_active: true },
        ];
  return (
    <div className="doctype" role="tablist" aria-label="Тип документа">
      {list
        .filter((t) => t.is_active)
        .map((t) => (
          <button
            key={t.code}
            type="button"
            role="tab"
            aria-selected={t.code === active}
            className={`doctype__btn${t.code === active ? " doctype__btn--active" : ""}`}
            onClick={() => onSelect(t.code)}
          >
            {t.code === "daily"
              ? "Ежедневный осмотр"
              : t.code === "exam_10d"
                ? "Осмотр за 10 дней"
                : t.title}
          </button>
        ))}
    </div>
  );
}

function GenerateError({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry: () => void;
}) {
  const f = friendlyError(error);
  return (
    <Banner
      tone={f.tone}
      title={f.title}
      text={f.detail}
      action={
        f.tone === "danger" ? (
          <Button size="sm" onClick={onRetry}>
            Повторить
          </Button>
        ) : undefined
      }
    />
  );
}

function FormSkeleton() {
  return (
    <div className="page-skeleton">
      {Array.from({ length: 5 }).map((_, i) => (
        <div key={i} style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          <Skeleton width="160px" height="14px" />
          <Skeleton height="40px" radius="8px" />
        </div>
      ))}
    </div>
  );
}
