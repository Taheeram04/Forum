package posts_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"forum/internal"
	"forum/internal/auth"
	"forum/internal/db"
	"forum/internal/posts"
	"forum/internal/reactions"
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

// registerAndGetCookie creates a user via the real registration flow and
// returns its session cookie, so downstream requests carry a context
// populated the same way auth.Middleware does in production.
func registerAndGetCookie(t *testing.T, authSvc *auth.Service, username string) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email": username + "@example.com", "username": username, "password": "supersecret",
	})
	req := httptest.NewRequest("POST", "/api/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	authSvc.Register(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("registering %s: expected 201, got %d: %s", username, rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("registering %s: expected a session cookie", username)
	}
	return cookies[0]
}

// authedRequest builds a request carrying the given session cookie, routed
// through the real auth middleware so the request context is populated
// exactly as it would be in production.
func authedRequest(authSvc *auth.Service, method, path string, body []byte, cookie *http.Cookie, handler func(w http.ResponseWriter, r *http.Request)) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	authSvc.Middleware(http.HandlerFunc(handler)).ServeHTTP(rec, req)
	return rec
}

func TestCreatePostRequiresCategory(t *testing.T) {
	conn := newTestDB(t)
	authSvc := auth.NewService(conn)
	postSvc := posts.NewService(conn)
	cookie := registerAndGetCookie(t, authSvc, "alice")

	body, _ := json.Marshal(map[string]any{"title": "Hello", "body": "World", "category_ids": []int{}})
	rec := authedRequest(authSvc, "POST", "/api/posts", body, cookie, postSvc.Create)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 with no categories, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFilterByCategoryMineAndLiked(t *testing.T) {
	conn := newTestDB(t)
	authSvc := auth.NewService(conn)
	postSvc := posts.NewService(conn)
	reactSvc := reactions.NewService(conn)

	aliceCookie := registerAndGetCookie(t, authSvc, "alice")
	bobCookie := registerAndGetCookie(t, authSvc, "bob")

	createPost := func(cookie *http.Cookie, title string, catIDs []int) string {
		body, _ := json.Marshal(map[string]any{"title": title, "body": "body text", "category_ids": catIDs})
		rec := authedRequest(authSvc, "POST", "/api/posts", body, cookie, postSvc.Create)
		if rec.Code != http.StatusCreated {
			t.Fatalf("creating post: expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]string
		json.Unmarshal(rec.Body.Bytes(), &resp)
		return resp["id"]
	}

	// Category IDs come from the seeded defaults: 1=General, 2=Technology.
	techPost := createPost(aliceCookie, "Alice on Tech", []int{2})
	generalPost := createPost(bobCookie, "Bob on General", []int{1})

	// Filter by category=2 should return only techPost.
	rec := authedRequest(authSvc, "GET", "/api/posts?category=2", nil, aliceCookie, postSvc.List)
	var results []internal.Post
	json.Unmarshal(rec.Body.Bytes(), &results)
	if len(results) != 1 || results[0].ID != techPost {
		t.Fatalf("category filter: expected only %s, got %+v", techPost, results)
	}

	// mine=1 for bob should return only generalPost.
	rec = authedRequest(authSvc, "GET", "/api/posts?mine=1", nil, bobCookie, postSvc.List)
	results = nil
	json.Unmarshal(rec.Body.Bytes(), &results)
	if len(results) != 1 || results[0].ID != generalPost {
		t.Fatalf("mine filter: expected only %s, got %+v", generalPost, results)
	}

	// mine/liked without auth must be rejected.
	rec = authedRequest(authSvc, "GET", "/api/posts?mine=1", nil, nil, postSvc.List)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for mine filter without auth, got %d", rec.Code)
	}

	// Bob likes Alice's tech post; liked=1 for bob should return it.
	rec = authedRequest(authSvc, "POST", "/api/posts/"+techPost+"/react", []byte(`{"value":1}`), bobCookie,
		func(w http.ResponseWriter, r *http.Request) { reactSvc.ReactToPost(w, r, techPost) })
	if rec.Code != http.StatusOK {
		t.Fatalf("liking post: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = authedRequest(authSvc, "GET", "/api/posts?liked=1", nil, bobCookie, postSvc.List)
	results = nil
	json.Unmarshal(rec.Body.Bytes(), &results)
	if len(results) != 1 || results[0].ID != techPost {
		t.Fatalf("liked filter: expected only %s, got %+v", techPost, results)
	}
	if results[0].Likes != 1 {
		t.Fatalf("expected like count 1, got %d", results[0].Likes)
	}
}
