// Лента новостей и релизов для врача.
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useNews } from "../api/queries";
import { Badge, EmptyState, Skeleton } from "../components/ui";
import { formatNewsDate } from "../lib/format";
import { isRecentNews, newsTypeLabel } from "../lib/newsMarkup";
import "./news.css";
import "./pages.css";

type Filter = "all" | "release" | "news";

export function NewsPage() {
  const [filter, setFilter] = useState<Filter>("all");
  const { data, isPending, isError } = useNews(filter);
  const items = data?.items ?? [];

  const featured = useMemo(() => {
    if (filter !== "all") return null;
    return items.find((it) => it.post_type === "release") ?? items[0] ?? null;
  }, [filter, items]);
  const rest = featured ? items.filter((it) => it.id !== featured.id) : items;

  return (
    <div className="news">
      <div className="news-filters" role="tablist" aria-label="Фильтр постов">
        {(
          [
            ["all", "Все"],
            ["release", "Релизы"],
            ["news", "Новости"],
          ] as const
        ).map(([id, label]) => (
          <button
            key={id}
            type="button"
            role="tab"
            aria-selected={filter === id}
            className={`news-filters__btn${filter === id ? " news-filters__btn--active" : ""}`}
            onClick={() => setFilter(id)}
          >
            {label}
          </button>
        ))}
      </div>

      {isPending && (
        <div className="news-feed" aria-hidden="true">
          <Skeleton height="160px" radius="16px" />
          <Skeleton height="88px" radius="12px" />
          <Skeleton height="88px" radius="12px" />
        </div>
      )}

      {isError && (
        <EmptyState
          title="Не удалось загрузить новости"
          text="Проверьте соединение и откройте раздел ещё раз."
        />
      )}

      {!isPending && !isError && items.length === 0 && (
        <EmptyState
          title="Пока тихо"
          text="Когда выйдет обновление, оно появится здесь."
        />
      )}

      {!isPending && !isError && featured && (
        <Link to={`/news/${featured.id}`} className="news-feature">
          <div className="news-feature__meta">
            <Badge tone="accent">{newsTypeLabel(featured.post_type)}</Badge>
            {featured.version_label && (
              <Badge mono>{featured.version_label}</Badge>
            )}
            {isRecentNews(featured.published_at ?? featured.created_at) && (
              <Badge tone="success">новое</Badge>
            )}
            <span className="news-feature__date">
              {formatNewsDate(featured.published_at ?? featured.created_at)}
            </span>
          </div>
          <h2 className="news-feature__title">{featured.title}</h2>
          {featured.summary && (
            <p className="news-feature__summary">{featured.summary}</p>
          )}
          <span className="news-feature__more">Читать</span>
        </Link>
      )}

      {rest.length > 0 && (
        <ol className="news-feed">
          {rest.map((it) => (
            <li key={it.id}>
              <Link to={`/news/${it.id}`} className="news-row">
                <time
                  className="news-row__date"
                  dateTime={it.published_at ?? it.created_at}
                >
                  {formatNewsDate(it.published_at ?? it.created_at)}
                </time>
                <div className="news-row__body">
                  <div className="news-row__meta">
                    <Badge tone={it.post_type === "release" ? "accent" : "default"}>
                      {newsTypeLabel(it.post_type)}
                    </Badge>
                    {it.version_label && <Badge mono>{it.version_label}</Badge>}
                    {isRecentNews(it.published_at ?? it.created_at) && (
                      <Badge tone="success">новое</Badge>
                    )}
                  </div>
                  <h3 className="news-row__title">{it.title}</h3>
                  {it.summary && <p className="news-row__summary">{it.summary}</p>}
                </div>
              </Link>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}
