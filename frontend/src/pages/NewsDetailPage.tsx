import { Link, useParams } from "react-router-dom";
import { useNewsPost } from "../api/queries";
import { NewsBody } from "../components/news/NewsBody";
import { Badge, EmptyState, Skeleton } from "../components/ui";
import { formatDateTime } from "../lib/format";
import { newsTypeLabel } from "../lib/newsMarkup";
import "./news.css";
import "./pages.css";

export function NewsDetailPage() {
  const { id } = useParams();
  const { data, isPending, isError } = useNewsPost(id);

  if (isPending) {
    return (
      <div className="news-article">
        <Link to="/news" className="back-link">
          ← К новостям
        </Link>
        <Skeleton height="18px" width="120px" />
        <div style={{ height: 12 }} />
        <Skeleton height="36px" width="80%" />
        <div style={{ height: 16 }} />
        <Skeleton height="160px" />
      </div>
    );
  }

  if (isError || !data) {
    return (
      <div className="news-article">
        <Link to="/news" className="back-link">
          ← К новостям
        </Link>
        <EmptyState
          title="Пост не найден"
          text="Его сняли с публикации или ссылка устарела."
        />
      </div>
    );
  }

  const when = data.published_at ?? data.created_at;

  return (
    <article className="news-article">
      <Link to="/news" className="back-link">
        ← К новостям
      </Link>
      <div className="news-article__meta">
        <Badge>{newsTypeLabel(data.post_type)}</Badge>
        {data.version_label && <Badge mono>{data.version_label}</Badge>}
        <time dateTime={when}>{formatDateTime(when)}</time>
      </div>
      <h1 className="news-article__title">{data.title}</h1>
      {data.summary && <p className="news-article__lead">{data.summary}</p>}
      <NewsBody source={data.body ?? ""} />
    </article>
  );
}
