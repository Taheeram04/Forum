// Package auth handles registration, login/logout, and cookie-based
// sessions with expiration, backed by SQLite.
package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"golang.org/x/crypto/bcrypt"

	"forum/internal"
	"forum/internal/httputil"
)

const (
	sessionCookieName = "forum_session"
	sessionTTL        = 24 * time.Hour
)

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// ---- context plumbing --------------------------------------------------

type ctxKey int

const userCtxKey ctxKey = 0

// UserFromContext returns the authenticated user for the request, or
// (nil, false) if the request is unauthenticated (i.e. a guest).
func UserFromContext(ctx context.Context) (*internal.User, bool) {
	u, ok := ctx.Value(userCtxKey).(*internal.User)
	return u, ok
}

// ---- HTTP handlers ------------------------------------------------------

type registerRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Service) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Username = strings.TrimSpace(req.Username)

	switch {
	case req.Email == "" || !emailRe.MatchString(req.Email):
		httputil.Error(w, http.StatusBadRequest, "a valid email is required")
		return
	case req.Username == "":
		httputil.Error(w, http.StatusBadRequest, "username is required")
		return
	case len(req.Password) < 8:
		httputil.Error(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		httputil.ServerError(w, err)
		return
	}

	id, err := uuid.NewV4()
	if err != nil {
		httputil.ServerError(w, err)
		return
	}

	_, err = s.db.Exec(
		`INSERT INTO users (id, email, username, password_hash) VALUES (?, ?, ?, ?)`,
		id.String(), req.Email, req.Username, string(hash),
	)
	if err != nil {
		if isUniqueConstraintErr(err) {
			httputil.Error(w, http.StatusConflict, "email or username already in use")
			return
		}
		httputil.ServerError(w, err)
		return
	}

	if err := s.startSession(w, id.String()); err != nil {
		httputil.ServerError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]string{
		"id": id.String(), "email": req.Email, "username": req.Username,
	})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	var id, hash string
	err := s.db.QueryRow(
		`SELECT id, password_hash FROM users WHERE email = ?`, req.Email,
	).Scan(&id, &hash)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		httputil.Error(w, http.StatusUnauthorized, "invalid email or password")
		return
	case err != nil:
		httputil.ServerError(w, err)
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		httputil.Error(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if err := s.startSession(w, id); err != nil {
		httputil.ServerError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "logged in"})
}

func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE id = ?`, c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// Me returns the current session's user, or 401 for guests. Handy for the
// frontend to check auth state on page load.
func (s *Service) Me(w http.ResponseWriter, r *http.Request) {
	u, ok := UserFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "not logged in")
		return
	}
	httputil.JSON(w, http.StatusOK, u)
}

// ---- sessions -------------------------------------------------------------

func (s *Service) startSession(w http.ResponseWriter, userID string) error {
	sid, err := uuid.NewV4()
	if err != nil {
		return err
	}
	expires := time.Now().Add(sessionTTL)

	// One active session per user: replace any existing one.
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err := s.db.Exec(
		`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`,
		sid.String(), userID, expires,
	); err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sid.String(),
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// Secure: true, // enable once served over HTTPS
	})
	return nil
}

// Middleware attaches the authenticated user to the request context when a
// valid, non-expired session cookie is present. It never rejects the
// request outright — guests are allowed through with no user in context,
// since posts/comments must remain visible to everyone. Use RequireAuth
// for handlers that need a logged-in user.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookieName)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		var user internal.User
		var expiresAt time.Time
		err = s.db.QueryRow(`
			SELECT u.id, u.email, u.username, u.created_at, se.expires_at
			FROM sessions se
			JOIN users u ON u.id = se.user_id
			WHERE se.id = ?`, c.Value,
		).Scan(&user.ID, &user.Email, &user.Username, &user.CreatedAt, &expiresAt)

		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		if time.Now().After(expiresAt) {
			_, _ = s.db.Exec(`DELETE FROM sessions WHERE id = ?`, c.Value)
			next.ServeHTTP(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), userCtxKey, &user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuth wraps a handler so it 401s any request without a valid
// session, e.g. creating posts, comments, or reactions.
func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserFromContext(r.Context()); !ok {
			httputil.Error(w, http.StatusUnauthorized, "you must be logged in")
			return
		}
		next(w, r)
	}
}

func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
