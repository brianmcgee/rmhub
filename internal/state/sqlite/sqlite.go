// Package sqlite is the SQLite backend of the state store.
//
// A version is the lowercase hex SHA-256 of the value.
package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/brianmcgee/rmhub/internal/state"

	_ "modernc.org/sqlite" // database/sql driver "sqlite"
)

// Store keeps values in one SQLite table.
type Store struct {
	db *sql.DB
}

var _ state.Store = (*Store)(nil)

// Open opens or creates the database file at path.
func Open(path string) (*Store, error) {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(FULL)")
	q.Set("_txlock", "immediate")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("state store: %w", err)
	}
	const schema = `CREATE TABLE IF NOT EXISTS kv (
		key TEXT PRIMARY KEY,
		value BLOB NOT NULL,
		version TEXT NOT NULL
	)`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("state store %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func version(value []byte) string {
	h := sha256.Sum256(value)
	return hex.EncodeToString(h[:])
}

// Get reads a key.
func (s *Store) Get(ctx context.Context, key string) ([]byte, string, error) {
	if err := state.CheckKey(key); err != nil {
		return nil, "", err
	}
	var value []byte
	var ver string
	err := s.db.QueryRowContext(ctx, `SELECT value, version FROM kv WHERE key = ?`, key).Scan(&value, &ver)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", state.ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	return value, ver, nil
}

// PutIfAbsent inserts a key.
func (s *Store) PutIfAbsent(ctx context.Context, key string, value []byte) (string, error) {
	if err := state.CheckKey(key); err != nil {
		return "", err
	}
	ver := version(value)
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO kv (key, value, version) VALUES (?, ?, ?) ON CONFLICT (key) DO NOTHING`,
		key, value, ver)
	return ver, changed(res, err)
}

// PutIfMatch updates a key with the given version.
func (s *Store) PutIfMatch(ctx context.Context, key string, value []byte, ver string) (string, error) {
	if err := state.CheckKey(key); err != nil {
		return "", err
	}
	nv := version(value)
	res, err := s.db.ExecContext(ctx,
		`UPDATE kv SET value = ?, version = ? WHERE key = ? AND version = ?`,
		value, nv, key, ver)
	return nv, changed(res, err)
}

func changed(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return state.ErrConflict
	}
	return nil
}

// Delete removes a key.
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := state.CheckKey(key); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM kv WHERE key = ?`, key)
	return err
}

// List returns keys with a prefix in byte order.
func (s *Store) List(ctx context.Context, prefix, after string, limit int) ([]string, error) {
	if err := state.CheckPrefix(prefix); err != nil {
		return nil, err
	}
	var (
		where []string
		args  []any
	)
	if prefix != "" {
		// Keys hold only ASCII, so the prefix plus 0xff is above each key
		// with the prefix.
		where = append(where, "key >= ?", "key < ?")
		args = append(args, prefix, prefix+"\xff")
	}
	if after != "" {
		where = append(where, "key > ?")
		args = append(args, after)
	}
	q := `SELECT key FROM kv`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY key"
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
