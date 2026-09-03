// Package comments implements comment creation and listing, scoped to a
// single post.
package comments

import (
	"database/sql"
	"errors"
	"net/http"
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

type createRequest struct {
	Body string `json:"body"`
}

// Create handles POST /api/posts/{postID}/comments (auth required).
func (s *Service) Create(w http.ResponseWriter, r *http.Request, postID string) {
	user, _ := auth.UserFromContext(r.Context()) // guaranteed by RequireAuth

	var req createRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Body = strings.TrimSpace(req.Body)
	if req.Body == "" {
		httputil.Error(w, http.StatusBadRequest, "comment body is required")
		return
	}

	var exists bool
	if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM posts WHERE id = ?)`, postID).Scan(&exists); err != nil {
		httputil.ServerError(w, err)
		return
	}
	if !exists {
		httputil.Error(w, http.StatusNotFound, "post not found")
		return
	}

	id, err := uuid.NewV4()
	if err != nil {
		httputil.ServerError(w, err)
		return
	}

	if _, err := s.db.Exec(
		`INSERT INTO comments (id, post_id, user_id, body) VALUES (?, ?, ?, ?)`,
		id.String(), postID, user.ID, req.Body,
	); err != nil {
		httputil.ServerError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]string{"id": id.String()})
}

// List handles GET /api/posts/{postID}/comments — visible to everyone.
func (s *Service) List(w http.ResponseWriter, r *http.Request, postID string) {
	user, loggedIn := auth.UserFromContext(r.Context())

	rows, err := s.db.Query(`
		SELECT c.id, c.post_id, c.user_id, u.username, c.body, c.created_at
		FROM comments c JOIN users u ON u.id = c.user_id
		WHERE c.post_id = ?
		ORDER BY c.created_at ASC`, postID)
	if err != nil {
		httputil.ServerError(w, err)
		return
	}
	defer rows.Close()

	result := []internal.Comment{}
	for rows.Next() {
		var c internal.Comment
		if err := rows.Scan(&c.ID, &c.PostID, &c.UserID, &c.Username, &c.Body, &c.CreatedAt); err != nil {
			httputil.ServerError(w, err)
			return
		}
		result = append(result, c)
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
		if err := s.attachReactions(&result[i], currentUserID); err != nil {
			httputil.ServerError(w, err)
			return
		}
	}

	httputil.JSON(w, http.StatusOK, result)
}

func (s *Service) attachReactions(c *internal.Comment, currentUserID string) error {
	if err := s.db.QueryRow(
		`SELECT COALESCE(SUM(value = 1), 0), COALESCE(SUM(value = -1), 0) FROM reactions WHERE comment_id = ?`,
		c.ID,
	).Scan(&c.Likes, &c.Dislikes); err != nil {
		return err
	}

	if currentUserID == "" {
		return nil
	}
	var v int
	err := s.db.QueryRow(
		`SELECT value FROM reactions WHERE comment_id = ? AND user_id = ?`, c.ID, currentUserID,
	).Scan(&v)
	if err == nil {
		c.UserReaction = v
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}
