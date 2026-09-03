// Command server is the forum's HTTP entrypoint: it wires the SQLite
// database, all feature services, and routes, then serves the API plus
// the static frontend.
package main

import (
	"log"
	"net/http"
	"os"

	"forum/internal/auth"
	"forum/internal/categories"
	"forum/internal/comments"
	"forum/internal/db"
	"forum/internal/httputil"
	"forum/internal/posts"
	"forum/internal/reactions"
)

func main() {
	dbPath := getenv("DB_PATH", "./data/forum.db")
	if err := os.MkdirAll(dirOf(dbPath), 0o755); err != nil {
		log.Fatalf("creating db directory: %v", err)
	}

	conn, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("opening database: %v", err)
	}
	defer conn.Close()

	authSvc := auth.NewService(conn)
	catSvc := categories.NewService(conn)
	postSvc := posts.NewService(conn)
	commentSvc := comments.NewService(conn)
	reactSvc := reactions.NewService(conn)

	mux := http.NewServeMux()

	// -- auth --
	mux.HandleFunc("POST /api/register", authSvc.Register)
	mux.HandleFunc("POST /api/login", authSvc.Login)
	mux.HandleFunc("POST /api/logout", authSvc.Logout)
	mux.HandleFunc("GET /api/me", authSvc.Me)

	// -- categories (public) --
	mux.HandleFunc("GET /api/categories", catSvc.List)

	// -- posts --
	mux.HandleFunc("GET /api/posts", postSvc.List)
	mux.HandleFunc("GET /api/posts/{id}", func(w http.ResponseWriter, r *http.Request) {
		postSvc.Get(w, r, r.PathValue("id"))
	})
	mux.HandleFunc("POST /api/posts", auth.RequireAuth(postSvc.Create))

	// -- comments (nested under a post) --
	mux.HandleFunc("GET /api/posts/{id}/comments", func(w http.ResponseWriter, r *http.Request) {
		commentSvc.List(w, r, r.PathValue("id"))
	})
	mux.HandleFunc("POST /api/posts/{id}/comments", auth.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		commentSvc.Create(w, r, r.PathValue("id"))
	}))

	// -- reactions --
	mux.HandleFunc("POST /api/posts/{id}/react", auth.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		reactSvc.ReactToPost(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/comments/{id}/react", auth.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		reactSvc.ReactToComment(w, r, r.PathValue("id"))
	}))

	// Catch-all for unmatched /api/* routes, so clients get a JSON 404
	// instead of falling through to the static file server.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		httputil.Error(w, http.StatusNotFound, "not found")
	})

	// -- static frontend --
	mux.Handle("/", http.FileServer(http.Dir("./web/static")))

	handler := authSvc.Middleware(mux)

	addr := ":" + getenv("PORT", "8080")
	log.Printf("forum listening on %s (db: %s)", addr, dbPath)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

