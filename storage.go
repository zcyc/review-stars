package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const databaseSchema = `
CREATE TABLE IF NOT EXISTS repositories (
    full_name TEXT PRIMARY KEY,
    payload_json TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    synced_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS reviews (
    full_name TEXT PRIMARY KEY REFERENCES repositories(full_name) ON DELETE CASCADE,
    payload_json TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    reviewed_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_reviews_reviewed_at ON reviews(reviewed_at);

CREATE TABLE IF NOT EXISTS rule_reviews (
    full_name TEXT PRIMARY KEY REFERENCES repositories(full_name) ON DELETE CASCADE,
    payload_json TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    reviewed_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_rule_reviews_reviewed_at ON rule_reviews(reviewed_at);
`

type SQLiteStore struct {
	db *sql.DB
}

func openSQLiteStore(filename string) (*SQLiteStore, error) {
	if strings.TrimSpace(filename) == "" {
		filename = "review-stars.db"
	}
	if filename != ":memory:" {
		directory := filepath.Dir(filename)
		if directory != "." {
			if err := os.MkdirAll(directory, 0755); err != nil {
				return nil, err
			}
		}
	}
	database, err := sql.Open("sqlite", filename)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	store := &SQLiteStore{db: database}
	if _, err := database.Exec("PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;"); err != nil {
		_ = database.Close()
		return nil, err
	}
	if _, err := database.Exec(databaseSchema); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("create sqlite schema: %w", err)
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) SaveRepositories(repos []Repository) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	rollback := func(err error) error {
		_ = tx.Rollback()
		return err
	}

	if len(repos) == 0 {
		if _, err := tx.Exec("DELETE FROM reviews"); err != nil {
			return rollback(err)
		}
		if _, err := tx.Exec("DELETE FROM rule_reviews"); err != nil {
			return rollback(err)
		}
		if _, err := tx.Exec("DELETE FROM repositories"); err != nil {
			return rollback(err)
		}
		return tx.Commit()
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(repos)), ",")
	args := make([]any, 0, len(repos))
	for _, repo := range repos {
		args = append(args, repo.FullName)
	}
	if _, err := tx.Exec("DELETE FROM reviews WHERE full_name NOT IN ("+placeholders+")", args...); err != nil {
		return rollback(err)
	}
	if _, err := tx.Exec("DELETE FROM rule_reviews WHERE full_name NOT IN ("+placeholders+")", args...); err != nil {
		return rollback(err)
	}
	if _, err := tx.Exec("DELETE FROM repositories WHERE full_name NOT IN ("+placeholders+")", args...); err != nil {
		return rollback(err)
	}

	statement, err := tx.Prepare(`
INSERT INTO repositories (full_name, payload_json, fingerprint, synced_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(full_name) DO UPDATE SET
    payload_json = excluded.payload_json,
    fingerprint = excluded.fingerprint,
    synced_at = excluded.synced_at`)
	if err != nil {
		return rollback(err)
	}
	defer statement.Close()
	syncedAt := time.Now().UTC().Format(time.RFC3339Nano)
	for _, repo := range repos {
		payload, err := json.Marshal(repo)
		if err != nil {
			return rollback(err)
		}
		if _, err := statement.Exec(repo.FullName, payload, repositoryFingerprint(repo), syncedAt); err != nil {
			return rollback(err)
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListRepositories() ([]Repository, error) {
	rows, err := s.db.Query(`SELECT payload_json FROM repositories ORDER BY full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	repos := make([]Repository, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var repo Repository
		if err := json.Unmarshal(payload, &repo); err != nil {
			return nil, fmt.Errorf("decode repository from sqlite: %w", err)
		}
		repos = append(repos, repo)
	}
	return repos, rows.Err()
}

func (s *SQLiteStore) GetReview(repo Repository) (RepositoryReview, bool, error) {
	var payload []byte
	var fingerprint string
	err := s.db.QueryRow(`SELECT payload_json, fingerprint FROM reviews WHERE full_name = ?`, repo.FullName).Scan(&payload, &fingerprint)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RepositoryReview{}, false, nil
		}
		return RepositoryReview{}, false, err
	}
	if fingerprint != repositoryFingerprint(repo) {
		return RepositoryReview{}, false, nil
	}
	var review RepositoryReview
	if err := json.Unmarshal(payload, &review); err != nil {
		return RepositoryReview{}, false, fmt.Errorf("decode review from sqlite: %w", err)
	}
	review.Repository = repo
	return review, true, nil
}

func (s *SQLiteStore) ListReviews(repos []Repository) ([]RepositoryReview, error) {
	return s.listReviews("reviews", repos)
}

func (s *SQLiteStore) ListRuleReviews(repos []Repository) ([]RepositoryReview, error) {
	return s.listReviews("rule_reviews", repos)
}

func (s *SQLiteStore) listReviews(table string, repos []Repository) ([]RepositoryReview, error) {
	currentRepos := make(map[string]Repository, len(repos))
	for _, repo := range repos {
		currentRepos[cacheKey(repo.FullName)] = repo
	}
	rows, err := s.db.Query(`SELECT full_name, payload_json, fingerprint FROM ` + table + ` ORDER BY reviewed_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reviews := make([]RepositoryReview, 0)
	for rows.Next() {
		var fullName string
		var payload []byte
		var fingerprint string
		if err := rows.Scan(&fullName, &payload, &fingerprint); err != nil {
			return nil, err
		}
		repo, ok := currentRepos[cacheKey(fullName)]
		if !ok || fingerprint != repositoryFingerprint(repo) {
			continue
		}
		var review RepositoryReview
		if err := json.Unmarshal(payload, &review); err != nil {
			return nil, fmt.Errorf("decode reviews from sqlite: %w", err)
		}
		review.Repository = repo
		reviews = append(reviews, review)
	}
	return reviews, rows.Err()
}

func (s *SQLiteStore) SaveReview(review RepositoryReview) error {
	return s.saveReview("reviews", review)
}

func (s *SQLiteStore) SaveRuleReview(review RepositoryReview) error {
	return s.saveReview("rule_reviews", review)
}

func (s *SQLiteStore) saveReview(table string, review RepositoryReview) error {
	payload, err := json.Marshal(review)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO `+table+` (full_name, payload_json, fingerprint, reviewed_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(full_name) DO UPDATE SET
    payload_json = excluded.payload_json,
    fingerprint = excluded.fingerprint,
    reviewed_at = excluded.reviewed_at`,
		review.FullName, payload, repositoryFingerprint(review.Repository), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *SQLiteStore) DeleteRepository(fullName string) error {
	_, err := s.db.Exec("DELETE FROM repositories WHERE full_name = ? COLLATE NOCASE", fullName)
	return err
}
