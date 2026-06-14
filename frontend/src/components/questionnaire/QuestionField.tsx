// QuestionField — рендер одного вопроса опросника (docs/06 §3, docs/08 §5.1).
// Поддерживает select / multiselect / text / number / boolean и опцию
// «свой вариант» (allow_custom). Контролируемый компонент: значение и onChange
// приходят сверху (QuestionnaireRenderer держит общий стейт answers).
//
// Задел под Этап 7: компонент уже принимает `conditional`-флаг для смещённого
// рендера дочерних вопросов; более сложные кастом-виджеты (МКБ-поиск и т.п.)
// добавятся как новые ветки `type` без изменения контракта пропсов.
import type { AnswerValue, Question } from "../../api/types";
import { CUSTOM_VALUE, isCustomAnswer } from "../../lib/questionnaire";

interface QuestionFieldProps {
  question: Question;
  value: AnswerValue | undefined;
  onChange: (value: AnswerValue) => void;
  /** true → дочерний условный вопрос (смещённый рендер). */
  conditional?: boolean;
}

export function QuestionField({
  question,
  value,
  onChange,
  conditional = false,
}: QuestionFieldProps) {
  return (
    <div className={`field${conditional ? " field--conditional" : ""}`}>
      <label className="field__label" htmlFor={`q-${question.id}`}>
        {question.label}
        {question.required ? (
          <span className="field__required" aria-hidden="true">
            *
          </span>
        ) : (
          <span className="field__optional">необязательно</span>
        )}
      </label>
      <FieldControl question={question} value={value} onChange={onChange} />
    </div>
  );
}

function FieldControl({
  question,
  value,
  onChange,
}: Omit<QuestionFieldProps, "conditional">) {
  switch (question.type) {
    case "select":
      return <SelectField question={question} value={value} onChange={onChange} />;
    case "multiselect":
      return (
        <MultiSelectField question={question} value={value} onChange={onChange} />
      );
    case "boolean":
      return <BooleanField question={question} value={value} onChange={onChange} />;
    case "number":
      return <NumberField question={question} value={value} onChange={onChange} />;
    case "text":
    default:
      return <TextField question={question} value={value} onChange={onChange} />;
  }
}

// ─── select (+ свой вариант) ─────────────────────────────────────────────────
function SelectField({
  question,
  value,
  onChange,
}: Omit<QuestionFieldProps, "conditional">) {
  const custom = isCustomAnswer(value);
  const selected = custom ? CUSTOM_VALUE : typeof value === "string" ? value : "";

  return (
    <>
      <select
        id={`q-${question.id}`}
        className="field__control"
        value={selected}
        onChange={(e) => {
          const v = e.target.value;
          if (v === CUSTOM_VALUE) {
            onChange({ value: CUSTOM_VALUE, custom_text: "" });
          } else {
            onChange(v);
          }
        }}
      >
        {!question.default && <option value="">— выберите —</option>}
        {question.options?.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
        {question.allow_custom && (
          <option value={CUSTOM_VALUE}>＋ Свой вариант…</option>
        )}
      </select>

      {custom && (
        <div className="field__custom">
          <input
            className="field__input"
            type="text"
            autoFocus
            placeholder="Введите свой вариант"
            value={isCustomAnswer(value) ? value.custom_text : ""}
            onChange={(e) =>
              onChange({ value: CUSTOM_VALUE, custom_text: e.target.value })
            }
            aria-label={`${question.label}: свой вариант`}
          />
        </div>
      )}
    </>
  );
}

// ─── multiselect (чипы + опц. свой вариант) ──────────────────────────────────
function MultiSelectField({
  question,
  value,
  onChange,
}: Omit<QuestionFieldProps, "conditional">) {
  const selected = Array.isArray(value) ? value : [];

  const toggle = (optValue: string) => {
    const next = selected.includes(optValue)
      ? selected.filter((v) => v !== optValue)
      : [...selected, optValue];
    onChange(next);
  };

  return (
    <div className="chips" role="group" aria-label={question.label}>
      {question.options?.map((opt) => {
        const active = selected.includes(opt.value);
        return (
          <button
            key={opt.value}
            type="button"
            className={`chip${active ? " chip--active" : ""}`}
            aria-pressed={active}
            onClick={() => toggle(opt.value)}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}

// ─── text ─────────────────────────────────────────────────────────────────
function TextField({
  question,
  value,
  onChange,
}: Omit<QuestionFieldProps, "conditional">) {
  const text =
    typeof value === "string"
      ? value
      : isCustomAnswer(value)
        ? value.custom_text
        : "";
  return (
    <textarea
      id={`q-${question.id}`}
      className="field__textarea"
      placeholder="Введите текст (без персональных данных)"
      value={text}
      onChange={(e) => onChange(e.target.value)}
    />
  );
}

// ─── number ─────────────────────────────────────────────────────────────────
function NumberField({
  question,
  value,
  onChange,
}: Omit<QuestionFieldProps, "conditional">) {
  return (
    <input
      id={`q-${question.id}`}
      className="field__input"
      type="number"
      value={typeof value === "number" ? value : ""}
      onChange={(e) => {
        const v = e.target.value;
        onChange(v === "" ? null : Number(v));
      }}
    />
  );
}

// ─── boolean (да/нет) ─────────────────────────────────────────────────────────
function BooleanField({
  question,
  value,
  onChange,
}: Omit<QuestionFieldProps, "conditional">) {
  const bool = value === true;
  const isSet = typeof value === "boolean";
  return (
    <div
      className="toggle-group"
      role="group"
      aria-label={question.label}
    >
      <button
        type="button"
        className={`toggle-group__btn${isSet && bool ? " toggle-group__btn--active" : ""}`}
        aria-pressed={isSet && bool}
        onClick={() => onChange(true)}
      >
        Да
      </button>
      <button
        type="button"
        className={`toggle-group__btn${isSet && !bool ? " toggle-group__btn--active" : ""}`}
        aria-pressed={isSet && !bool}
        onClick={() => onChange(false)}
      >
        Нет
      </button>
    </div>
  );
}
