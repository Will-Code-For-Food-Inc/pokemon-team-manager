package web

import (
	"database/sql"

	"github.com/user/pokemon-team-manager/internal/handlers"
)

type chatRepo struct{ db *sql.DB }

type displayMessage = handlers.DisplayMessage

func (r *chatRepo) chatRepo() *handlers.ChatRepo { return &handlers.ChatRepo{DB: r.db} }

func (r *chatRepo) messages() ([]displayMessage, error) {
	return r.chatRepo().Messages()
}

func (r *chatRepo) appendMessage(role, content string) (int64, error) {
	return r.chatRepo().AppendMessage(role, content)
}

func (r *chatRepo) loadContext() ([]ollamaMessage, error) {
	return r.chatRepo().LoadContext()
}

func (r *chatRepo) saveContext(msgs []ollamaMessage) error {
	return r.chatRepo().SaveContext(msgs)
}


func (r *chatRepo) semanticHistory(queryVec []float32, k, fallbackN int) ([]ollamaMessage, error) {
	return r.chatRepo().SemanticHistory(queryVec, k, fallbackN)
}

func lastN(msgs []displayMessage, n int) []displayMessage {
	return handlers.LastN(msgs, n)
}
