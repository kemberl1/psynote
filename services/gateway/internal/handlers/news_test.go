package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aimed/gateway/internal/store"
)

type fakeNewsRepo struct {
	items     []store.NewsPost
	getErr    error
	listErr   error
	mutErr    error
	lastIn    store.NewsPostInput
	lastPatch store.NewsPostPatch
	deleted   string
}

func (f *fakeNewsRepo) CreateNewsPost(_ context.Context, in store.NewsPostInput) (*store.NewsPost, error) {
	if f.mutErr != nil {
		return nil, f.mutErr
	}
	f.lastIn = in
	now := time.Now().UTC()
	p := store.NewsPost{
		ID:           "news-1",
		Title:        in.Title,
		Summary:      in.Summary,
		Body:         in.Body,
		PostType:     in.PostType,
		VersionLabel: in.VersionLabel,
		IsPublished:  in.IsPublished,
		AuthorID:     in.AuthorID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if in.IsPublished {
		p.PublishedAt = &now
	}
	return &p, nil
}

func (f *fakeNewsRepo) UpdateNewsPost(_ context.Context, id string, patch store.NewsPostPatch) (*store.NewsPost, error) {
	if f.mutErr != nil {
		return nil, f.mutErr
	}
	if f.getErr != nil {
		return nil, f.getErr
	}
	f.lastPatch = patch
	now := time.Now().UTC()
	p := store.NewsPost{ID: id, Title: "было", Body: "текст", PostType: "release", UpdatedAt: now}
	if patch.Title != nil {
		p.Title = *patch.Title
	}
	if patch.Body != nil {
		p.Body = *patch.Body
	}
	if patch.IsPublished != nil {
		p.IsPublished = *patch.IsPublished
		if *patch.IsPublished {
			p.PublishedAt = &now
		}
	}
	return &p, nil
}

func (f *fakeNewsRepo) DeleteNewsPost(_ context.Context, id string) error {
	if f.mutErr != nil {
		return f.mutErr
	}
	if f.getErr != nil {
		return f.getErr
	}
	f.deleted = id
	return nil
}

func (f *fakeNewsRepo) GetNewsPost(_ context.Context, _ string, publishedOnly bool) (*store.NewsPost, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if len(f.items) == 0 {
		return nil, store.ErrNotFound
	}
	p := f.items[0]
	if publishedOnly && !p.IsPublished {
		return nil, store.ErrNotFound
	}
	return &p, nil
}

func (f *fakeNewsRepo) ListNewsPosts(_ context.Context, _ store.NewsListFilter) ([]store.NewsPost, int, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	return f.items, len(f.items), nil
}

func TestNewsList_PublishedOnly(t *testing.T) {
	now := time.Now().UTC()
	h := newNewsListHandler(&fakeNewsRepo{items: []store.NewsPost{{
		ID: "n1", Title: "Релиз 1.4", IsPublished: true, PublishedAt: &now, PostType: "release",
	}}})
	req := withDoctor(httptest.NewRequest(http.MethodGet, "/api/v1/news", nil), "doc-1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data []store.NewsPost `json:"data"`
		Meta struct {
			Total *int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data) != 1 || env.Data[0].Title != "Релиз 1.4" {
		t.Fatalf("unexpected data: %+v", env.Data)
	}
	if env.Meta.Total == nil || *env.Meta.Total != 1 {
		t.Fatalf("want total=1, got %v", env.Meta.Total)
	}
}

func TestNewsList_BadType(t *testing.T) {
	h := newNewsListHandler(&fakeNewsRepo{})
	req := withDoctor(httptest.NewRequest(http.MethodGet, "/api/v1/news?type=blog", nil), "doc-1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestNewsDetail_NotFound(t *testing.T) {
	h := newNewsDetailHandler(&fakeNewsRepo{getErr: store.ErrNotFound})
	req := withDoctor(httptest.NewRequest(http.MethodGet, "/api/v1/news/missing", nil), "doc-1")
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rec.Code)
	}
}

func TestNewsDetail_HidesDraft(t *testing.T) {
	h := newNewsDetailHandler(&fakeNewsRepo{items: []store.NewsPost{{
		ID: "n1", Title: "черновик", IsPublished: false, Body: "секрет",
	}}})
	req := withDoctor(httptest.NewRequest(http.MethodGet, "/api/v1/news/n1", nil), "doc-1")
	req.SetPathValue("id", "n1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404 for draft", rec.Code)
	}
}

func TestAdminNewsCreate_OK(t *testing.T) {
	repo := &fakeNewsRepo{}
	h := newAdminNewsCreateHandler(repo)
	req := withAdmin(httptest.NewRequest(http.MethodPost, "/api/v1/admin/news",
		strings.NewReader(`{"title":"PsyNote 1.4","body":"Пакетные дневники","post_type":"release","version_label":"1.4.0","is_published":true}`)), "admin-1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastIn.Title != "PsyNote 1.4" || repo.lastIn.AuthorID != "admin-1" || !repo.lastIn.IsPublished {
		t.Errorf("unexpected saved: %+v", repo.lastIn)
	}
}

func TestAdminNewsCreate_EmptyTitle(t *testing.T) {
	h := newAdminNewsCreateHandler(&fakeNewsRepo{})
	req := withAdmin(httptest.NewRequest(http.MethodPost, "/api/v1/admin/news",
		strings.NewReader(`{"title":"  ","body":"текст"}`)), "admin-1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestAdminNewsPatch_Publish(t *testing.T) {
	repo := &fakeNewsRepo{}
	h := newAdminNewsPatchHandler(repo)
	req := withAdmin(httptest.NewRequest(http.MethodPatch, "/api/v1/admin/news/n1",
		strings.NewReader(`{"is_published":true}`)), "admin-1")
	req.SetPathValue("id", "n1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastPatch.IsPublished == nil || !*repo.lastPatch.IsPublished {
		t.Errorf("want is_published=true, got %+v", repo.lastPatch)
	}
}

func TestAdminNewsPatch_Empty(t *testing.T) {
	h := newAdminNewsPatchHandler(&fakeNewsRepo{})
	req := withAdmin(httptest.NewRequest(http.MethodPatch, "/api/v1/admin/news/n1",
		strings.NewReader(`{}`)), "admin-1")
	req.SetPathValue("id", "n1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestAdminNewsDelete_OK(t *testing.T) {
	repo := &fakeNewsRepo{}
	h := newAdminNewsDeleteHandler(repo)
	req := withAdmin(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/news/n1", nil), "admin-1")
	req.SetPathValue("id", "n1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if repo.deleted != "n1" {
		t.Errorf("deleted=%q", repo.deleted)
	}
}

func TestAdminNewsDelete_NotFound(t *testing.T) {
	h := newAdminNewsDeleteHandler(&fakeNewsRepo{getErr: store.ErrNotFound})
	req := withAdmin(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/news/missing", nil), "admin-1")
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rec.Code)
	}
}
