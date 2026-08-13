// Все отзывы врачей на генерации — для разбора качества.
import { useAdminFeedback } from "../api/queries";
import { StarsStatic } from "../components/feedback/GenerationFeedback";
import { Badge, EmptyState, Spinner } from "../components/ui";
import { documentTypeLabel, formatDateTime } from "../lib/format";
import "./admin.css";

export function AdminFeedbackPage() {
  const { data, isPending, isError } = useAdminFeedback();
  const items = data?.items ?? [];

  return (
    <div className="admin-feedback">
      <header className="admin-feedback__head">
        <h2>Отзывы на генерации</h2>
        <p>
          Врач ставит звёзды и пишет, что поправить. Если есть цитата —
          смотрите её в первую очередь: так проще понять конкретный промах.
        </p>
      </header>

      {isPending && (
        <div className="admin-inbox__loading"><Spinner /></div>
      )}
      {isError && (
        <EmptyState icon="⚠" title="Не удалось загрузить отзывы" />
      )}
      {!isPending && items.length === 0 && (
        <EmptyState
          icon="★"
          title="Отзывов пока нет"
          text="После генерации врач может оценить дневник — карточки появятся здесь."
        />
      )}

      <div className="admin-feedback__list">
        {items.map((it) => (
          <article key={it.id} className="admin-fb-card">
            <div className="admin-fb-card__top">
              <StarsStatic rating={it.rating} />
              <Badge>{documentTypeLabel(it.document_type)}</Badge>
              <span className="admin-fb-card__date">{formatDateTime(it.updated_at)}</span>
            </div>
            <div className="admin-fb-card__title">{it.title_safe || "Дневник без названия"}</div>
            <div className="admin-fb-card__who">
              {it.doctor_name || "Врач"}
              {it.doctor_email ? ` · ${it.doctor_email}` : ""}
            </div>
            {it.comment && (
              <p className="admin-fb-card__comment">{it.comment}</p>
            )}
            {it.quote && (
              <blockquote className="admin-fb-card__quote">
                {it.quote}
              </blockquote>
            )}
            {!it.comment && !it.quote && (
              <p className="admin-fb-card__muted">Только оценка, без комментария</p>
            )}
          </article>
        ))}
      </div>
    </div>
  );
}
