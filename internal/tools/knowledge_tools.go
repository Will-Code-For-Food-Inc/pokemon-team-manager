package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/user/pokemon-team-manager/internal/handlers"
)

func registerKnowledgeTools(s *server.MCPServer, svc *Services) {
	s.AddTool(mcp.NewTool("search_knowledge",
		mcp.WithDescription("Search the strategy knowledge base (VGC rules, tier lists, guides) using full-text search."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search terms (e.g. 'trick room setters', 'Regulation H restricted')")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 5)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		results, err := handlers.SearchKnowledge(svc, getString(args, "query"), getInt(args, "limit"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if len(results) == 0 {
			return textResult("No knowledge base results found for that query."), nil
		}
		return jsonResult(results), nil
	})

	s.AddTool(mcp.NewTool("ingest_document",
		mcp.WithDescription("Add a document to the knowledge base for future searching."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Document title")),
		mcp.WithString("source", mcp.Description("Source URL or file path")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Full document content (markdown supported)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		if err := svc.Knowledge.Ingest(
			getString(args, "title"),
			getString(args, "source"),
			getString(args, "content"),
			1500,
		); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult("Document ingested successfully."), nil
	})
}
