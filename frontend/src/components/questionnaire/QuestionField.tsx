// QuestionField — рендер одного вопроса опросника (docs/06 §3, docs/08 §5.1).
// Поддерживает select / multiselect / text / number / boolean и опцию «свой
// вариант» (allow_custom) — в т.ч. НЕСКОЛЬКО своих вариантов внутри multiselect
// (Этап 7.1). Контролируемый компонент: значение и onChange приходят сверху.
//
// UX «своего варианта» (Этап 7.1):
//   • multiselect: кнопка «＋ Свой вариант» открывает инлайн-поле; ввод
//     подтверждается Enter или кнопкой «＋» → текст превращается в выбранный
//     чип-тег (как обычная опция), поле очищается и остаётся в фокусе для
//     серии. Каждый кастом-чип удаляется крестиком ×. Пустой ввод/дубликаты не
//     добавляются. Esc или клик вне — закрыть поле без добавления.
//   • select (одиночный): «Свой вариант» открывает поле; Enter/кнопка
//     подтверждают значение (показывается как выбранное, можно изменить/очистить).
//
// Приватность (docs/06 §1.4, docs/09): свободный ввод анонимизируется на
// gateway — на фронте только подсказка «без персональных данных», ничего не
// хранится локально сверх стейта формы.
import { useEffect, useRef, useState } from "react";
import type {
  AnswerValue,
  MultiAnswerItem,
  Question
} from "../../api/types";
import {
  addCustomItem,
  CUSTOM_VALUE,
  isCustomAnswer,
  multiCodes,
  multiCustomTexts,
  removeCustomItemAt,
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
  const customText = custom ? value.custom_text : "";
  const selected = custom ? CUSTOM_VALUE : typeof value === "string" ? value : "";

  // Поле редактирования открыто, пока кастом выбран, но ещё не подтверждён
  // (нет текста) ИЛИ пользователь явно нажал «изменить».
  const [editing, setEditing] = useState(custom && customText.length === 0);

  // Открыть поле, когда переключились на «Свой вариант» без текста.
  useEffect(() => {
    if (custom && customText.length === 0) setEditing(true);
  }, [custom, customText.length]);

  const confirm = (text: string) => {
    const trimmed = text.trim();
    if (!trimmed) {
      // Пустой ввод — сбрасываем выбор «свой вариант».
      onChange("");
      setEditing(false);
      return;
    }
    onChange({ value: CUSTOM_VALUE, custom_text: trimmed });
    setEditing(false);
  };

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
            setEditing(true);
          } else {
            onChange(v);
            setEditing(false);
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

      {custom && editing && (
        <CustomTextEditor
          label={question.label}
          initial={customText}
          onConfirm={confirm}
          onCancel={() => {
            // Esc/клик-вне без текста → снять выбор «своего варианта».
            if (!customText.trim()) onChange("");
            setEditing(false);
          }}
        />
      )}

      {custom && !editing && customText.trim() && (
        <div className="custom-chips">
          <span className="chip chip--active chip--tag">
            <span className="chip__text">{customText}</span>
            <button
              type="button"
              className="chip__edit"
              aria-label={`Изменить свой вариант: ${customText}`}
              onClick={() => setEditing(true)}
            >
              Изменить
            </button>
            <button
              type="button"
              className="chip__remove"
              aria-label={`Удалить свой вариант: ${customText}`}
              onClick={() => onChange("")}
            >
              ×
            </button>
          </span>
        </div>
      )}
    </>
  );
}

// ─── multiselect (чипы + опц. несколько своих вариантов) ─────────────────────
function MultiSelectField({ question, value, onChange }: ControlProps) {
  const codes = multiCodes(value);
  const customTexts = multiCustomTexts(value);
  const [adding, setAdding] = useState(false);

  const toggle = (optValue: string) => {
    const items: MultiAnswerItem[] = Array.isArray(value) ? [...value] : [];
    const next = codes.includes(optValue)
      ? items.filter((i) => !(typeof i === "string" && i === optValue))
      : [...items, optValue];
    onChange(next);
  };

  const addCustom = (text: string): boolean => {
    const before = customTexts.length;
    const next = addCustomItem(value, text);
    onChange(next);
    // Успех, если число кастомов выросло (не дубль/не пусто).
    return multiCustomTexts(next).length > before;
  };

  const removeCustom = (customIndex: number) => {
    onChange(removeCustomItemAt(value, customIndex));
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

        {/* Подтверждённые кастом-варианты — такие же выбранные чипы с ×. */}
        {customTexts.map((text, idx) => (
          <span key={`custom-${idx}`} className="chip chip--active chip--tag">
            <span className="chip__text">{text}</span>
            <button
              type="button"
              className="chip__remove"
              aria-label={`Удалить свой вариант: ${text}`}
              onClick={() => removeCustom(idx)}
            >
              ×
            </button>
          </span>
        ))}

        {question.allow_custom && !adding && (
          <button
            type="button"
            className="chip chip--custom"
            onClick={() => setAdding(true)}
          >
            ＋ Свой вариант
          </button>
        )}
      </div>

      {question.allow_custom && adding && (
        <CustomTextEditor
          label={question.label}
          initial=""
          keepOpenOnConfirm
          onConfirm={(text) => addCustom(text)}
          onCancel={() => setAdding(false)}
        />
      )}
    </>
  );
}

// ─── Инлайн-редактор «свой вариант» с явным подтверждением ───────────────────
// Поддерживает Enter/кнопку «＋» для подтверждения и Esc/клик-вне для отмены.
// При keepOpenOnConfirm (multiselect-серия) — после успешного добавления поле
// очищается и фокус возвращается для следующего ввода.
function CustomTextEditor({
  label,
  initial,
  onConfirm,
  onCancel,
  keepOpenOnConfirm = false,
}: {
  label: string;
  initial: string;
  /** Возвращает true, если значение принято (для серии). */
  onConfirm: (text: string) => boolean | void;
  onCancel: () => void;
  keepOpenOnConfirm?: boolean;
}) {
  const [text, setText] = useState(initial);
  const inputRef = useRef<HTMLInputElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);

  // Автофокус при открытии.
  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  // Клик вне поля — отмена (без добавления).
  useEffect(() => {
    const onDocPointer = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) {
        onCancel();
      }
    };
    document.addEventListener("mousedown", onDocPointer);
    return () => document.removeEventListener("mousedown", onDocPointer);
  }, [onCancel]);

  const submit = () => {
    const accepted = onConfirm(text);
    if (keepOpenOnConfirm) {
      // Серия: очищаем поле и возвращаем фокус (даже если дубль/пусто —
      // просто остаёмся готовы к новому вводу).
      if (accepted !== false) setText("");
      inputRef.current?.focus();
    }
  };

  return (
    <div className="field__custom" ref={wrapRef}>
      <div className="custom-input">
        <input
          ref={inputRef}
          className="field__input custom-input__field"
          type="text"
          placeholder="Свой вариант (без персональных данных)"
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              submit();
            } else if (e.key === "Escape") {
              e.preventDefault();
              onCancel();
            }
          }}
          aria-label={`${label}: свой вариант`}
        />
        <button
          type="button"
          className="custom-input__confirm"
          aria-label="Добавить свой вариант"
          disabled={!text.trim()}
          onClick={submit}
        >
          ＋
        </button>
      </div>
      <p className="field__help">
        Подтвердите по Enter или кнопкой «＋». Без персональных данных.
      </p>
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
