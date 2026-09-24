package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aimed/gateway/internal/store"
)

type fakeSupportRepo struct {
	thread     *store.SupportThread
	inbox      *store.SupportThreadListItem
	threads    []store.SupportThreadListItem
	messages   []store.SupportMessage
	summary    *store.SupportSummary
	getErr     error
	listErr    error
	addErr     error
	lastBody   string
	lastFiles  []store.SupportAttachmentUpload
	attachment *store.SupportAttachmentFile
	lastRole   string
	markedWho  string
	createdFor string
}

func (f *fakeSupportRepo) GetThreadByDoctor(_ context.Context, _ string) (*store.SupportThread, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.thread == nil {
		return nil, store.ErrNotFound
	}
	return f.thread, nil
}

func (f *fakeSupportRepo) GetThreadByID(_ context.Context, _ string) (*store.SupportThread, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.thread == nil {
		return nil, store.ErrNotFound
	}
	return f.thread, nil
}

func (f *fakeSupportRepo) GetThreadInboxItem(_ context.Context, _ string) (*store.SupportThreadListItem, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.inbox != nil {
		return f.inbox, nil
	}
	if f.thread == nil {
		return nil, store.ErrNotFound
	}
	return &store.SupportThreadListItem{SupportThread: *f.thread, DoctorName: "Врач", DoctorEmail: "d@x.ru"}, nil
}

func (f *fakeSupportRepo) GetOrCreateThread(_ context.Context, doctorID string) (*store.SupportThread, error) {
	f.createdFor = doctorID
	if f.thread != nil {
		return f.thread, nil
	}
	return &store.SupportThread{ID: "th-1", DoctorID: doctorID, Status: "open"}, nil
}

func (f *fakeSupportRepo) ListThreads(_ context.Context, _, _ int) ([]store.SupportThreadListItem, int, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	return f.threads, len(f.threads), nil
}

func (f *fakeSupportRepo) ListMessages(_ context.Context, _ string) ([]store.SupportMessage, error) {
	return f.messages, nil
}

func (f *fakeSupportRepo) AddMessage(_ context.Context, threadID, senderID, senderRole, body string, files []store.SupportAttachmentUpload) (*store.SupportMessage, error) {
	if f.addErr != nil {
		return nil, f.addErr
	}
	f.lastBody = body
	f.lastFiles = files
	f.lastRole = senderRole
	return &store.SupportMessage{
		ID: "m-1", ThreadID: threadID, SenderID: senderID,
		SenderRole: senderRole, SenderName: "Тест", Body: body, CreatedAt: time.Now(),
	}, nil
}

func (f *fakeSupportRepo) GetAttachment(_ context.Context, id string) (*store.SupportAttachmentFile, error) {
	if f.attachment == nil || f.attachment.ID != id {
		return nil, store.ErrNotFound
	}
	return f.attachment, nil
}

func (f *fakeSupportRepo) MarkRead(_ context.Context, _ string, who string) error {
	f.markedWho = who
	return nil
}

func (f *fakeSupportRepo) SupportSummary(_ context.Context) (*store.SupportSummary, error) {
	if f.summary == nil {
		return &store.SupportSummary{}, nil
	}
	return f.summary, nil
}

