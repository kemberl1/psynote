// QuestionField — рендер одного вопроса опросника (docs/06 §3, docs/08 §5.1).
// Поддерживает select / multiselect / text / number / boolean и опцию «свой
// вариант» (allow_custom) — в т.ч. внутри multiselect (Этап 7). Контролируемый
// компонент: значение и onChange приходят сверху.
//
// Приватность (docs/06 §1.4, docs/09): свободный ввод анонимизируется на
// gateway — на фронте только подсказка «без персональных данных», ничего не
// хранится локально сверх стейта формы.
import type {
  AnswerValue,
  CustomAnswer,
  MultiAnswerItem,
  Question,
} from "../../api/types";
import {
  CUSTOM_VALUE,
  customItemText,
  hasCustomItem,
  isCustomAnswer,
} from "../../lib/questionnaire";

interface QuestionFieldProps {
  question: Question;
  value: AnswerValue | undefined;
  onChange: (value: AnswerValue) => void;
  /** true → дочерний условный вопрос (смещённый рендер). */
  conditional?: boolean;
  /** true → подсветить как обязательный незаполненный (валидация перед отправкой). */
  invalid?: boolean;
}

export function QuestionField({
  question,
  value,
  onChange,
  conditional = false,
  invalid = false,
}: QuestionFieldProps) {
  const cls = [
    "field",
    conditional ? "field--conditional" : "",
    invalid ? "field--invalid" : "",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <div className={cls}>
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
      {question.help && <p className="field__help">{question.help}</p>}
      {invalid && (
        <p className="field__error" role="alert">
          Обязательный вопрос — выберите или укажите значение.
        </p>
      )}
    </div>
  );
}

type ControlProps = Pick<QuestionFieldProps, "question" | "value" | "onChange">;

function FieldControl({ question, value, onChange }: ControlProps) {
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
function SelectField({ question, value, onChange }: ControlProps) {
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
        <CustomTextInput
          label={question.label}
          value={isCustomAnswer(value) ? value.custom_text : ""}
          onChange={(text) =>
            onChange({ value: CUSTOM_VALUE, custom_text: text })
          }
        />
      )}
    </>
  );
}

// ─── multiselect (чипы + опц. свой вариант) ──────────────────────────────────
function MultiSelectField({ question, value, onChange }: ControlProps) {
  const items: MultiAnswerItem[] = Array.isArray(value) ? value : [];
  const codes = items.filter((i): i is string => typeof i === "string");
  const customActive = hasCustomItem(value);

  const setItems = (codeList: string[], custom?: CustomAnswer) => {
    const next: MultiAnswerItem[] = [...codeList];
    if (custom) next.push(custom);
    onChange(next);
  };

  const toggle = (optValue: string) => {
    const nextCodes = codes.includes(optValue)
      ? codes.filter((v) => v !== optValue)
      : [...codes, optValue];
    const existingCustom = items.find((i) => isCustomAnswer(i)) as
      | CustomAnswer
      | undefined;
    setItems(nextCodes, existingCustom);
  };

  const toggleCustom = () => {
    if (customActive) {
      setItems(codes); // убрать кастом
    } else {
      setItems(codes, { value: CUSTOM_VALUE, custom_text: "" });
    }
  };

  const updateCustomText = (text: string) => {
    setItems(codes, { value: CUSTOM_VALUE, custom_text: text });
  };

  return (
    <>
      <div className="chips" role="group" aria-label={question.label}>
        {question.options?.map((opt) => {
          const active = codes.includes(opt.value);
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
        {question.allow_custom && (
          <button
            type="button"
            className={`chip chip--custom${customActive ? " chip--active" : ""}`}
            aria-pressed={customActive}
            onClick={toggleCustom}
          >
            ＋ Свой вариант
          </button>
        )}
      </div>

      {customActive && (
        <CustomTextInput
          label={question.label}
          value={customItemText(value)}
          onChange={updateCustomText}
        />
      )}
    </>
  );
}

// ─── Инлайн-поле «свой вариант» (общее для select/multiselect) ───────────────
function CustomTextInput({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (text: string) => void;
}) {
  return (
    <div className="field__custom">
      <input
        className="field__input"
        type="text"
        autoFocus
        placeholder="Свой вариант (без персональных данных)"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-label={`${label}: свой вариант`}
      />
    </div>
  );
}

// ─── text ─────────────────────────────────────────────────────────────────
function TextField({ question, value, onChange }: ControlProps) {
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
function NumberField({ question, value, onChange }: ControlProps) {
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
function BooleanField({ question, value, onChange }: ControlProps) {
  const bool = value === true;
  const isSet = typeof value === "boolean";
  return (
    <div className="toggle-group" role="group" aria-label={question.label}>
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
