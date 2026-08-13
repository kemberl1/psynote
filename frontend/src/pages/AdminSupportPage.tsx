// Админ-инбокс поддержки: список диалогов + переписка + ответ.
import { useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  useAdminSupportThread,
  useAdminSupportThreads,
  useMarkAdminSupportRead,
  useReplyAdminSupport,
} from "../api/queries";
import type { SupportMessage, SupportThreadListItem } from "../api/types";
import { Badge, Button, EmptyState, Spinner } from "../components/ui";
import { formatChatTime, formatDateTime } from "../lib/format";
import "../components/support/support.css";
import "./admin.css";

export function AdminSupportPage() {
  const { threadId } = useParams();
  const navigate = useNavigate();
  const listQuery = useAdminSupportThreads();
  const items = listQuery.data?.items ?? [];

  useEffect(() => {
    if (threadId || items.length === 0) return;
    navigate(`/admin/support/${items[0].thread_id}`, { replace: true });
  }, [threadId, items, navigate]);

  return (
    <div className="admin-inbox">
      <aside className="admin-inbox__list" aria-label="Диалоги">
        <div className="admin-inbox__list-head">
          <h2>Сообщения</h2>
          <span className="admin-inbox__count">
            {listQuery.data?.total ?? 0}
          </span>
        </div>
        {listQuery.isPending && (
          <div className="admin-inbox__loading"><Spinner /></div>
        )}
        {!listQuery.isPending && items.length === 0 && (
          <div className="admin-inbox__empty">Пока никто не писал</div>
        )}
        {items.map((it) => (
          <button
            key={it.thread_id}
            type="button"
            className={`admin-thread${it.thread_id === threadId ? " admin-thread--active" : ""}`}
            onClick={() => navigate(`/admin/support/${it.thread_id}`)}
          >
            <div className="admin-thread__row">
              <span className="admin-thread__name">
                {it.doctor_name || it.doctor_email}
              </span>
              {it.unread_by_admin > 0 && (
                <span className="admin-thread__unread">{it.unread_by_admin}</span>
              )}
            </div>
            <div className="admin-thread__preview">{it.last_message_preview || "—"}</div>
            <div className="admin-thread__meta">
              <span>{it.doctor_email}</span>
              <span>{formatChatTime(it.last_message_at)}</span>
            </div>
          </button>
        ))}
      </aside>

      <section className="admin-inbox__chat" aria-label="Переписка">
        {threadId ? (
          <AdminChat threadId={threadId} fallback={items.find((i) => i.thread_id === threadId)} />
        ) : (
          <EmptyState
            icon="💬"
            title="Выберите диалог"
            text="Слева — все обращения из чата поддержки."
          />
        )}
      </section>
    </div>
  );
}

function AdminChat({
  threadId,
  fallback,
}: {
  threadId: string;
  fallback?: SupportThreadListItem;
}) {
  const { data, isPending, isError } = useAdminSupportThread(threadId);
  const markRead = useMarkAdminSupportRead();
  const reply = useReplyAdminSupport(threadId);
  const [draft, setDraft] = useState("");
  const logRef = useRef<HTMLDivElement>(null);
  const thread = data?.thread ?? fallback;
  const messages = data?.messages ?? [];

  useEffect(() => {
    markRead.mutate(threadId);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [threadId]);

  useEffect(() => {
    const el = logRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages.length, threadId]);

  const submit = () => {
    const body = draft.trim();
    if (!body || reply.isPending) return;
    reply.mutate(body, { onSuccess: () => setDraft("") });
  };

  return (
    <>
      <header className="admin-chat__head">
        <div>
          <div className="admin-chat__name">
            {thread?.doctor_name || "Врач"}
          </div>
          <div className="admin-chat__email">{thread?.doctor_email}</div>
        </div>
        {thread && (
          <Badge tone={thread.unread_by_admin > 0 ? "accent" : "default"}>
            {formatDateTime(thread.last_message_at)}
          </Badge>
        )}
      </header>

      <div className="admin-chat__log" ref={logRef}>
        {isPending && messages.length === 0 && (
          <div className="admin-inbox__loading"><Spinner /></div>
        )}
        {isError && (
          <div className="admin-inbox__empty">Не удалось загрузить переписку</div>
        )}
        {messages.map((m) => (
          <AdminBubble key={m.id} message={m} />
        ))}
      </div>

      <form
        className="admin-chat__form"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <textarea
          className="admin-chat__input"
          rows={2}
          value={draft}
          maxLength={4000}
          placeholder="Ответ врачу…"
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              submit();
            }
          }}
        />
        <Button
          variant="primary"
          type="submit"
          loading={reply.isPending}
          disabled={!draft.trim() || reply.isPending}
        >
          Ответить
        </Button>
      </form>
    </>
  );
}

function AdminBubble({ message }: { message: SupportMessage }) {
  const staff = message.sender_role === "support";
  return (
    <div className={`support-msg ${staff ? "support-msg--user" : "support-msg--support"}`}>
      <div className="support-msg__who">
        {staff ? "Вы · поддержка" : message.sender_name || "Врач"}
      </div>
      <div className="support-msg__bubble">{message.body}</div>
      <div className="support-msg__time">{formatChatTime(message.created_at)}</div>
    </div>
  );
}
