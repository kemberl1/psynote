// DocumentView — карточка-документ с моношрифтом, жирными метками разделов
// и подсветкой плейсхолдеров вида [ДАТА], [ФИО_ВРАЧА] (docs/08 §5.3).
import { Fragment, type ReactNode } from "react";
import { parseDiaryMarkup } from "../../lib/diaryMarkup";
import {
  prepareDiaryPreview,
  type SignatureDoctor,
} from "../../lib/exportSubstitutions";

interface DocumentViewProps {
  content: string;
  title?: string;
  diaryDate?: string;
  createdAt?: string;
  doctor?: SignatureDoctor;
}

export function DocumentView({
  content,
  title,
  diaryDate,
  createdAt,
  doctor,
}: DocumentViewProps) {
  const stamped = prepareDiaryPreview(content, { title, diaryDate, createdAt, doctor });
  return <div className="document">{renderDiaryContent(stamped)}</div>;
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
    if (run.kind === "underline") {
      return <u key={i}>{run.text}</u>;
    }
    return <Fragment key={i}>{run.text}</Fragment>;
  });
}
