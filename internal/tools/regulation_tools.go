package tools

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerRegulationTools(s *server.MCPServer, svc *Services) {
	s.AddTool(mcp.NewTool("list_regulations",
		mcp.WithDescription("List active regulation sets. Pass all=true to include inactive ones."),
		mcp.WithBoolean("all", mcp.Description("Include inactive regulations (default false)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var regs any
		var err error
		if getBool(req.GetArguments(), "all") {
			regs, err = svc.Team.ListRegulations()
		} else {
			regs, err = svc.Team.ListActiveRegulations()
		}
		if err != nil {
			return errResult(err.Error()), nil
		}
		return jsonResult(regs), nil
	})

	s.AddTool(mcp.NewTool("get_regulation",
		mcp.WithDescription("Get details for a regulation set including banned and restricted Pokemon."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Regulation ID (e.g. 'I2', 'M-A')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := strings.ToUpper(getString(req.GetArguments(), "id"))
		reg, err := svc.Team.GetRegulation(id)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return jsonResult(reg), nil
	})
}
