// Плашка анонимизации (docs/08 §5.2/§7, docs/04 §7).
// Показывает ТОЛЬКО счётчики/категории удалённых ПДн — никогда сами значения.
import type { AnonymizationSummary } from "../../api/types";
import { pluralizePii } from "../../lib/format";

interface AnonymizationNoticeProps {
  summary: AnonymizationSummary;
}

export function AnonymizationNotice({ summary }: AnonymizationNoticeProps) {
  const { removed_count, removed_by_type } = summary;

  // Если ничего не убрано — спокойное подтверждение приватности.
  if (!removed_count) {
    return (
      <div className="anon">
        <div className="anon__head">
          <span className="anon__icon" aria-hidden="true">
            ✓
          </span>
          Персональные данные не обнаружены
        </div>
        <div className="anon__text">
          В вашем вводе не найдено персональных данных. Текст обработан безопасно.
        </div>
      </div>
    );
  }

  const types = Object.entries(removed_by_type ?? {}).filter(([, n]) => n > 0);

  return (
    <div className="anon">
      <div className="anon__head">
        <span className="anon__icon" aria-hidden="true">
          ✓
        </span>
        Из вашего ввода удалено {pluralizePii(removed_count)}
      </div>
      <div className="anon__text">
        Перед генерацией персональные данные автоматически обезличены — в систему
        и в историю они не попадают.
      </div>
      {types.length > 0 && (
        <div className="anon__types">
          {types.map(([type, count]) => (
            <span key={type} className="anon__chip">
              {type} <b>{count}</b>
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
