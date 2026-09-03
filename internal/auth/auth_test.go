package auth_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"forum/internal/auth"
	"forum/internal/db"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func doJSON(t *testing.T, h http.HandlerFunc, method, path string, body any, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encoding body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestRegisterThenLogin(t *testing.T) {
	conn := newTestDB(t)
	svc := auth.NewService(conn)

	rec := doJSON(t, svc.Register, "POST", "/api/register", map[string]string{
		"email": "ada@example.com", "username": "ada", "password": "supersecret",
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Duplicate email must be rejected.
	rec = doJSON(t, svc.Register, "POST", "/api/register", map[string]string{
		"email": "ada@example.com", "username": "ada2", "password": "supersecret",
	}, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate register: expected 409, got %d", rec.Code)
	}

	// Wrong password must be rejected.
	rec = doJSON(t, svc.Login, "POST", "/api/login", map[string]string{
		"email": "ada@example.com", "password": "wrongpassword",
	}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login: expected 401, got %d", rec.Code)
	}

	// Correct password must succeed and set a session cookie.
	rec = doJSON(t, svc.Login, "POST", "/api/login", map[string]string{
		"email": "ada@example.com", "password": "supersecret",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login: expected a session cookie to be set")
	}
}

func TestMiddlewareAttachesUser(t *testing.T) {
	conn := newTestDB(t)
	svc := auth.NewService(conn)

	rec := doJSON(t, svc.Register, "POST", "/api/register", map[string]string{
		"email": "grace@example.com", "username": "grace", "password": "supersecret",
	}, nil)
	cookies := rec.Result().Cookies()

	var sawUser bool
	protected := auth.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		u, ok := auth.UserFromContext(r.Context())
		sawUser = ok && u.Username == "grace"
		w.WriteHeader(http.StatusOK)
	})

	wrapped := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protected(w, r)
	}))

	req := httptest.NewRequest("GET", "/api/me", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec2, req)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 for authenticated request, got %d", rec2.Code)
	}
	if !sawUser {
		t.Fatal("expected middleware to attach the logged-in user to the request context")
	}

	// A request with no cookie must be rejected by RequireAuth.
	req2 := httptest.NewRequest("GET", "/api/me", nil)
	rec3 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec3, req2)
	if rec3.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for guest request, got %d", rec3.Code)
	}
}
