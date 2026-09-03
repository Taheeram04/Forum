// Package categories exposes the fixed list of forum categories
// ("subforums") that posts can be filed under.
package categories

import (
	"database/sql"
	"net/http"

	"forum/internal"
	"forum/internal/httputil"
)

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) List(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT id, name FROM categories ORDER BY name`)
	if err != nil {
		httputil.ServerError(w, err)
		return
	}
	defer rows.Close()

	cats := []internal.Category{}
	for rows.Next() {
		var c internal.Category
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			httputil.ServerError(w, err)
			return
		}
		cats = append(cats, c)
	}
	httputil.JSON(w, http.StatusOK, cats)
}
