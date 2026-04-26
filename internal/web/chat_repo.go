package web

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

type chatRepo struct {
	db *sql.DB
}

type displayMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (r *chatRepo) messages() ([]displayMessage, error) {
	rows, err := r.db.Query(`SELECT role, content FROM chat_messages ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("chat_messages: %w", err)
	}
	defer rows.Close()
	var out []displayMessage
	for rows.Next() {
		var m displayMessage
		if err := rows.Scan(&m.Role, &m.Content); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// lastN returns the last n non-tool messages plus any tool messages interspersed
// among them (i.e. everything from the nth-from-last non-tool message onward).
func lastN(msgs []displayMessage, n int) []displayMessage {
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

func (r *chatRepo) appendMessage(role, content string) (int64, error) {
	res, err := r.db.Exec(`INSERT INTO chat_messages(role, content) VALUES(?, ?)`, role, content)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}


func (r *chatRepo) loadContext() ([]ollamaMessage, error) {
	var raw string
	err := r.db.QueryRow(`SELECT messages FROM chat_context WHERE id = 1`).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("chat_context load: %w", err)
	}
	var msgs []ollamaMessage
	if err := json.Unmarshal([]byte(raw), &msgs); err != nil {
		return nil, fmt.Errorf("chat_context decode: %w", err)
	}
	return msgs, nil
}

func (r *chatRepo) saveContext(msgs []ollamaMessage) error {
	raw, err := json.Marshal(msgs)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(
		`INSERT INTO chat_context(id, messages) VALUES(1, ?)
		 ON CONFLICT(id) DO UPDATE SET messages = excluded.messages`,
		string(raw),
	)
	return err
}

func (r *chatRepo) clear() error {
	_, err := r.db.Exec(`DELETE FROM chat_messages`)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`DELETE FROM chat_context`)
	return err
}
