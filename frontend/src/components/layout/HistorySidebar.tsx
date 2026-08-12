// Левый сайдбар — история запросов врача (docs/08 §4.1).
// Список GET /requests; клик открывает /requests/:id. Удаление записи —
// крестик на строке. Табы «Один день / Период» живут на страницах дневника.
import { useMemo, useState, type MouseEvent } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { friendlyError } from "../../api/errors";
import { useDeleteRequest, useHistory } from "../../api/queries";
import type { HistoryItem } from "../../api/types";
import {
  documentTypeLabel,
  formatDateShort,
  isPendingStatus,
  statusLabel,
} from "../../lib/format";
import { Badge, Banner, Button, EmptyState, Skeleton } from "../ui";

export function HistorySidebar() {
  const navigate = useNavigate();
  const { id: activeId } = useParams();
  const [search, setSearch] = useState("");
  const { data, isPending, isError, error, refetch } = useHistory();
  const deleteMutation = useDeleteRequest();

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

  const handleDelete = (item: HistoryItem, e: MouseEvent) => {
    e.stopPropagation();
    if (deleteMutation.isPending) return;
    const label =
      item.document_type === "batch"
        ? "Удалить весь пакет дневников из истории?"
        : "Удалить запись из истории?";
    if (!window.confirm(label)) return;
    deleteMutation.mutate(item.request_id, {
      onSuccess: () => {
        if (activeId === item.request_id) navigate("/diary");
      },
    });
  };

  return (
    <aside className="sidebar" aria-label="История запросов">
      <div className="sidebar__head">
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
            deleting={
              deleteMutation.isPending &&
              deleteMutation.variables === item.request_id
            }
            onClick={() => navigate(`/requests/${item.request_id}`)}
            onDelete={(e) => handleDelete(item, e)}
          />
        ))}
      </div>
    </aside>
  );
}

function HistoryRow({
  item,
  active,
  deleting,
  onClick,
  onDelete,
}: {
  item: HistoryItem;
  active: boolean;
  deleting: boolean;
  onClick: () => void;
  onDelete: (e: MouseEvent) => void;
}) {
  const pending = isPendingStatus(item.status);
  const typeLabel =
    item.document_type === "batch"
      ? item.children_count
        ? `Пакет · ${item.children_count} дн.`
        : "Пакет дневников"
      : documentTypeLabel(item.document_type);

  return (
    <div
      className={`history-item${active ? " history-item--active" : ""}${
        pending ? " history-item--pending" : ""
      }`}
    >
      <button
        type="button"
        className="history-item__main"
        onClick={onClick}
        aria-current={active ? "true" : undefined}
      >
        <span className="history-item__title">{item.title_safe}</span>
        <span className="history-item__meta">
          <Badge tone={pending ? "accent" : "default"}>{typeLabel}</Badge>
          {pending && (
            <>
              <span className="history-item__dot" aria-hidden="true" />
              <span className="history-item__status">{statusLabel(item.status)}</span>
            </>
          )}
          <span className="history-item__dot" aria-hidden="true" />
          <span>{formatDateShort(item.created_at)}</span>
        </span>
      </button>
      <button
        type="button"
        className="history-item__delete"
        onClick={onDelete}
        disabled={deleting}
        aria-label="Удалить из истории"
        title="Удалить"
      >
        ×
      </button>
    </div>
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
