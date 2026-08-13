// Оценка и комментарий к сгенерированному дневнику.
import { useEffect, useState } from "react";
import { useRequestFeedback, useUpsertFeedback } from "../../api/queries";
import { Button } from "../ui";
import "./feedback.css";

const RATING_HINT: Record<number, string> = {
  1: "Совсем мимо",
  2: "Слабо",
  3: "Нормально, но есть что поправить",
  4: "Хорошо, почти то",
  5: "Отлично, можно так и оставить",
};

function Star({ on, size = 20 }: { on: boolean; size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true">
      <path
        d="M12 3.6 14.6 9l6 .6-4.6 4 1.4 5.8L12 16.8 6.6 19.4 8 13.6 3.4 9.6l6-.6L12 3.6Z"
        fill={on ? "currentColor" : "none"}
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function StarsStatic({ rating, muted }: { rating: number; muted?: boolean }) {
  return (
    <span
      className={`fb-stars-static${muted ? " fb-stars-static--muted" : ""}`}
      aria-label={`${rating} из 5`}
    >
      {[1, 2, 3, 4, 5].map((n) => (
        <Star key={n} on={n <= rating} size={14} />
      ))}
    </span>
  );
}

interface GenerationFeedbackProps {
  requestId: string;
  compact?: boolean;
}

export function GenerationFeedback({ requestId, compact }: GenerationFeedbackProps) {
  const { data, isPending } = useRequestFeedback(requestId);
  const save = useUpsertFeedback(requestId);
  const existing = data?.feedback ?? null;

  const [rating, setRating] = useState(0);
  const [hover, setHover] = useState(0);
  const [comment, setComment] = useState("");
  const [quote, setQuote] = useState("");
  const [savedFlash, setSavedFlash] = useState(false);

  useEffect(() => {
    if (!existing) return;
    setRating(existing.rating);
    setComment(existing.comment);
    setQuote(existing.quote);
  }, [existing]);

  const shown = hover || rating;
  const dirty =
    rating > 0 &&
    (!existing ||
      existing.rating !== rating ||
      existing.comment !== comment.trim() ||
      existing.quote !== quote.trim());

  const onSubmit = () => {
    if (rating < 1 || save.isPending) return;
    save.mutate(
      { rating, comment: comment.trim(), quote: quote.trim() },
      {
        onSuccess: () => {
          setSavedFlash(true);
          window.setTimeout(() => setSavedFlash(false), 2400);
        },
      },
    );
  };

  if (isPending && !existing) {
    return null;
  }

  return (
    <section className={`fb${compact ? " fb--compact" : ""}`} aria-label="Отзыв о генерации">
      <div className="fb__head">
        <div className="fb__title">
          {existing ? "Ваш отзыв о дневнике" : "Как вам этот дневник?"}
        </div>
        <div className="fb__hint">
          Оценка помогает понять, где генерация промахивается. Можно поправить
          позже — сохраняется один отзыв на этот документ.
        </div>
      </div>

      <div className="fb__stars" onMouseLeave={() => setHover(0)}>
        {[1, 2, 3, 4, 5].map((n) => (
          <button
            key={n}
            type="button"
            className={`fb__star${n <= shown ? " fb__star--on" : ""}`}
            aria-label={`${n} из 5`}
            onMouseEnter={() => setHover(n)}
            onClick={() => setRating(n)}
          >
            <Star on={n <= shown} />
          </button>
        ))}
        {shown > 0 && (
          <span className="fb__star-label">{RATING_HINT[shown]}</span>
        )}
      </div>

      {rating > 0 && (
        <div className="fb__fields">
          <label className="fb__label">
            Что неточно или что улучшить
            <textarea
              className="fb__area"
              rows={compact ? 2 : 3}
              maxLength={4000}
              value={comment}
              placeholder="Например: слишком общие формулировки в разделе динамики, не отражён отказ от еды…"
              onChange={(e) => setComment(e.target.value)}
            />
          </label>
          <label className="fb__label">
            Цитата из дневника
            <span className="fb__quote-tip">
              Чтобы было понятно, в чём проблема, вставьте конкретный фрагмент
              сгенерированного текста.
            </span>
            <textarea
              className="fb__area"
              rows={2}
              maxLength={2000}
              value={quote}
              placeholder="«Состояние стабильное, без выраженной динамики…»"
              onChange={(e) => setQuote(e.target.value)}
            />
          </label>
        </div>
      )}

      {rating > 0 && (
        <div className="fb__actions">
          <Button
            variant="primary"
            size="sm"
            onClick={onSubmit}
            loading={save.isPending}
            disabled={!dirty || save.isPending}
          >
            {existing ? "Обновить отзыв" : "Отправить отзыв"}
          </Button>
          {savedFlash && <span className="fb__saved">Сохранено, спасибо</span>}
        </div>
      )}
    </section>
  );
}