func TestSupportThread_Empty(t *testing.T) {
	h := newSupportThreadHandler(&fakeSupportRepo{getErr: store.ErrNotFound})
	req := withDoctor(httptest.NewRequest(http.MethodGet, "/api/v1/support/thread", nil), "doc-1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data struct {
			Status   string `json:"status"`
			Unread   int    `json:"unread"`
			Messages []any  `json:"messages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("json: %v", err)
	}
	if env.Data.Status != "none" || env.Data.Unread != 0 {
		t.Errorf("want empty thread, got %+v", env.Data)
	}
}

func TestSupportSend_CreatesAndSends(t *testing.T) {
	repo := &fakeSupportRepo{}
	h := newSupportSendHandler(repo)
	req := withDoctor(httptest.NewRequest(http.MethodPost, "/api/v1/support/messages",
		strings.NewReader(`{"body":"Не генерируется дневник"}`)), "doc-1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if repo.createdFor != "doc-1" {
		t.Errorf("thread not created for doctor, got %q", repo.createdFor)
	}
	if repo.lastRole != "user" || repo.lastBody != "Не генерируется дневник" {
		t.Errorf("unexpected message role=%q body=%q", repo.lastRole, repo.lastBody)
	}
	if repo.markedWho != "user" {
		t.Errorf("should mark user read, got %q", repo.markedWho)
	}
}

func TestSupportSend_EmptyBody(t *testing.T) {
	h := newSupportSendHandler(&fakeSupportRepo{})
	req := withDoctor(httptest.NewRequest(http.MethodPost, "/api/v1/support/messages",
		strings.NewReader(`{"body":"   "}`)), "doc-1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestAdminSupportReply(t *testing.T) {
	repo := &fakeSupportRepo{
		thread: &store.SupportThread{ID: "th-1", DoctorID: "doc-1", Status: "open"},
	}
	h := newAdminSupportReplyHandler(repo)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/support/threads/th-1/messages",
		strings.NewReader(`{"body":"Сейчас посмотрим"}`))
	req.SetPathValue("id", "th-1")
	req = withAdmin(req, "admin-1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastRole != "support" {
		t.Errorf("admin reply role=%q, want support", repo.lastRole)
	}
}

func TestAdminSupportList(t *testing.T) {
	repo := &fakeSupportRepo{
		threads: []store.SupportThreadListItem{{
			SupportThread: store.SupportThread{ID: "th-1", UnreadByAdmin: 2},
			DoctorName:    "Иванов",
			DoctorEmail:   "i@x.ru",
		}},
	}
	h := newAdminSupportListHandler(repo)
	req := withAdmin(httptest.NewRequest(http.MethodGet, "/api/v1/admin/support/threads", nil), "admin-1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("json: %v", err)
	}
	if env.Meta.Total != 1 || len(env.Data) != 1 {
		t.Errorf("unexpected list: %+v", env)
	}
}

// pngHeader is enough bytes for http.DetectContentType to say image/png.
var pngHeader = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

func multipartMessage(t *testing.T, body string, files map[string][]byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if body != "" {
		_ = mw.WriteField("body", body)
	}
	for name, data := range files {
		part, err := mw.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(data)
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

func TestSupportSend_ImageWithoutText(t *testing.T) {
	repo := &fakeSupportRepo{}
	h := newSupportSendHandler(repo)
	buf, ct := multipartMessage(t, "", map[string][]byte{"скрин.png": pngHeader})
	req := withDoctor(httptest.NewRequest(http.MethodPost, "/api/v1/support/messages", buf), "doc-1")
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(repo.lastFiles) != 1 {
		t.Fatalf("files=%d, want 1", len(repo.lastFiles))
	}
	f := repo.lastFiles[0]
	if f.Filename != "скрин.png" || f.ContentType != "image/png" {
		t.Errorf("file=%q type=%q", f.Filename, f.ContentType)
	}
}

func TestSupportSend_SpoofedTypeIsNotImage(t *testing.T) {
	repo := &fakeSupportRepo{}
	h := newSupportSendHandler(repo)
	buf, ct := multipartMessage(t, "вот", map[string][]byte{
		"evil.png": []byte("<html><script>alert(1)</script></html>"),
	})
	req := withDoctor(httptest.NewRequest(http.MethodPost, "/api/v1/support/messages", buf), "doc-1")
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := repo.lastFiles[0].ContentType; got != "application/octet-stream" {
		t.Errorf("content type=%q, want octet-stream", got)
	}
}

func TestSupportSend_TooManyFiles(t *testing.T) {
	files := map[string][]byte{}
	for i := 0; i <= supportMaxFiles; i++ {
		files["f"+string(rune('a'+i))+".txt"] = []byte("x")
	}
	buf, ct := multipartMessage(t, "", files)
	req := withDoctor(httptest.NewRequest(http.MethodPost, "/api/v1/support/messages", buf), "doc-1")
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	newSupportSendHandler(&fakeSupportRepo{})(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestSupportAttachment_OwnerOnly(t *testing.T) {
	repo := &fakeSupportRepo{attachment: &store.SupportAttachmentFile{
		SupportAttachment: store.SupportAttachment{ID: "a-1", Filename: "скрин.png", ContentType: "image/png"},
		DoctorID:          "doc-1",
		Data:              pngHeader,
	}}

	get := func(h http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
		req.SetPathValue("id", "a-1")
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec
	}

	rec := get(newSupportAttachmentHandler(repo, false),
		withDoctor(httptest.NewRequest(http.MethodGet, "/api/v1/support/attachments/a-1", nil), "doc-1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("owner status=%d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type=%q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "inline") {
		t.Errorf("Content-Disposition=%q, want inline", cd)
	}
	if !bytes.Equal(rec.Body.Bytes(), pngHeader) {
		t.Errorf("body mismatch")
	}

	rec = get(newSupportAttachmentHandler(repo, false),
		withDoctor(httptest.NewRequest(http.MethodGet, "/api/v1/support/attachments/a-1", nil), "doc-2"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("stranger status=%d, want 404", rec.Code)
	}

	rec = get(newSupportAttachmentHandler(repo, true),
		withAdmin(httptest.NewRequest(http.MethodGet, "/api/v1/admin/support/attachments/a-1", nil), "admin-1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("admin status=%d", rec.Code)
	}
}

func TestSanitizeSupportFilename(t *testing.T) {
	cases := map[string]string{
		"../../etc/passwd":     "passwd",
		`C:\Users\a\скрин.png`: "скрин.png",
		"  ":                   "файл",
		"a\x00b.txt":           "ab.txt",
	}
	for in, want := range cases {
		if got := sanitizeSupportFilename(in); got != want {
			t.Errorf("sanitize(%q)=%q, want %q", in, got, want)
		}
	}
}
