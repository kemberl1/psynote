package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const newsFromSelect = `
	SELECT
		p.id, p.title, p.summary, p.post_type, p.version_label,
		p.is_published, p.published_at, COALESCE(p.author_id::text, ''),
		COALESCE(NULLIF(d.display_name, ''), NULLIF(d.full_name, ''), COALESCE(d.email, '')),
		p.created_at, p.updated_at, p.body
	FROM %s p
	LEFT JOIN doctor d ON d.id = p.author_id`

func scanNewsPost(row pgx.Row, includeBody bool) (*NewsPost, error) {
	var p NewsPost
	var body string
	err := row.Scan(
		&p.ID, &p.Title, &p.Summary, &p.PostType, &p.VersionLabel,
		&p.IsPublished, &p.PublishedAt, &p.AuthorID, &p.AuthorName,
		&p.CreatedAt, &p.UpdatedAt, &body,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if includeBody {
		p.Body = body
	}
	return &p, nil
}

func (r *PgxRepository) CreateNewsPost(ctx context.Context, in NewsPostInput) (*NewsPost, error) {
	query := fmt.Sprintf(`
		WITH inserted AS (
			INSERT INTO news_post
				(title, summary, body, post_type, version_label, is_published, published_at, author_id)
			VALUES (
				$1, $2, $3, $4, $5, $6,
				CASE WHEN $6 THEN now() ELSE NULL END,
				NULLIF($7, '')::uuid
			)
			RETURNING *
		)
		`+newsFromSelect, "inserted")
	p, err := scanNewsPost(r.pool.QueryRow(ctx, query,
		in.Title, in.Summary, in.Body, in.PostType, in.VersionLabel, in.IsPublished, in.AuthorID,
	), true)
	if err != nil {
		return nil, fmt.Errorf("store: create news post: %w", err)
	}
	return p, nil
}

func (r *PgxRepository) UpdateNewsPost(ctx context.Context, id string, patch NewsPostPatch) (*NewsPost, error) {
	query := fmt.Sprintf(`
		WITH updated AS (
			UPDATE news_post
			SET
				title = COALESCE($2, title),
				summary = COALESCE($3, summary),
				body = COALESCE($4, body),
				post_type = COALESCE($5, post_type),
				version_label = COALESCE($6, version_label),
				is_published = COALESCE($7, is_published),
				published_at = CASE
					WHEN COALESCE($7, is_published) AND published_at IS NULL THEN now()
					ELSE published_at
				END,
				updated_at = now()
			WHERE id = $1
			RETURNING *
		)
		`+newsFromSelect, "updated")
	p, err := scanNewsPost(r.pool.QueryRow(ctx, query,
		id,
		patch.Title,
		patch.Summary,
		patch.Body,
		patch.PostType,
		patch.VersionLabel,
		patch.IsPublished,
	), true)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: update news post: %w", err)
	}
	return p, nil
}

func (r *PgxRepository) DeleteNewsPost(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM news_post WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("store: delete news post: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PgxRepository) GetNewsPost(ctx context.Context, id string, publishedOnly bool) (*NewsPost, error) {
	query := fmt.Sprintf(newsFromSelect+` WHERE p.id = $1`, "news_post")
	if publishedOnly {
		query += ` AND p.is_published = TRUE`
	}
	p, err := scanNewsPost(r.pool.QueryRow(ctx, query, id), true)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get news post: %w", err)
	}
	return p, nil
}

func (r *PgxRepository) ListNewsPosts(ctx context.Context, f NewsListFilter) ([]NewsPost, int, error) {
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	where := "TRUE"
	args := []any{}
	arg := 1
	if f.PublishedOnly {
		where += " AND p.is_published = TRUE"
	}
	if f.PostType != "" {
		where += fmt.Sprintf(" AND p.post_type = $%d", arg)
		args = append(args, f.PostType)
		arg++
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM news_post p WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count news posts: %w", err)
	}

	order := "p.updated_at DESC"
	if f.PublishedOnly {
		order = "p.published_at DESC NULLS LAST, p.created_at DESC"
	}

	listQ := fmt.Sprintf(newsFromSelect+`
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d`, "news_post", where, order, arg, arg+1)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list news posts: %w", err)
	}
	defer rows.Close()

	items := make([]NewsPost, 0, limit)
	for rows.Next() {
		p, err := scanNewsPost(rows, f.IncludeBody)
		if err != nil {
			return nil, 0, fmt.Errorf("store: scan news post: %w", err)
		}
		items = append(items, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: news rows: %w", err)
	}
	return items, total, nil
}

var _ NewsRepository = (*PgxRepository)(nil)
