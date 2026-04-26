package tools

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerRegulationTools(s *server.MCPServer, svc *Services) {
	s.AddTool(mcp.NewTool("list_regulations",
		mcp.WithDescription("List all VGC regulation sets (A through H and beyond)."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		regs, err := svc.Team.ListRegulations()
		if err != nil {
			return errResult(err.Error()), nil
		}
		return jsonResult(regs), nil
	})

	s.AddTool(mcp.NewTool("get_regulation",
		mcp.WithDescription("Get details for a VGC regulation set including banned and restricted Pokemon and banned moves."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Regulation ID (e.g. 'H')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		regs, err := svc.Team.ListRegulations()
		if err != nil {
			return errResult(err.Error()), nil
		}
		id := strings.ToUpper(getString(req.GetArguments(), "id"))
		for _, r := range regs {
			if r.ID == id {
				return jsonResult(r), nil
			}
		}
		return errResult("regulation not found: " + id), nil
	})
}
