package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aimed/gateway/internal/anonymizer"
)

// fakeAnon lets us drive handler branches deterministically without depending
// on detector behaviour.
type fakeAnon struct {
	res anonymizer.Result
	err error
}

func (f fakeAnon) Anonymize(_ context.Context, _ string) (anonymizer.Result, error) {
	return f.res, f.err
}

func TestAnonymizeHandler(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		anon       anonymizer.Anonymizer
		wantStatus int
		wantCode   string // error code when error envelope expected
	}{
		{
			name:       "bad_json",
			body:       "{not json",
			anon:       fakeAnon{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name:       "empty_text",
			body:       `{"text":""}`,
			anon:       fakeAnon{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "clean_ok",
			body: `{"text":"Осмотр сегодня."}`,
			anon: fakeAnon{res: anonymizer.Result{
				Text: "Осмотр [ДАТА].", RemovedCount: 1,
				RemovedByType: map[anonymizer.EntityType]int{anonymizer.EntityDate: 1},
				Clean:         true,
			}},
			wantStatus: http.StatusOK,
		},
		{
			name: "pii_blocked_422",
			body: `{"text":"Иванов Иван Иванович."}`,
			anon: fakeAnon{res: anonymizer.Result{
				Text: "[ФИО]", Clean: false,
				Suspicions: []anonymizer.Suspicion{{Type: anonymizer.EntityPerson}},
			}},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "PII_DETECTED",
		},
		{
			name:       "pipeline_error_fail_closed",
			body:       `{"text":"что-то"}`,
			anon:       fakeAnon{err: anonymizer.ErrPIIDetected},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "PII_DETECTED",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newAnonymizeHandler(tc.anon)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/anonymize", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			h(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status=%d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}

			var env envelope
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("invalid JSON envelope: %v; body=%s", err, rec.Body.String())
			}
			if tc.wantCode != "" {
				if env.Error == nil || env.Error.Code != tc.wantCode {
					t.Errorf("ожидался error.code=%s; got %+v", tc.wantCode, env.Error)
				}
			} else if env.Error != nil {
				t.Errorf("неожиданная ошибка в ответе: %+v", env.Error)
			}
		})
	}
}
