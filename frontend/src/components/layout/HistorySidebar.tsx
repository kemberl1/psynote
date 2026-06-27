// Левый сайдбар — история запросов врача (docs/08 §4.1).
// Список GET /requests (обезличенный title_safe + тип + дата); клик открывает
// прошлый результат (/requests/:id). Кнопка «＋ Новый дневник» вверху.
// Состояния: загрузка (скелетоны) / пусто (empty-state) / ошибка (баннер).
import { useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { friendlyError } from "../../api/errors";
import { useHistory } from "../../api/queries";
import type { HistoryItem } from "../../api/types";
import { documentTypeLabel, formatDateShort } from "../../lib/format";
import { Badge, Banner, Button, EmptyState, Skeleton } from "../ui";

export function HistorySidebar() {
  const navigate = useNavigate();
  const { id: activeId } = useParams();
  const [search, setSearch] = useState("");
  const { data, isPending, isError, error, refetch } = useHistory();

  const items = data?.items ?? [];
  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return items;
    return items.filter(
      (it) =>
        it.title_safe.toLowerCase().includes(q) ||
        documentTypeLabel(it.document_type).toLowerCase().includes(q),
    );
  }, [items, search]);

  return (
    <aside className="sidebar" aria-label="История запросов">
      <div className="sidebar__head">
        <Button
          variant="primary"
          block
          onClick={() => navigate("/diary")}
          aria-label="Создать новый дневник"
        >
          ＋ Новый дневник
        </Button>
        <Button
          variant="secondary"
          block
          onClick={() => navigate("/diary/batch")}
          aria-label="Пакетная генерация за период"
        >
          Новые дневники за период
        </Button>
        <input
          className="sidebar__search"
          type="search"
          placeholder="Поиск по истории…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          aria-label="Поиск по истории"
          disabled={isPending || isError || items.length === 0}
        />
      </div>

      <div className="sidebar__title">История</div>

      <div className="sidebar__list">
        {isPending && <SidebarSkeleton />}

        {isError && (
          <div style={{ padding: "0 8px" }}>
            <Banner
              tone="danger"
              title={friendlyError(error).title}
              text={friendlyError(error).detail}
              action={
                <Button size="sm" onClick={() => void refetch()}>
                  Повторить
                </Button>
              }
            />
          </div>
        )}

        {!isPending && !isError && items.length === 0 && (
          <EmptyState
            icon="🗂"
            title="Пока пусто"
            text="Здесь появятся ваши сгенерированные дневники."
          />
        )}

        {!isPending &&
          !isError &&
          items.length > 0 &&
          filtered.length === 0 && (
            <EmptyState
              icon="🔍"
              title="Ничего не найдено"
              text="Измените поисковый запрос."
            />
          )}

        {filtered.map((item) => (
          <HistoryRow
            key={item.request_id}
            item={item}
            active={item.request_id === activeId}
            onClick={() => navigate(`/requests/${item.request_id}`)}
          />
        ))}
      </div>
    </aside>
  );
}

function HistoryRow({
  item,
  active,
  onClick,
}: {
  item: HistoryItem;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className={`history-item${active ? " history-item--active" : ""}`}
      onClick={onClick}
      aria-current={active ? "true" : undefined}
    >
      <span className="history-item__title">{item.title_safe}</span>
      <span className="history-item__meta">
        <Badge>{documentTypeLabel(item.document_type)}</Badge>
        <span className="history-item__dot" aria-hidden="true" />
        <span>{formatDateShort(item.created_at)}</span>
      </span>
    </button>
  );
}

function SidebarSkeleton() {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8, padding: 6 }}>
      {Array.from({ length: 6 }).map((_, i) => (
        <div
          key={i}
          style={{ display: "flex", flexDirection: "column", gap: 6, padding: 6 }}
        >
          <Skeleton width="85%" height="13px" />
          <Skeleton width="50%" height="10px" />
        </div>
      ))}
    </div>
  );
}
