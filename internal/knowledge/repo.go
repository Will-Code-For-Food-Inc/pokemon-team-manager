// Package knowledge manages the local knowledge base used for strategy doc
// search. Documents are indexed with SQLite FTS5 for full-text search and
// optionally with Ollama-generated embeddings for semantic vector search.
package knowledge

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// Document is a knowledge base entry.
type Document struct {
	ID          int     `json:"id"`
	Title       string  `json:"title"`
	Source      string  `json:"source"`
	Content     string  `json:"content"`
	ChunkIndex  int     `json:"chunk_index"`
	ExpiresAt   string  `json:"expires_at"`
	ReviewedAt  *string `json:"reviewed_at,omitempty"`
	Stale       bool    `json:"stale,omitempty"`
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
// TTL is set to 6 months from ingestion.
func (r *Repo) Ingest(title, source, content string, chunkSize int) error {
	chunks := chunkText(content, chunkSize)
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, chunk := range chunks {
		if _, err := tx.Exec(
			`INSERT INTO kb_documents(title, source, content, chunk_index, expires_at)
			 VALUES(?,?,?,?,datetime('now','+6 months'))`,
			title, source, chunk, i,
		); err != nil {
			return fmt.Errorf("Ingest: %w", err)
		}
	}
	return tx.Commit()
}

// Touch resets the TTL on all chunks sharing the same title, marking them reviewed now.
func (r *Repo) Touch(title string) (int64, error) {
	res, err := r.db.Exec(
		`UPDATE kb_documents
		 SET expires_at = datetime('now','+6 months'), reviewed_at = datetime('now')
		 WHERE title = ?`, title)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// StaleDocuments returns one row per distinct title that has expired.
func (r *Repo) StaleDocuments() ([]Document, error) {
	rows, err := r.db.Query(`
		SELECT id, title, source, content, chunk_index, expires_at, reviewed_at
		FROM kb_documents
		WHERE expires_at < datetime('now')
		GROUP BY title
		ORDER BY expires_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocs(rows, true)
}

// AllDocuments returns all documents.
func (r *Repo) AllDocuments() ([]Document, error) {
	rows, err := r.db.Query(`SELECT id, title, source, content, chunk_index, expires_at, reviewed_at FROM kb_documents`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocs(rows, false)
}

// UnembeddedDocuments returns documents that don't yet have embeddings.
func (r *Repo) UnembeddedDocuments() ([]Document, error) {
	rows, err := r.db.Query(`
		SELECT d.id, d.title, d.source, d.content, d.chunk_index, d.expires_at, d.reviewed_at
		FROM kb_documents d
		LEFT JOIN kb_embeddings e ON e.document_id = d.id
		WHERE e.document_id IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocs(rows, false)
}

func scanDocs(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}, markStale bool) ([]Document, error) {
	var out []Document
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.Title, &d.Source, &d.Content, &d.ChunkIndex, &d.ExpiresAt, &d.ReviewedAt); err != nil {
			return nil, err
		}
		d.Stale = markStale
		out = append(out, d)
	}
	return out, rows.Err()
}

// SaveEmbedding stores a float32 embedding for a document.
func (r *Repo) SaveEmbedding(docID int, vec []float32) error {
	blob := float32sToBlob(vec)
	_, err := r.db.Exec(
		`INSERT INTO kb_embeddings(document_id, embedding) VALUES(?,?)
		 ON CONFLICT(document_id) DO UPDATE SET embedding=excluded.embedding`,
		docID, blob)
	return err
}

// EmbeddingCount returns the number of stored embeddings.
func (r *Repo) EmbeddingCount() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT count(*) FROM kb_embeddings`).Scan(&n)
	return n, err
}

// VectorSearch performs cosine similarity search against stored embeddings.
func (r *Repo) VectorSearch(query []float32, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.Query(`
		SELECT d.id, d.title, d.source, d.content, d.chunk_index, d.expires_at, d.reviewed_at, e.embedding
		FROM kb_embeddings e
		JOIN kb_documents d ON d.id = e.document_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		doc   SearchResult
		score float64
	}
	var candidates []scored
	for rows.Next() {
		var d SearchResult
		var blob []byte
		if err := rows.Scan(&d.ID, &d.Title, &d.Source, &d.Content, &d.ChunkIndex, &d.ExpiresAt, &d.ReviewedAt, &blob); err != nil {
			return nil, err
		}
		vec := blobToFloat32s(blob)
		d.Rank = cosineSimilarity(query, vec)
		candidates = append(candidates, scored{doc: d, score: d.Rank})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by descending similarity.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].score > candidates[j-1].score; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}
	if limit < len(candidates) {
		candidates = candidates[:limit]
	}
	out := make([]SearchResult, len(candidates))
	for i, c := range candidates {
		out[i] = c.doc
	}
	return out, nil
}

// Search performs semantic vector search if embeddings are available, falling
// back to FTS5 and then LIKE search.
func (r *Repo) Search(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	return r.ftsSearch(query, limit)
}

// SearchWithVector uses a pre-computed query embedding for semantic search,
// falling back to FTS if no embeddings are stored.
func (r *Repo) SearchWithVector(queryVec []float32, ftsQuery string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	n, _ := r.EmbeddingCount()
	if n > 0 && len(queryVec) > 0 {
		results, err := r.VectorSearch(queryVec, limit)
		if err == nil && len(results) > 0 {
			return results, nil
		}
	}
	return r.ftsSearch(ftsQuery, limit)
}

func (r *Repo) ftsSearch(query string, limit int) ([]SearchResult, error) {
	ftsQuery := strings.ReplaceAll(query, `"`, `""`)
	rows, err := r.db.Query(`
		SELECT d.id, d.title, d.source, d.content, d.chunk_index, d.expires_at, d.reviewed_at, kb_fts.rank
		FROM kb_fts
		JOIN kb_documents d ON d.id = kb_fts.rowid
		WHERE kb_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, ftsQuery, limit)
	if err != nil {
		return r.likeSearch(query, limit)
	}
	defer rows.Close()
	var out []SearchResult
	for rows.Next() {
		var sr SearchResult
		if err := rows.Scan(&sr.ID, &sr.Title, &sr.Source, &sr.Content, &sr.ChunkIndex, &sr.ExpiresAt, &sr.ReviewedAt, &sr.Rank); err != nil {
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
		SELECT id, title, source, content, chunk_index, expires_at, reviewed_at
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
		if err := rows.Scan(&sr.ID, &sr.Title, &sr.Source, &sr.Content, &sr.ChunkIndex, &sr.ExpiresAt, &sr.ReviewedAt); err != nil {
			return nil, err
		}
		out = append(out, sr)
	}
	return out, rows.Err()
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func float32sToBlob(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

func blobToFloat32s(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
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
