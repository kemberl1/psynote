// Левый сайдбар — история запросов врача (docs/08 §4.1).
// Список GET /requests; клик открывает /requests/:id. Удаление записи —
// крестик на строке. Табы «Один день / Период» живут на страницах дневника.
import { useMemo, useState, type MouseEvent } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { friendlyError } from "../../api/errors";
import { useDeleteRequest, useHistory, usePatchRequest } from "../../api/queries";
import type { HistoryItem } from "../../api/types";
import {
    documentTypeLabel,
    formatDateTimeShort,
    isPendingStatus,
    statusLabel,
} from "../../lib/format";
import { displayHistoryTitle } from "../../lib/historyTitles";
import { EditableTitle, TitleEditButton } from "../history/EditableTitle";
import { Banner, Button, EmptyState, Skeleton } from "../ui";
import { useConfirm } from "../ui/confirm";

export function HistorySidebar() {
  const navigate = useNavigate();
  const { id: activeId } = useParams();
  const [search, setSearch] = useState("");
  const { data, isPending, isError, error, refetch } = useHistory();
  const deleteMutation = useDeleteRequest();
  const patchMutation = usePatchRequest();
  const confirm = useConfirm();

  const items = data?.items ?? [];
  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return items;
    return items.filter(
      (it) =>
        displayHistoryTitle(it.title_safe).toLowerCase().includes(q) ||
        documentTypeLabel(it.document_type).toLowerCase().includes(q),
    );
  }, [items, search]);

  const handleDelete = (item: HistoryItem, e: MouseEvent) => {
    e.stopPropagation();
    if (deleteMutation.isPending) return;
    const isBatch = item.document_type === "batch";
    void confirm({
      title: isBatch ? "Удалить период?" : "Удалить запись?",
      text: isBatch
        ? "Все дневники за этот период исчезнут из истории."
        : "Запись исчезнет из истории.",
      confirmLabel: "Удалить",
      cancelLabel: "Отмена",
      danger: true,
    }).then((ok) => {
      if (!ok) return;
      deleteMutation.mutate(item.request_id, {
        onSuccess: () => {
          if (activeId === item.request_id) navigate("/diary");
        },
      });
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
            renaming={
              patchMutation.isPending &&
              patchMutation.variables?.id === item.request_id
            }
            onClick={() => navigate(`/requests/${item.request_id}`)}
            onDelete={(e) => handleDelete(item, e)}
            onRename={(title) =>
              patchMutation.mutate({
                id: item.request_id,
                body: { title_safe: title },
              })
            }
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
  renaming,
  onClick,
  onDelete,
  onRename,
}: {
  item: HistoryItem;
  active: boolean;
  deleting: boolean;
  renaming: boolean;
  onClick: () => void;
  onDelete: (e: MouseEvent) => void;
  onRename: (title: string) => void;
}) {
  const [editing, setEditing] = useState(false);
  const pending = isPendingStatus(item.status);
  const title = displayHistoryTitle(item.title_safe);
  const when = formatDateTimeShort(item.created_at);
  const meta = pending ? `${statusLabel(item.status)} · ${when}` : when;

  return (
    <div
      className={`history-item${active ? " history-item--active" : ""}${
        pending ? " history-item--pending" : ""
      }${editing ? " history-item--editing" : ""}`}
    >
      {editing ? (
        <div className="history-item__main">
          <EditableTitle
            value={title}
            editing
            onEditingChange={setEditing}
            onSave={onRename}
            saving={renaming}
            className="history-item__title"
            inputClassName="history-item__edit-input"
          />
          <span className="history-item__meta">{meta}</span>
        </div>
      ) : (
        <button
          type="button"
          className="history-item__main"
          onClick={onClick}
          aria-current={active ? "true" : undefined}
        >
          <span className="history-item__title" title={title}>{title}</span>
          <span className="history-item__meta">{meta}</span>
        </button>
      )}
      <div className="history-item__actions">
        {!editing && (
          <TitleEditButton
            className="history-item__icon"
            disabled={renaming || deleting}
            onClick={() => setEditing(true)}
          />
        )}
        <button
          type="button"
          className="history-item__icon"
          onClick={onDelete}
          disabled={deleting}
          aria-label="Удалить из истории"
          title="Удалить"
        >
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path
              d="M18 6 6 18M6 6l12 12"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
            />
          </svg>
        </button>
      </div>
    </div>
  );
}

function SidebarSkeleton() {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 4, padding: 4 }}>
      {Array.from({ length: 8 }).map((_, i) => (
        <div
          key={i}
          style={{ display: "flex", flexDirection: "column", gap: 4, padding: 6 }}
        >
          <Skeleton width="88%" height="12px" />
          <Skeleton width="42%" height="9px" />
        </div>
      ))}
    </div>
  );
}
