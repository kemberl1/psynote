package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aimed/gateway/internal/auth"
)

func testAuthDeps(t *testing.T) (authDeps, *fakeRepo) {
	t.Helper()
	ts, err := auth.NewTokenService("test-secret-for-handlers", 15*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatalf("token service: %v", err)
	}
	repo := &fakeRepo{}
	return authDeps{repo: repo, tokens: ts, params: fastArgon}, repo
}

// fastArgon keeps register hashing quick in handler tests.
var fastArgon = auth.Argon2Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

func doRegister(t *testing.T, d authDeps, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := newRegisterHandler(d)
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body)))
	return rec
}

func TestRegister_Success(t *testing.T) {
	d, repo := testAuthDeps(t)
	rec := doRegister(t, d, `{"email":"Doc@Example.com","password":"longenough1","display_name":"Врач Т."}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Email must be normalized (lowercased) and password stored as Argon2id hash.
	d2, ok := repo.doctorsByEmail["doc@example.com"]
	if !ok {
		t.Fatalf("doctor not stored under normalized email; have %v", repo.doctorsByEmail)
	}
	if !strings.HasPrefix(d2.PasswordHash, "$argon2id$") {
		t.Errorf("password not hashed with argon2id: %q", d2.PasswordHash)
	}
	if strings.Contains(d2.PasswordHash, "longenough1") {
		t.Error("plaintext password leaked into stored hash")
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	d, _ := testAuthDeps(t)
	first := doRegister(t, d, `{"email":"dup@example.com","password":"longenough1","display_name":"A"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("first register status=%d", first.Code)
	}
	second := doRegister(t, d, `{"email":"dup@example.com","password":"longenough1","display_name":"B"}`)
	if second.Code != http.StatusConflict {
		t.Fatalf("duplicate email status=%d, want 409", second.Code)
	}
	var env envelope
	_ = json.Unmarshal(second.Body.Bytes(), &env)
	if env.Error == nil || env.Error.Code != "EMAIL_TAKEN" {
		t.Errorf("want EMAIL_TAKEN, got %+v", env.Error)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	d, _ := testAuthDeps(t)
	rec := doRegister(t, d, `{"email":"x@example.com","password":"short","display_name":"A"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestRegister_BadEmail(t *testing.T) {
	d, _ := testAuthDeps(t)
	rec := doRegister(t, d, `{"email":"not-an-email","password":"longenough1","display_name":"A"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestLogin_SuccessAndWrongPassword(t *testing.T) {
	d, _ := testAuthDeps(t)
	if rec := doRegister(t, d, `{"email":"u@example.com","password":"longenough1","display_name":"U"}`); rec.Code != http.StatusCreated {
		t.Fatalf("register failed: %d", rec.Code)
	}

	// Correct credentials → 200 + token pair.
	loginH := newLoginHandler(d)
	rec := httptest.NewRecorder()
	loginH(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"email":"u@example.com","password":"longenough1"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data tokenData `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data.AccessToken == "" || env.Data.RefreshToken == "" || env.Data.ExpiresIn == 0 {
		t.Errorf("token pair incomplete: %+v", env.Data)
	}
	// Access token must verify and carry the doctor_id.
	claims, err := d.tokens.ParseAccessToken(env.Data.AccessToken)
	if err != nil || claims.DoctorID == "" {
		t.Errorf("issued access token invalid: %v / %+v", err, claims)
	}

	// Wrong password → 401.
	rec = httptest.NewRecorder()
	loginH(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"email":"u@example.com","password":"WRONGpassword"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status=%d, want 401", rec.Code)
	}
}

func TestLogin_UnknownEmail(t *testing.T) {
	d, _ := testAuthDeps(t)
	loginH := newLoginHandler(d)
	rec := httptest.NewRecorder()
	loginH(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"email":"ghost@example.com","password":"longenough1"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rec.Code)
	}
}

// loginAndGetTokens registers + logs in, returning the issued token pair.
func loginAndGetTokens(t *testing.T, d authDeps, email string) tokenData {
	t.Helper()
	if rec := doRegister(t, d, `{"email":"`+email+`","password":"longenough1","display_name":"U"}`); rec.Code != http.StatusCreated {
		t.Fatalf("register: %d", rec.Code)
	}
	loginH := newLoginHandler(d)
	rec := httptest.NewRecorder()
	loginH(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"email":"`+email+`","password":"longenough1"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d", rec.Code)
	}
	var env struct {
		Data tokenData `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return env.Data
}

func TestRefresh_RotatesAndInvalidatesOld(t *testing.T) {
	d, _ := testAuthDeps(t)
	tokens := loginAndGetTokens(t, d, "rot@example.com")

	refreshH := newRefreshHandler(d)
	rec := httptest.NewRecorder()
	refreshH(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		strings.NewReader(`{"refresh_token":"`+tokens.RefreshToken+`"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data tokenData `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data.RefreshToken == "" || env.Data.RefreshToken == tokens.RefreshToken {
		t.Error("refresh did not rotate the token")
	}

	// Reusing the OLD refresh token must now fail (rotation revoked it).
	rec = httptest.NewRecorder()
	refreshH(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		strings.NewReader(`{"refresh_token":"`+tokens.RefreshToken+`"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("reused old refresh token status=%d, want 401", rec.Code)
	}
}

func TestRefresh_InvalidToken(t *testing.T) {
	d, _ := testAuthDeps(t)
	refreshH := newRefreshHandler(d)
	rec := httptest.NewRecorder()
	refreshH(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		strings.NewReader(`{"refresh_token":"bogus"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rec.Code)
	}
}

func TestLogout_RevokesSession(t *testing.T) {
	d, _ := testAuthDeps(t)
	tokens := loginAndGetTokens(t, d, "out@example.com")

	logoutH := newLogoutHandler(d)
	rec := httptest.NewRecorder()
	logoutH(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout",
		strings.NewReader(`{"refresh_token":"`+tokens.RefreshToken+`"}`)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d, want 204", rec.Code)
	}

	// After logout the refresh token must no longer work.
	refreshH := newRefreshHandler(d)
	rec = httptest.NewRecorder()
	refreshH(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		strings.NewReader(`{"refresh_token":"`+tokens.RefreshToken+`"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("refresh after logout status=%d, want 401", rec.Code)
	}
}

func TestMe_ReturnsProfile(t *testing.T) {
	d, _ := testAuthDeps(t)
	tokens := loginAndGetTokens(t, d, "me@example.com")
	claims, _ := d.tokens.ParseAccessToken(tokens.AccessToken)

	meH := requireAuth(d.tokens, newMeHandler(d.repo))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	rec := httptest.NewRecorder()
	meH(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me status=%d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data meData `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data.DoctorID != claims.DoctorID || env.Data.Email != "me@example.com" {
		t.Errorf("me profile wrong: %+v", env.Data)
	}
}

// ─── middleware ──────────────────────────────────────────────────────────────

func TestRequireAuth_NoToken(t *testing.T) {
	ts, _ := auth.NewTokenService("k", time.Minute, time.Hour)
	called := false
	h := requireAuth(ts, func(http.ResponseWriter, *http.Request) { called = true })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rec.Code)
	}
	if called {
		t.Error("downstream handler must not run without a valid token")
	}
}

func TestRequireAuth_ValidToken_InjectsDoctorID(t *testing.T) {
	ts, _ := auth.NewTokenService("k", time.Minute, time.Hour)
	access, _ := ts.IssueAccessToken("doc-77", "doctor")
	var seen string
	h := requireAuth(ts, func(_ http.ResponseWriter, r *http.Request) {
		if id, ok := doctorIDFromContext(r.Context()); ok {
			seen = id
		}
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	rec := httptest.NewRecorder()
	h(rec, req)
	if seen != "doc-77" {
		t.Errorf("doctor_id not injected into context, got %q", seen)
	}
}

func TestRequireAuth_BadToken(t *testing.T) {
	ts, _ := auth.NewTokenService("k", time.Minute, time.Hour)
	h := requireAuth(ts, func(http.ResponseWriter, *http.Request) {})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer not.a.jwt")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rec.Code)
	}
}
