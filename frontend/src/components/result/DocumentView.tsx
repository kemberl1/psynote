// DocumentView — карточка-документ с моношрифтом, жирными метками разделов
// и подсветкой плейсхолдеров вида [ДАТА], [ФИО_ВРАЧА] (docs/08 §5.3).
import { Fragment, type ReactNode } from "react";
import { parseDiaryMarkup, omitEmptyDiarySections } from "../../lib/diaryMarkup";

interface DocumentViewProps {
  content: string;
}

export function DocumentView({ content }: DocumentViewProps) {
  return <div className="document">{renderDiaryContent(omitEmptyDiarySections(content))}</div>;
}

function renderDiaryContent(text: string): ReactNode[] {
  return parseDiaryMarkup(text).map((run, i) => {
    if (run.kind === "placeholder") {
      return (
        <span key={i} className="placeholder">
          {run.text}
        </span>
      );
    }
    if (run.kind === "bold") {
      return <strong key={i}>{run.text}</strong>;
    }
    return <Fragment key={i}>{run.text}</Fragment>;
  });
}
