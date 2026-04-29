package web

import (
	"github.com/user/pokemon-team-manager/internal/handlers"
)

// Re-export types used across the web package.
type ollamaMessage = handlers.OllamaMessage
type chatMessage = handlers.ChatMessage
type agentView = handlers.AgentView

func runAgent(ollama *handlers.OllamaClient, svc *Services, history []ollamaMessage, userMsg string, onMsg func(chatMessage)) ([]chatMessage, []ollamaMessage, *agentView) {
	return handlers.RunAgent(ollama, svc.Handlers(), history, userMsg, onMsg)
}

func newOllamaClient(cfg agentConfig) *handlers.OllamaClient {
	return handlers.NewOllamaClient(cfg)
}
