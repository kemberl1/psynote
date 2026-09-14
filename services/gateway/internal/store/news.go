// Package store — новости и релизы (посты из админки для врачей).
package store

import (
	"context"
	"time"
)

const (
	NewsTypeRelease = "release"
	NewsTypeNews    = "news"
)

// NewsPost is a product announcement or release note.
type NewsPost struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Summary      string     `json:"summary"`
	Body         string     `json:"body,omitempty"`
	PostType     string     `json:"post_type"`
	VersionLabel string     `json:"version_label"`
	IsPublished  bool       `json:"is_published"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
	AuthorID     string     `json:"author_id,omitempty"`
	AuthorName   string     `json:"author_name,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// NewsPostInput is the payload for creating a post.
type NewsPostInput struct {
	Title        string
	Summary      string
	Body         string
	PostType     string
	VersionLabel string
	IsPublished  bool
	AuthorID     string
}

// NewsPostPatch is a partial update. Nil fields stay unchanged.
type NewsPostPatch struct {
	Title        *string
	Summary      *string
	Body         *string
	PostType     *string
	VersionLabel *string
	IsPublished  *bool
}

// NewsListFilter controls list queries for public and admin views.
type NewsListFilter struct {
	PublishedOnly bool
	IncludeBody   bool
	PostType      string
	Limit         int
	Offset        int
}

// NewsRepository persists news and release posts.
type NewsRepository interface {
	CreateNewsPost(ctx context.Context, in NewsPostInput) (*NewsPost, error)
	UpdateNewsPost(ctx context.Context, id string, patch NewsPostPatch) (*NewsPost, error)
	DeleteNewsPost(ctx context.Context, id string) error
	GetNewsPost(ctx context.Context, id string, publishedOnly bool) (*NewsPost, error)
	ListNewsPosts(ctx context.Context, f NewsListFilter) ([]NewsPost, int, error)
}
