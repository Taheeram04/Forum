// Package db owns the SQLite connection and schema bootstrap.
package db

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)


var schemaSQL string


func Open(path string) (*sql.DB, error) {
	// _foreign_keys=on is required per-connection; SQLite does not persist it.
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL", path)

	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// SQLite only supports one writer at a time; a single connection avoids
	// "database is locked" errors under concurrent requests.
	conn.SetMaxOpenConns(1)

	if _, err := conn.Exec(schemaSQL); err != nil {
		conn.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}

	return conn, nil
}
