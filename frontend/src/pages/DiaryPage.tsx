// DiaryPage (/diary) — поток генерации дневников (docs/08 §5).
// Шаги: выбор типа документа → опросник (по схеме gateway) → генерация →
// результат. Этап 6: статичный/реактивный рендер схемы (условная логика —
// базовая, см. lib/questionnaire), POST /generate, лоадер, обработка ошибок.
import { useEffect, useMemo, useState } from "react";
import { friendlyError } from "../api/errors";
import {
  useDocumentTypes,
  useGenerate,
  useQuestionnaire,
} from "../api/queries";
import type { AnswerValue, Answers, GenerateResult } from "../api/types";
import { QuestionnaireRenderer } from "../components/questionnaire/QuestionnaireRenderer";
import { GenerationResult } from "../components/result/GenerationResult";
import { Banner, Button, EmptyState, Skeleton, Spinner } from "../components/ui";
import {
  buildDefaults,
  clearHiddenAnswers,
  computeProgress,
  computeVisibleIds,
  prepareAnswers,
} from "../lib/questionnaire";
import "./pages.css";

const DEFAULT_DOC_TYPE = "daily";

export function DiaryPage() {
  const docTypesQuery = useDocumentTypes();
  const [docType, setDocType] = useState<string>(DEFAULT_DOC_TYPE);
  const [answers, setAnswers] = useState<Answers>({});
  const [result, setResult] = useState<GenerateResult | null>(null);
  // Подсветка незаполненных обязательных после попытки отправки (docs/08 §5.1).
  const [showInvalid, setShowInvalid] = useState(false);

  const schemaQuery = useQuestionnaire(docType);
  const generateMutation = useGenerate();

  // При загрузке/смене схемы — выставляем дефолты и сбрасываем результат.
  useEffect(() => {
    if (schemaQuery.data) {
      setAnswers(buildDefaults(schemaQuery.data));
      setResult(null);
    }
  }, [schemaQuery.data]);

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
      // Меняем ответ и сразу чистим вопросы, ставшие невидимыми (docs/06 §3):
      // скрытые ответы не должны влиять на генерацию.
      const updated = { ...prev, [id]: value };
      const vis = computeVisibleIds(schema, updated);
      return clearHiddenAnswers(schema, updated, vis);
    });
  };

  const handleSelectType = (code: string) => {
    if (code === docType) return;
    setDocType(code);
    setAnswers({});
    setResult(null);
    setShowInvalid(false);
    generateMutation.reset();
  };

  const handleGenerate = () => {
    if (!schema || progress === null) return;
    if (progress.missingRequired.length > 0) {
      setShowInvalid(true);
      return;
    }
    const payload = prepareAnswers(schema, answers, visible);
    generateMutation.mutate(
      { document_type: docType, answers: payload },
      { onSuccess: (data) => setResult(data) },
    );
  };

  const invalidIds = useMemo(
    () =>
      showInvalid && progress
        ? new Set(progress.missingRequired)
        : new Set<string>(),
    [showInvalid, progress],
  );

  const canGenerate =
    Boolean(schema) && progress !== null && !generateMutation.isPending;

  // ─── Результат ───────────────────────────────────────────────────────────
  if (result) {
    return (
      <>
        <GenerationResult
          requestId={result.request_id}
          documentType={docType}
          documentTypes={docTypesQuery.data}
          content={result.content}
          status={result.status}
          anonymization={result.anonymization}
        />
        <div style={{ marginTop: 24 }}>
          <Button
            onClick={() => {
              setResult(null);
              generateMutation.reset();
            }}
          >
            ← Заполнить ещё раз
          </Button>
        </div>
      </>
    );
  }

  // ─── Генерация идёт ────────────────────────────────────────────────────────
  if (generateMutation.isPending) {
    return (
      <div className="generating">
        <Spinner size="lg" />
        <div className="generating__title">Генерируем дневник…</div>
        <div className="generating__hint">
          Модель формирует обезличенный текст по вашим ответам. Это может занять
          до минуты — пожалуйста, подождите.
        </div>
      </div>
    );
  }

  return (
    <>
      <div className="page-head">
        <h1 className="page-head__title">Новый дневник</h1>
        <p className="page-head__subtitle">
          Выберите тип документа и заполните короткий опросник — система
          сгенерирует обезличенный текст.
        </p>
      </div>

      {/* Выбор типа документа */}
      <DocumentTypeSwitcher
        active={docType}
        onSelect={handleSelectType}
        loading={docTypesQuery.isPending}
        types={docTypesQuery.data}
      />

      {/* Ошибка генерации (PII_DETECTED / LLM_UNAVAILABLE и пр.) */}
      {generateMutation.isError && (
        <div className="section">
          <GenerateError error={generateMutation.error} onRetry={handleGenerate} />
        </div>
      )}

      {/* Опросник */}
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

      {/* Нижняя панель: прогресс + submit */}
      {schema && schema.questions.length > 0 && progress && (
        <div className="form-footer">
          <span className="form-footer__progress">
            Отвечено <b>{progress.answered}</b> из {progress.total} обязательных
          </span>
          <Button
            variant="primary"
            size="lg"
            disabled={!canGenerate}
            onClick={handleGenerate}
          >
            Сгенерировать
          </Button>
        </div>
      )}
    </>
  );
}

// ─── DocumentTypeSwitcher (docs/08 §4.3) ──────────────────────────────────────
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
      ? types
      : [
          { code: "daily", title: "Ежедневный дневник", is_active: true },
          { code: "exam_10d", title: "Осмотр (раз в 10 дней)", is_active: true },
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
            {t.title}
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
