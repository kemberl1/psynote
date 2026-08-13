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

type fakeFeedbackRepo struct {
	item    *store.GenerationFeedback
	list    []store.AdminFeedbackItem
	getErr  error
	putErr  error
	lastRec store.GenerationFeedback
}

func (f *fakeFeedbackRepo) UpsertFeedback(_ context.Context, rec store.GenerationFeedback) (*store.GenerationFeedback, error) {
	if f.putErr != nil {
		return nil, f.putErr
	}
	f.lastRec = rec
	rec.ID = "fb-1"
	rec.CreatedAt = time.Now()
	rec.UpdatedAt = rec.CreatedAt
	return &rec, nil
}

func (f *fakeFeedbackRepo) GetFeedback(_ context.Context, _, _ string) (*store.GenerationFeedback, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.item == nil {
		return nil, store.ErrNotFound
	}
	return f.item, nil
}

func (f *fakeFeedbackRepo) ListFeedback(_ context.Context, _, _ int) ([]store.AdminFeedbackItem, int, error) {
	return f.list, len(f.list), nil
}

func TestFeedbackGet_None(t *testing.T) {
	d := feedbackDeps{
		history:  &fakeRepo{detail: &store.HistoryDetail{RequestID: "r1", Status: "done"}},
		feedback: &fakeFeedbackRepo{getErr: store.ErrNotFound},
	}
	h := newFeedbackGetHandler(d)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/requests/r1/feedback", nil)
	req.SetPathValue("id", "r1")
	req = withDoctor(req, "doc-1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data["feedback"] != nil {
		t.Errorf("want null feedback, got %v", env.Data["feedback"])
	}
}

func TestFeedbackPut_OK(t *testing.T) {
	fb := &fakeFeedbackRepo{}
	d := feedbackDeps{
		history:  &fakeRepo{detail: &store.HistoryDetail{RequestID: "r1", Status: "done"}},
		feedback: fb,
	}
	h := newFeedbackPutHandler(d)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/requests/r1/feedback",
		strings.NewReader(`{"rating":4,"comment":"тонковато","quote":"состояние стабильное"}`))
	req.SetPathValue("id", "r1")
	req = withDoctor(req, "doc-1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if fb.lastRec.Rating != 4 || fb.lastRec.DoctorID != "doc-1" || fb.lastRec.Quote != "состояние стабильное" {
		t.Errorf("unexpected saved rec: %+v", fb.lastRec)
	}
}

func TestFeedbackPut_BadRating(t *testing.T) {
	d := feedbackDeps{
		history:  &fakeRepo{detail: &store.HistoryDetail{RequestID: "r1"}},
		feedback: &fakeFeedbackRepo{},
	}
	h := newFeedbackPutHandler(d)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/requests/r1/feedback",
		strings.NewReader(`{"rating":0}`))
	req.SetPathValue("id", "r1")
	req = withDoctor(req, "doc-1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestFeedbackPut_ForeignRequest(t *testing.T) {
	d := feedbackDeps{
		history:  &fakeRepo{getErr: store.ErrNotFound},
		feedback: &fakeFeedbackRepo{},
	}
	h := newFeedbackPutHandler(d)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/requests/r1/feedback",
		strings.NewReader(`{"rating":5}`))
	req.SetPathValue("id", "r1")
	req = withDoctor(req, "doc-1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rec.Code)
	}
}

func TestAdminFeedbackList(t *testing.T) {
	h := newAdminFeedbackListHandler(&fakeFeedbackRepo{
		list: []store.AdminFeedbackItem{{
			GenerationFeedback: store.GenerationFeedback{ID: "fb-1", Rating: 3, Comment: "хм"},
			DoctorName:         "Петров",
			TitleSafe:          "Дневник 12.08",
			DocumentType:       "daily",
		}},
	})
	req := withAdmin(httptest.NewRequest(http.MethodGet, "/api/v1/admin/feedback", nil), "admin-1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
