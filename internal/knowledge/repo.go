// Package knowledge manages the local knowledge base used for strategy doc
// search. Documents are indexed with SQLite FTS5 for full-text search.
package knowledge

import (
	"database/sql"
	"fmt"
	"strings"
)

// Document is a knowledge base entry.
type Document struct {
	ID         int    `json:"id"`
	Title      string `json:"title"`
	Source     string `json:"source"`
	Content    string `json:"content"`
	ChunkIndex int    `json:"chunk_index"`
}

// SearchResult is a document returned from a search query.
type SearchResult struct {
	Document
	Rank float64 `json:"rank"`
}

// Repo provides access to the knowledge base.
type Repo struct {
	db *sql.DB
}

// NewRepo creates a new knowledge Repo.
func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

// Ingest adds a document to the knowledge base, splitting it into chunks of
// roughly chunkSize runes each. Pass chunkSize <= 0 to store as a single chunk.
func (r *Repo) Ingest(title, source, content string, chunkSize int) error {
	chunks := chunkText(content, chunkSize)
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, chunk := range chunks {
		if _, err := tx.Exec(
			`INSERT INTO kb_documents(title, source, content, chunk_index) VALUES(?,?,?,?)`,
			title, source, chunk, i,
		); err != nil {
			return fmt.Errorf("Ingest: %w", err)
		}
	}
	return tx.Commit()
}

// Search performs an FTS5 full-text search and returns up to limit results.
func (r *Repo) Search(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	// Sanitise query for FTS5 (escape double-quotes).
	ftsQuery := strings.ReplaceAll(query, `"`, `""`)

	rows, err := r.db.Query(`
		SELECT d.id, d.title, d.source, d.content, d.chunk_index, kb_fts.rank
		FROM kb_fts
		JOIN kb_documents d ON d.id = kb_fts.rowid
		WHERE kb_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, ftsQuery, limit)
	if err != nil {
		// Fall back to LIKE search if FTS query is malformed.
		return r.likeSearch(query, limit)
	}
	defer rows.Close()

	var out []SearchResult
	for rows.Next() {
		var sr SearchResult
		if err := rows.Scan(&sr.ID, &sr.Title, &sr.Source, &sr.Content, &sr.ChunkIndex, &sr.Rank); err != nil {
			return nil, err
		}
		out = append(out, sr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return r.likeSearch(query, limit)
	}
	return out, nil
}

// likeSearch is a fallback substring search used when FTS fails or returns nothing.
func (r *Repo) likeSearch(query string, limit int) ([]SearchResult, error) {
	pat := "%" + query + "%"
	rows, err := r.db.Query(`
		SELECT id, title, source, content, chunk_index
		FROM kb_documents
		WHERE lower(content) LIKE lower(?) OR lower(title) LIKE lower(?)
		LIMIT ?`, pat, pat, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchResult
	for rows.Next() {
		var sr SearchResult
		if err := rows.Scan(&sr.ID, &sr.Title, &sr.Source, &sr.Content, &sr.ChunkIndex); err != nil {
			return nil, err
		}
		out = append(out, sr)
	}
	return out, rows.Err()
}

// chunkText splits text into chunks of at most size runes. If size <= 0, returns
// the whole text as one chunk.
func chunkText(text string, size int) []string {
	if size <= 0 || len([]rune(text)) <= size {
		return []string{text}
	}
	runes := []rune(text)
	var chunks []string
	for len(runes) > 0 {
		n := size
		if n > len(runes) {
			n = len(runes)
		}
		// Try to break on a newline within the last 20% of the chunk.
		breakAt := n
		for i := n - 1; i >= n*4/5; i-- {
			if runes[i] == '\n' {
				breakAt = i + 1
				break
			}
		}
		chunks = append(chunks, string(runes[:breakAt]))
		runes = runes[breakAt:]
	}
	return chunks
}
