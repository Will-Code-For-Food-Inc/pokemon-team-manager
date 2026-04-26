package web

import (
	"database/sql"

	"github.com/user/pokemon-team-manager/internal/handlers"
)

type chatRepo struct{ db *sql.DB }

type displayMessage = handlers.DisplayMessage

func (r *chatRepo) messages() ([]displayMessage, error) {
	return (&handlers.ChatRepo{DB: r.db}).Messages()
}

func (r *chatRepo) appendMessage(role, content string) (int64, error) {
	return (&handlers.ChatRepo{DB: r.db}).AppendMessage(role, content)
}

func (r *chatRepo) loadContext() ([]ollamaMessage, error) {
	return (&handlers.ChatRepo{DB: r.db}).LoadContext()
}

func (r *chatRepo) saveContext(msgs []ollamaMessage) error {
	return (&handlers.ChatRepo{DB: r.db}).SaveContext(msgs)
}

func (r *chatRepo) clear() error {
	return (&handlers.ChatRepo{DB: r.db}).Clear()
}

func lastN(msgs []displayMessage, n int) []displayMessage {
	return handlers.LastN(msgs, n)
}
