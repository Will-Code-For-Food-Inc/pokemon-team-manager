package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/user/pokemon-team-manager/internal/handlers"
)

// registerAgentTool adds chat_agent — an MCP-only tool that runs the Ollama
// agent loop. It is intentionally excluded from buildPtmTools so the agent
// cannot call itself.
func registerAgentTool(s *server.MCPServer, svc *Services) {
	s.AddTool(mcp.NewTool("chat_agent",
		mcp.WithDescription("Send a message to the ptm Ollama agent and return its full response including tool calls. Use this to delegate team-editing tasks to the agent instead of calling tools manually."),
		mcp.WithString("message", mcp.Required(), mcp.Description("The message to send to the agent")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if svc.DB == nil {
			return errResult("chat_agent requires DB — start ptm mcp with a database"), nil
		}

		cfg := handlers.LoadConfig(svc.DB)
		ollama := handlers.NewOllamaClient(cfg)
		repo := &handlers.ChatRepo{DB: svc.DB}

		msg := getString(req.GetArguments(), "message")

		queryVec, _ := ollama.Embed(msg)
		hist, _ := repo.SemanticHistory(queryVec, cfg.Lookback, cfg.Lookback)

		produced, newCtx, _ := handlers.RunAgent(ollama, svc, hist, msg, nil)

		if userID, err := repo.AppendMessage("user", msg); err == nil {
			go handlers.EmbedAndSave(ollama, repo, userID, msg)
		}
		for _, m := range produced {
			if id, err := repo.AppendMessage(m.Role, m.Content); err == nil && (m.Role == "assistant" || m.Role == "tool") {
				go handlers.EmbedAndSave(ollama, repo, id, m.Content)
			}
		}
		fullCtx, _ := repo.LoadContext()
		_ = repo.SaveContext(append(fullCtx, newCtx...))

		// Return a readable summary of what the agent did.
		var b strings.Builder
		for _, m := range produced {
			switch m.Role {
			case "assistant":
				fmt.Fprintf(&b, "%s\n", m.Content)
			case "tool":
				fmt.Fprintf(&b, "%s\n", m.Content)
			case "error":
				fmt.Fprintf(&b, "[error] %s\n", m.Content)
			}
		}
		if b.Len() == 0 {
			return textResult("Agent produced no output."), nil
		}
		return textResult(strings.TrimSpace(b.String())), nil
	})
}
