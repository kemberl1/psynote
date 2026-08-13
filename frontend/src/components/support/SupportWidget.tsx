// Плавающий чат поддержки: врач пишет разработчику, ответ приходит сюда же.
import { useEffect, useRef, useState } from "react";
import {
  useMarkSupportRead,
  useSendSupportMessage,
  useSupportThread,
} from "../../api/queries";
import { formatChatTime } from "../../lib/format";
import { Button } from "../ui";
import "./support.css";

function ChatIcon() {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M5 6.8A2.8 2.8 0 0 1 7.8 4h8.4A2.8 2.8 0 0 1 19 6.8v6.4A2.8 2.8 0 0 1 16.2 16H13l-3.6 3.2c-.5.44-1.4.08-1.4-.58V16H7.8A2.8 2.8 0 0 1 5 13.2V6.8Z"
        stroke="currentColor"
        strokeWidth="1.7"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function SupportWidget() {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const logRef = useRef<HTMLDivElement>(null);
  const { data, isPending } = useSupportThread();
  const send = useSendSupportMessage();
  const markRead = useMarkSupportRead();
  const messages = data?.messages ?? [];
  const unread = data?.unread ?? 0;

  useEffect(() => {
    if (open) markRead.mutate();
    // помечаем прочитанным при открытии панели
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const el = logRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [open, messages.length]);

  const submit = () => {
    const body = draft.trim();
    if (!body || send.isPending) return;
    send.mutate(body, { onSuccess: () => setDraft("") });
  };

  return (
    <>
      <button
        type="button"
        className={`support-fab${open ? " support-fab--open" : ""}`}
        aria-label={open ? "Закрыть чат поддержки" : "Открыть чат поддержки"}
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <ChatIcon />
        {!open && unread > 0 && (
          <span className="support-fab__badge">{unread > 9 ? "9+" : unread}</span>
        )}
      </button>

      {open && (
        <section className="support-panel" aria-label="Чат поддержки">
          <header className="support-panel__head">
            <div>
              <div className="support-panel__title">Поддержка</div>
              <div className="support-panel__sub">
                Напишите, если что-то сломалось, неточно или есть идея.
                Ответим в этом же окне.
              </div>
            </div>
            <button
              type="button"
              className="support-panel__close"
              aria-label="Закрыть"
              onClick={() => setOpen(false)}
            >
              ×
            </button>
          </header>

          <div className="support-panel__log" ref={logRef}>
            {isPending && messages.length === 0 && (
              <div className="support-empty">
                <div className="support-empty__text">Загружаем переписку…</div>
              </div>
            )}
            {!isPending && messages.length === 0 && (
              <div className="support-empty">
                <div className="support-empty__icon" aria-hidden="true">💬</div>
                <div className="support-empty__title">Пока тихо</div>
                <div className="support-empty__text">
                  Опишите проблему своими словами — чем конкретнее, тем быстрее
                  разберёмся. Можно вставить фрагмент дневника.
                </div>
              </div>
            )}
            {messages.map((m) => {
              const mine = m.sender_role === "user";
              return (
                <div
                  key={m.id}
                  className={`support-msg ${mine ? "support-msg--user" : "support-msg--support"}`}
                >
                  <div className="support-msg__who">
                    {mine ? "Вы" : "Поддержка"}
                  </div>
                  <div className="support-msg__bubble">{m.body}</div>
                  <div className="support-msg__time">{formatChatTime(m.created_at)}</div>
                </div>
              );
            })}
          </div>

          <form
            className="support-panel__form"
            onSubmit={(e) => {
              e.preventDefault();
              submit();
            }}
          >
            <textarea
              className="support-panel__input"
              rows={1}
              value={draft}
              placeholder="Сообщение…"
              maxLength={4000}
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
              size="sm"
              type="submit"
              loading={send.isPending}
              disabled={!draft.trim() || send.isPending}
            >
              Отправить
            </Button>
          </form>
        </section>
      )}
    </>
  );
}
