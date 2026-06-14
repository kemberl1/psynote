// RequestDetailPage (/requests/:id) — просмотр прошлого результата из истории
// (docs/08 §4.2 «Состояние Результат»). Показывает обезличенный дневник,
// метаданные (модель/дата), обезличенные ответы и аудит-счётчик удалённых ПДн.
import { useNavigate, useParams } from "react-router-dom";
import { friendlyError } from "../api/errors";
import { useDocumentTypes, useRequestDetail } from "../api/queries";
import type { Answers } from "../api/types";
import { GenerationResult } from "../components/result/GenerationResult";
import { Banner, Button, Skeleton } from "../components/ui";
import { isCustomAnswer } from "../lib/questionnaire";
import "./pages.css";

export function RequestDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const docTypesQuery = useDocumentTypes();
  const { data, isPending, isError, error, refetch } = useRequestDetail(id);

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

      {data && (
        <>
          <GenerationResult
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
          />

          {data.answers_anonymized &&
            Object.keys(data.answers_anonymized).length > 0 && (
              <div className="section">
                <span className="section__label">
                  Ответы опросника (обезличенные)
                </span>
                <AnswersTable answers={data.answers_anonymized} />
              </div>
            )}
        </>
      )}
    </>
  );
}

// Таблица обезличенных ответов (только метаданные/коды, без ПДн — бэкенд уже
// отдаёт обезличенные значения, docs/05).
function AnswersTable({ answers }: { answers: Answers }) {
  const rows = Object.entries(answers);
  return (
    <div className="answers">
      {rows.map(([key, value]) => (
        <div key={key} className="answers__row">
          <span className="answers__key">{key}</span>
          <span className="answers__value">{renderValue(value)}</span>
        </div>
      ))}
    </div>
  );
}

function renderValue(value: Answers[string]): string {
  if (value === null || value === undefined) return "—";
  if (Array.isArray(value)) return value.join(", ");
  if (isCustomAnswer(value)) return value.custom_text || "(свой вариант)";
  if (typeof value === "boolean") return value ? "да" : "нет";
  return String(value);
}
