import {
    useEffect,
    useRef,
    useState,
    type FormEvent,
    type KeyboardEvent,
    type MouseEvent,
} from "react";

interface EditableTitleProps {
  value: string;
  editing: boolean;
  onEditingChange: (editing: boolean) => void;
  onSave: (next: string) => void;
  saving?: boolean;
  as?: "span" | "h2" | "div";
  className?: string;
  inputClassName?: string;
}

export function EditableTitle({
  value,
  editing,
  onEditingChange,
  onSave,
  saving,
  as: Tag = "span",
  className,
  inputClassName,
}: EditableTitleProps) {
  const [draft, setDraft] = useState(value);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (editing) {
      setDraft(value);
      const id = window.requestAnimationFrame(() => {
        inputRef.current?.focus();
        inputRef.current?.select();
      });
      return () => window.cancelAnimationFrame(id);
    }
    setDraft(value);
    return undefined;
  }, [editing, value]);

  const commit = () => {
    const next = draft.trim();
    if (!next) {
      setDraft(value);
      onEditingChange(false);
      return;
    }
    if (next === value) {
      onEditingChange(false);
      return;
    }
    onSave(next);
    onEditingChange(false);
  };

  const cancel = () => {
    setDraft(value);
    onEditingChange(false);
  };

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    commit();
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Escape") {
      e.preventDefault();
      cancel();
    }
  };

  if (!editing) {
    return <Tag className={className}>{value}</Tag>;
  }

  return (
    <form className="editable-title__form" onSubmit={handleSubmit}>
      <input
        ref={inputRef}
        className={inputClassName ?? "editable-title__input"}
        value={draft}
        disabled={saving}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        onKeyDown={handleKeyDown}
        aria-label="Название сессии"
      />
    </form>
  );
}

export function TitleEditButton({
  onClick,
  className,
  disabled,
}: {
  onClick: () => void;
  className?: string;
  disabled?: boolean;
}) {
  const handleClick = (e: MouseEvent<HTMLButtonElement>) => {
    e.preventDefault();
    e.stopPropagation();
    onClick();
  };
  return (
    <button
      type="button"
      className={className}
      onClick={handleClick}
      disabled={disabled}
      aria-label="Переименовать сессию"
      title="Переименовать"
    >
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <path
          d="M12 20h9M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
    </button>
  );
}
