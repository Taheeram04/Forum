// Package posts implements post creation and the filtered post listing
// (by category, by author, by liked-by-user) that all posts.
package posts

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gofrs/uuid"

	"forum/internal"
	"forum/internal/auth"
	"forum/internal/httputil"
)

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// ---- create --------------------------------------------------------------

type createRequest struct {
	Title       string `json:"title"`
	Body        string `json:"body"`
	CategoryIDs []int  `json:"category_ids"`
}

func (s *Service) Create(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context()) // guaranteed by RequireAuth

	var req createRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)

	if req.Title == "" || req.Body == "" {
		httputil.Error(w, http.StatusBadRequest, "title and body are required")
		return
	}
	if len(req.CategoryIDs) == 0 {
		httputil.Error(w, http.StatusBadRequest, "at least one category is required")
		return
	}

	id, err := uuid.NewV4()
	if err != nil {
		httputil.ServerError(w, err)
		return
	}

	tx, err := s.db.Begin()
	if err != nil {
		httputil.ServerError(w, err)
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO posts (id, user_id, title, body) VALUES (?, ?, ?, ?)`,
		id.String(), user.ID, req.Title, req.Body,
	); err != nil {
		httputil.ServerError(w, err)
		return
	}

	catStmt, err := tx.Prepare(`INSERT INTO post_categories (post_id, category_id) VALUES (?, ?)`)
	if err != nil {
		httputil.ServerError(w, err)
		return
	}
	defer catStmt.Close()

	for _, cid := range req.CategoryIDs {
		if _, err := catStmt.Exec(id.String(), cid); err != nil {
			if isFKError(err) {
				httputil.Error(w, http.StatusBadRequest, "unknown category id")
				return
			}
			httputil.ServerError(w, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		httputil.ServerError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]string{"id": id.String()})
}

// ---- list / filter ---------------------------------------------------------

// List handles GET /api/posts with optional query params:
//
//	?category=<id>   posts filed under a given category
//	?mine=1          posts authored by the logged-in user (requires auth)
//	?liked=1         posts the logged-in user has liked (requires auth)
//
// With no params it returns all posts, newest first. Comments/likes are
// visible to everyone; mine/liked are scoped to the current session.
func (s *Service) List(w http.ResponseWriter, r *http.Request) {
	user, loggedIn := auth.UserFromContext(r.Context())
	q := r.URL.Query()

	if (q.Get("mine") == "1" || q.Get("liked") == "1") && !loggedIn {
		httputil.Error(w, http.StatusUnauthorized, "you must be logged in to use this filter")
		return
	}

	var (
		where []string
		args  []any
	)

	if catStr := q.Get("category"); catStr != "" {
		catID, err := strconv.Atoi(catStr)
		if err != nil {
			httputil.Error(w, http.StatusBadRequest, "category must be a number")
			return
		}
		where = append(where, `p.id IN (SELECT post_id FROM post_categories WHERE category_id = ?)`)
		args = append(args, catID)
	}

	if q.Get("mine") == "1" {
		where = append(where, `p.user_id = ?`)
		args = append(args, user.ID)
	}

	if q.Get("liked") == "1" {
		where = append(where, `p.id IN (SELECT post_id FROM reactions WHERE user_id = ? AND post_id IS NOT NULL AND value = 1)`)
		args = append(args, user.ID)
	}

	query := `
		SELECT p.id, p.user_id, u.username, p.title, p.body, p.created_at
		FROM posts p
		JOIN users u ON u.id = p.user_id`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY p.created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		httputil.ServerError(w, err)
		return
	}
	defer rows.Close()

	result := []internal.Post{}
	for rows.Next() {
		var p internal.Post
		if err := rows.Scan(&p.ID, &p.UserID, &p.Username, &p.Title, &p.Body, &p.CreatedAt); err != nil {
			httputil.ServerError(w, err)
			return
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		httputil.ServerError(w, err)
		return
	}

	var currentUserID string
	if loggedIn {
		currentUserID = user.ID
	}
	for i := range result {
		if err := s.attachDetails(&result[i], currentUserID); err != nil {
			httputil.ServerError(w, err)
			return
		}
	}

	httputil.JSON(w, http.StatusOK, result)
}

// Get handles GET /api/posts/{id} — a single post with full details.
func (s *Service) Get(w http.ResponseWriter, r *http.Request, id string) {
	user, loggedIn := auth.UserFromContext(r.Context())

	var p internal.Post
	err := s.db.QueryRow(`
		SELECT p.id, p.user_id, u.username, p.title, p.body, p.created_at
		FROM posts p JOIN users u ON u.id = p.user_id
		WHERE p.id = ?`, id,
	).Scan(&p.ID, &p.UserID, &p.Username, &p.Title, &p.Body, &p.CreatedAt)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		httputil.Error(w, http.StatusNotFound, "post not found")
		return
	case err != nil:
		httputil.ServerError(w, err)
		return
	}

	var currentUserID string
	if loggedIn {
		currentUserID = user.ID
	}
	if err := s.attachDetails(&p, currentUserID); err != nil {
		httputil.ServerError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, p)
}

// attachDetails fills in categories, like/dislike counts, and the
// requesting user's own reaction (0 if not logged in or no reaction).
func (s *Service) attachDetails(p *internal.Post, currentUserID string) error {
	catRows, err := s.db.Query(`
		SELECT c.id, c.name FROM categories c
		JOIN post_categories pc ON pc.category_id = c.id
		WHERE pc.post_id = ? ORDER BY c.name`, p.ID)
	if err != nil {
		return err
	}
	defer catRows.Close()
	p.Categories = []internal.Category{}
	for catRows.Next() {
		var c internal.Category
		if err := catRows.Scan(&c.ID, &c.Name); err != nil {
			return err
		}
		p.Categories = append(p.Categories, c)
	}
	if err := catRows.Err(); err != nil {
		return err
	}

	if err := s.db.QueryRow(
		`SELECT COALESCE(SUM(value = 1), 0), COALESCE(SUM(value = -1), 0) FROM reactions WHERE post_id = ?`,
		p.ID,
	).Scan(&p.Likes, &p.Dislikes); err != nil {
		return err
	}

	if currentUserID != "" {
		var v int
		err := s.db.QueryRow(
			`SELECT value FROM reactions WHERE post_id = ? AND user_id = ?`, p.ID, currentUserID,
		).Scan(&v)
		if err == nil {
			p.UserReaction = v
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}

	return nil
}

func isFKError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
