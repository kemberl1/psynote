// Админка: список постов + редактор релиза/новости.
import { useEffect, useRef, useState } from "react";
import { friendlyError } from "../api/errors";
import {
    useAdminNews,
    useCreateNewsPost,
    useDeleteNewsPost,
    usePatchNewsPost,
} from "../api/queries";
import type { NewsPost, NewsPostType, NewsWriteBody } from "../api/types";
import { NewsBody } from "../components/news/NewsBody";
import "../components/questionnaire/questionnaire.css";
import { Badge, Banner, Button, EmptyState, Spinner } from "../components/ui";
import { useConfirm } from "../components/ui/confirm";
import { formatDateTime, formatNewsDate } from "../lib/format";
import { newsTypeLabel } from "../lib/newsMarkup";
import "./admin.css";
import "./news.css";

const EMPTY_FORM: NewsWriteBody = {
  title: "",
  summary: "",
  body: "",
  post_type: "release",
  version_label: "",
  is_published: false,
};

type Selection = { kind: "new" } | { kind: "edit"; id: string };

export function AdminNewsPage() {
  const confirm = useConfirm();
  const listQuery = useAdminNews();
  const createMut = useCreateNewsPost();
  const patchMut = usePatchNewsPost();
  const deleteMut = useDeleteNewsPost();
  const items = listQuery.data?.items ?? [];

  const [selection, setSelection] = useState<Selection | null>(null);
  const [form, setForm] = useState<NewsWriteBody>(EMPTY_FORM);
  const [error, setError] = useState<string | null>(null);
  const [showPreview, setShowPreview] = useState(true);
  const hydratedId = useRef<string | null>(null);

  const selectedId = selection?.kind === "edit" ? selection.id : null;

  useEffect(() => {
    if (!selectedId) {
      hydratedId.current = null;
      return;
    }
    if (hydratedId.current === selectedId) return;
    const post = items.find((it) => it.id === selectedId);
    if (!post) return;
    setForm(formFromPost(post));
    setError(null);
    hydratedId.current = selectedId;
  }, [selectedId, items]);

  const saving = createMut.isPending || patchMut.isPending || deleteMut.isPending;
  const dirty = selection !== null;

  function startNew() {
    setSelection({ kind: "new" });
    setForm({ ...EMPTY_FORM });
    setError(null);
  }

  function selectPost(id: string) {
    setSelection({ kind: "edit", id });
    setError(null);
  }

  async function onSave() {
    setError(null);
    const body: NewsWriteBody = {
      title: form.title?.trim(),
      summary: form.summary?.trim() ?? "",
      body: form.body?.trim(),
      post_type: form.post_type || "release",
      version_label: form.version_label?.trim() ?? "",
      is_published: Boolean(form.is_published),
    };
    try {
      if (selection?.kind === "edit") {
        const saved = await patchMut.mutateAsync({ id: selection.id, body });
        setForm(formFromPost(saved));
      } else {
        const saved = await createMut.mutateAsync(body);
        hydratedId.current = saved.id;
        setSelection({ kind: "edit", id: saved.id });
        setForm(formFromPost(saved));
      }
    } catch (err) {
      setError(friendlyError(err).detail || "Не удалось сохранить пост");
    }
  }

  async function onDelete() {
    if (selection?.kind !== "edit") return;
    const ok = await confirm({
      title: "Удалить пост?",
      text: "Он исчезнет из ленты у всех врачей. Это нельзя отменить.",
      confirmLabel: "Удалить",
      danger: true,
    });
    if (!ok) return;
    setError(null);
    try {
      await deleteMut.mutateAsync(selection.id);
      setSelection(null);
      setForm({ ...EMPTY_FORM });
    } catch (err) {
      setError(friendlyError(err).detail || "Не удалось удалить пост");
    }
  }

  return (
    <div className="admin-news">
      <aside className="admin-news__list" aria-label="Посты">
        <div className="admin-inbox__list-head">
          <h2>Посты</h2>
          <span className="admin-inbox__count">{listQuery.data?.total ?? 0}</span>
        </div>
        <div className="admin-news__list-actions">
          <Button size="sm" variant="primary" onClick={startNew}>
            Новый пост
          </Button>
        </div>
        {listQuery.isPending && (
          <div className="admin-inbox__loading"><Spinner /></div>
        )}
        {listQuery.isError && (
          <div className="admin-inbox__empty">Не удалось загрузить посты</div>
        )}
        {!listQuery.isPending && items.length === 0 && (
          <div className="admin-inbox__empty">Постов ещё нет — напишите первый релиз</div>
        )}
        {items.map((it) => {
          const active = selection?.kind === "edit" && selection.id === it.id;
          return (
            <button
              key={it.id}
              type="button"
              className={`admin-thread${active ? " admin-thread--active" : ""}`}
              onClick={() => selectPost(it.id)}
            >
              <div className="admin-thread__row">
                <span className="admin-thread__name">{it.title || "Без названия"}</span>
                {it.is_published ? (
                  <Badge tone="success">опубл.</Badge>
                ) : (
                  <Badge tone="warning">черновик</Badge>
                )}
              </div>
              <div className="admin-thread__preview">
                {newsTypeLabel(it.post_type)}
                {it.version_label ? ` · ${it.version_label}` : ""}
                {it.summary ? ` · ${it.summary}` : ""}
              </div>
              <div className="admin-thread__meta">
                <span>
                  {it.is_published
                    ? formatNewsDate(it.published_at ?? it.updated_at)
                    : formatDateTime(it.updated_at)}
                </span>
              </div>
            </button>
          );
        })}
      </aside>

      <section className="admin-news__editor" aria-label="Редактор">
        {!dirty ? (
          <EmptyState
            title="Выберите пост"
            text="Слева лента. «Новый пост» — чтобы рассказать врачам про релиз."
          />
        ) : (
          <form
            className="admin-news-form"
            onSubmit={(e) => {
              e.preventDefault();
              void onSave();
            }}
          >
            <header className="admin-news-form__head">
              <h2>{selection?.kind === "new" ? "Новый пост" : "Редактирование"}</h2>
              <p>
                Врачи увидят пост в разделе «Новости и релизы», только если включена публикация.
              </p>
            </header>

            {error && <Banner tone="danger" title="Не сохранилось" text={error} />}

            <label className="field">
              <span className="field__label">
                Заголовок <span className="field__required">*</span>
              </span>
              <input
                className="field__input"
                value={form.title ?? ""}
                onChange={(e) => setForm((f) => ({ ...f, title: e.target.value }))}
                placeholder="PsyNote 1.4 — пакетные дневники"
                maxLength={200}
                required
              />
            </label>

            <div className="admin-news-form__row">
              <label className="field">
                <span className="field__label">Тип</span>
                <select
                  className="field__control"
                  value={form.post_type ?? "release"}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, post_type: e.target.value as NewsPostType }))
                  }
                >
                  <option value="release">Релиз</option>
                  <option value="news">Новость</option>
                </select>
              </label>
              <label className="field">
                <span className="field__label">
                  Версия <span className="field__optional">необязательно</span>
                </span>
                <input
                  className="field__input"
                  value={form.version_label ?? ""}
                  onChange={(e) => setForm((f) => ({ ...f, version_label: e.target.value }))}
                  placeholder="1.4.0"
                  maxLength={40}
                />
              </label>
            </div>

            <label className="field">
              <span className="field__label">
                Коротко <span className="field__optional">для карточки в ленте</span>
              </span>
              <input
                className="field__input"
                value={form.summary ?? ""}
                onChange={(e) => setForm((f) => ({ ...f, summary: e.target.value }))}
                placeholder="Что изменилось в одном предложении"
                maxLength={500}
              />
            </label>

            <label className="field">
              <span className="field__label">
                Текст <span className="field__required">*</span>
              </span>
              <textarea
                className="field__textarea admin-news-form__body"
                value={form.body ?? ""}
                onChange={(e) => setForm((f) => ({ ...f, body: e.target.value }))}
                placeholder={"## Что нового\n- пункт списка\n\nМожно **жирный** и `код`."}
                required
              />
              <p className="field__help">
                Разметка: ## заголовок, - список, **жирный**, `моно`.
              </p>
            </label>

            <label className="admin-news-form__publish">
              <input
                type="checkbox"
                checked={Boolean(form.is_published)}
                onChange={(e) => setForm((f) => ({ ...f, is_published: e.target.checked }))}
              />
              <span>Опубликовать — показать врачам в ленте</span>
            </label>

            <div className="admin-news-form__actions">
              <Button type="submit" variant="primary" loading={saving}>
                Сохранить
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setShowPreview((v) => !v)}
              >
                {showPreview ? "Скрыть превью" : "Показать превью"}
              </Button>
              {selection?.kind === "edit" && (
                <Button
                  type="button"
                  variant="danger"
                  onClick={() => void onDelete()}
                  disabled={saving}
                >
                  Удалить
                </Button>
              )}
            </div>

            {showPreview && (
              <div className="admin-news-form__preview">
                <div className="admin-news-form__preview-label">Как увидит врач</div>
                <div className="news-article__meta">
                  <Badge>{newsTypeLabel(form.post_type ?? "release")}</Badge>
                  {form.version_label && <Badge mono>{form.version_label}</Badge>}
                </div>
                <h3 className="news-article__title">
                  {form.title?.trim() || "Заголовок"}
                </h3>
                {form.summary?.trim() && (
                  <p className="news-article__lead">{form.summary.trim()}</p>
                )}
                <NewsBody source={form.body ?? ""} />
              </div>
            )}
          </form>
        )}
      </section>
    </div>
  );
}

function formFromPost(post: NewsPost): NewsWriteBody {
  return {
    title: post.title,
    summary: post.summary,
    body: post.body ?? "",
    post_type: post.post_type,
    version_label: post.version_label,
    is_published: post.is_published,
  };
}
