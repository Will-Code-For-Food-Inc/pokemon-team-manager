package handlers

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// DisplayMessage is a persisted chat display entry.
type DisplayMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRepo persists display messages and Ollama context to SQLite.
type ChatRepo struct {
	DB *sql.DB
}

// Messages returns all display messages ordered oldest-first.
func (r *ChatRepo) Messages() ([]DisplayMessage, error) {
	rows, err := r.DB.Query(`SELECT role, content FROM chat_messages ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("chat_messages: %w", err)
	}
	defer rows.Close()
	var out []DisplayMessage
	for rows.Next() {
		var m DisplayMessage
		if err := rows.Scan(&m.Role, &m.Content); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AppendMessage inserts a single display message and returns its ID.
func (r *ChatRepo) AppendMessage(role, content string) (int64, error) {
	res, err := r.DB.Exec(`INSERT INTO chat_messages(role, content) VALUES(?, ?)`, role, content)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SaveEmbedding stores an embedding vector for a chat message.
func (r *ChatRepo) SaveEmbedding(id int64, vec []float32) error {
	blob := chatFloat32sToBlob(vec)
	_, err := r.DB.Exec(`UPDATE chat_messages SET embedding = ? WHERE id = ?`, blob, id)
	return err
}

// HistoryMessage is a chat message with its embedding for semantic retrieval.
type HistoryMessage struct {
	ID        int64
	Role      string
	Content   string
	Embedding []float32
}

// SemanticHistory retrieves the top-k most relevant chat messages by cosine
// similarity to the query vector. Only messages with embeddings are considered.
// Falls back to returning the last fallbackN messages if no embeddings exist.
func (r *ChatRepo) SemanticHistory(query []float32, k, fallbackN int) ([]OllamaMessage, error) {
	rows, err := r.DB.Query(
		`SELECT id, role, content, embedding FROM chat_messages
		 WHERE embedding IS NOT NULL AND role IN ('user','assistant','tool')
		 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		id    int64
		msg   OllamaMessage
		score float64
	}
	var candidates []scored
	for rows.Next() {
		var id int64
		var role, content string
		var blob []byte
		if err := rows.Scan(&id, &role, &content, &blob); err != nil {
			return nil, err
		}
		vec := chatBlobToFloat32s(blob)
		score := chatCosineSimilarity(query, vec)
		candidates = append(candidates, scored{
			id:    id,
			msg:   OllamaMessage{Role: role, Content: content},
			score: score,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fall back to plain recency if nothing is embedded yet.
	if len(candidates) == 0 {
		return r.recentMessages(fallbackN)
	}

	// Pick top-k by relevance, then re-sort chronologically so the agent sees
	// freshest last. Without this re-sort the model anchors on highly-similar
	// but stale turns instead of the most recent context.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	if k < len(candidates) {
		candidates = candidates[:k]
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].id < candidates[j].id
	})
	out := make([]OllamaMessage, len(candidates))
	for i, c := range candidates {
		out[i] = c.msg
	}
	return out, nil
}

func (r *ChatRepo) recentMessages(n int) ([]OllamaMessage, error) {
	rows, err := r.DB.Query(
		`SELECT role, content FROM chat_messages
		 WHERE role IN ('user','assistant','tool')
		 ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OllamaMessage
	for rows.Next() {
		var m OllamaMessage
		if err := rows.Scan(&m.Role, &m.Content); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Reverse to chronological order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func chatFloat32sToBlob(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

func chatBlobToFloat32s(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

func chatCosineSimilarity(a, b []float32) float64 {
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

// LoadContext retrieves the persisted Ollama message history.
func (r *ChatRepo) LoadContext() ([]OllamaMessage, error) {
	var raw string
	err := r.DB.QueryRow(`SELECT messages FROM chat_context WHERE id = 1`).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("chat_context load: %w", err)
	}
	var msgs []OllamaMessage
	if err := json.Unmarshal([]byte(raw), &msgs); err != nil {
		return nil, fmt.Errorf("chat_context decode: %w", err)
	}
	return msgs, nil
}

// SaveContext persists the Ollama message history.
func (r *ChatRepo) SaveContext(msgs []OllamaMessage) error {
	raw, err := json.Marshal(msgs)
	if err != nil {
		return err
	}
	_, err = r.DB.Exec(
		`INSERT INTO chat_context(id, messages) VALUES(1, ?)
		 ON CONFLICT(id) DO UPDATE SET messages = excluded.messages`,
		string(raw),
	)
	return err
}

// Clear deletes all display messages and the persisted context.
func (r *ChatRepo) Clear() error {
	if _, err := r.DB.Exec(`DELETE FROM chat_messages`); err != nil {
		return err
	}
	_, err := r.DB.Exec(`DELETE FROM chat_context`)
	return err
}

// ArchiveAndClear moves every live chat_messages and chat_context row into
// the archive tables under a single archive_id, then truncates the live
// tables. Returns the archive_id and the number of messages archived. Runs
// in one transaction so a failure leaves the live state untouched.
//
// We archive rather than delete because chat history is high-signal training
// data — agent traces, tool calls, model recoveries — we want to mine later.
func (r *ChatRepo) ArchiveAndClear(archiveID int64) (int64, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return 0, fmt.Errorf("chat archive: begin: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		INSERT INTO chat_messages_archive
			(archive_id, id, role, content, in_context, embedding, created_at)
		SELECT ?, id, role, content, in_context, embedding, created_at
		FROM chat_messages`, archiveID)
	if err != nil {
		return 0, fmt.Errorf("chat archive: copy messages: %w", err)
	}
	moved, _ := res.RowsAffected()

	if _, err := tx.Exec(`
		INSERT INTO chat_context_archive (archive_id, messages)
		SELECT ?, messages FROM chat_context WHERE id = 1`, archiveID); err != nil {
		return 0, fmt.Errorf("chat archive: copy context: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM chat_messages`); err != nil {
		return 0, fmt.Errorf("chat archive: clear messages: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM chat_context`); err != nil {
		return 0, fmt.Errorf("chat archive: clear context: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("chat archive: commit: %w", err)
	}
	return moved, nil
}

// LiveStats returns the row count of the live chat_messages table for
// display on the settings page.
func (r *ChatRepo) LiveStats() (int, error) {
	var n int
	err := r.DB.QueryRow(`SELECT COUNT(*) FROM chat_messages`).Scan(&n)
	return n, err
}

// LastN returns the last n non-tool messages plus any tool messages among them.
func LastN(msgs []DisplayMessage, n int) []DisplayMessage {
	count := 0
	cutoff := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "tool" {
			count++
			if count == n {
				cutoff = i
				break
			}
		}
	}
	return msgs[cutoff:]
}
