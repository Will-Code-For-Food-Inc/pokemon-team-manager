package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
