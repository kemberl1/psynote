// DocumentView — карточка-документ с моношрифтом и подсветкой плейсхолдеров
// вида [ДАТА], [ФИО_ВРАЧА] (docs/08 §5.3). Подстановка реальных значений —
// Этап 8 (экспорт), здесь только визуальная подсветка.
import { Fragment, type ReactNode } from "react";

const PLACEHOLDER_SPLIT_RE = /(\[[A-ZА-ЯЁ0-9_]+\])/g;
const PLACEHOLDER_TEST_RE = /^\[[A-ZА-ЯЁ0-9_]+\]$/;

interface DocumentViewProps {
  content: string;
}

export function DocumentView({ content }: DocumentViewProps) {
  return (
    <div className="document">{highlightPlaceholders(content)}</div>
  );
}

/** Разбивает текст на сегменты, оборачивая плейсхолдеры в .placeholder. */
function highlightPlaceholders(text: string): ReactNode[] {
  const parts = text.split(PLACEHOLDER_SPLIT_RE);
  return parts.map((part, i) =>
    PLACEHOLDER_TEST_RE.test(part) ? (
      <span key={i} className="placeholder">
        {part}
      </span>
    ) : (
      <Fragment key={i}>{part}</Fragment>
    ),
  );
}
