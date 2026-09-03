// Package internal holds domain types shared across feature packages
// (auth, posts, comments, reactions) to avoid import cycles.
package internal

import "time"

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

type Category struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Post struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Username   string     `json:"username"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	CreatedAt  time.Time  `json:"created_at"`
	Categories []Category `json:"categories"`
	Likes      int        `json:"likes"`
	Dislikes   int        `json:"dislikes"`
	// UserReaction is the requesting user's own reaction to this post:
	// 1 (liked), -1 (disliked), or 0 (none / not logged in).
	UserReaction int `json:"user_reaction"`
}

type Comment struct {
	ID           string    `json:"id"`
	PostID       string    `json:"post_id"`
	UserID       string    `json:"user_id"`
	Username     string    `json:"username"`
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"created_at"`
	Likes        int       `json:"likes"`
	Dislikes     int       `json:"dislikes"`
	UserReaction int       `json:"user_reaction"`
}
