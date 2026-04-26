package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/user/pokemon-team-manager/internal/handlers"
)

func registerLogTools(s *server.MCPServer, svc *Services) {
	s.AddTool(mcp.NewTool("add_team_log",
		mcp.WithDescription("Append a combat/session log entry to a team. Use to record notable matchups, what worked, what didn't."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("entry", mcp.Required(), mcp.Description("Log entry text (markdown supported)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		id, err := handlers.AddTeamLog(svc, getInt(args, "team_id"), getString(args, "entry"))
		if err != nil {
			return textResult("Error: " + err.Error()), nil
		}
		return textResult(fmt.Sprintf("Log entry %d recorded.", id)), nil
	})

	s.AddTool(mcp.NewTool("get_team_logs",
		mcp.WithDescription("Retrieve all combat/session log entries for a team, newest first."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		logs, err := handlers.GetTeamLogs(svc, getInt(req.GetArguments(), "team_id"))
		if err != nil {
			return textResult("Error: " + err.Error()), nil
		}
		if len(logs) == 0 {
			return textResult("No log entries yet."), nil
		}
		b, _ := json.Marshal(logs)
		return textResult(string(b)), nil
	})
}
