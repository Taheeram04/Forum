-- Forum database schema.
-- Applied automatically at startup (see db.go) via CREATE TABLE IF NOT EXISTS,
-- so the app is safe to restart against an existing forum.db.

PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    email         TEXT UNIQUE NOT NULL,
    username      TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);

CREATE TABLE IF NOT EXISTS categories (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS posts (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_posts_user ON posts(user_id);
CREATE INDEX IF NOT EXISTS idx_posts_created ON posts(created_at DESC);

CREATE TABLE IF NOT EXISTS post_categories (
    post_id     TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    PRIMARY KEY (post_id, category_id)
);
CREATE INDEX IF NOT EXISTS idx_post_categories_cat ON post_categories(category_id);

CREATE TABLE IF NOT EXISTS comments (
    id         TEXT PRIMARY KEY,
    post_id    TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body       TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_comments_post ON comments(post_id);

-- One row per (user, target). value is +1 (like) or -1 (dislike).
-- Exactly one of post_id / comment_id is set, enforced in application code
-- (SQLite CHECK constraints can't easily express XOR across two nullable FKs
-- without more complexity than it's worth here).
CREATE TABLE IF NOT EXISTS reactions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    post_id    TEXT REFERENCES posts(id) ON DELETE CASCADE,
    comment_id TEXT REFERENCES comments(id) ON DELETE CASCADE,
    value      INTEGER NOT NULL CHECK (value IN (1, -1))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_reactions_user_post
    ON reactions(user_id, post_id) WHERE post_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_reactions_user_comment
    ON reactions(user_id, comment_id) WHERE comment_id IS NOT NULL;

-- Seed a handful of default categories (subforums). Safe to run repeatedly.
INSERT OR IGNORE INTO categories (name) VALUES
    ('General'),
    ('Technology'),
    ('Off-Topic'),
    ('Help & Support'),
    ('Announcements');
