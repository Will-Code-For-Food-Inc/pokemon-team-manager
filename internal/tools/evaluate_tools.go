package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/user/pokemon-team-manager/internal/handlers"
	"github.com/user/pokemon-team-manager/internal/team"
)

func registerEvaluateTools(s *server.MCPServer, svc *Services) {
	s.AddTool(mcp.NewTool("evaluate_pokemon",
		mcp.WithDescription("Evaluate a Pokemon: type chart, offensive coverage, stat role, bulk, speed tier. Pass team_id for move/EV analysis."),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name (e.g. 'Garchomp')")),
		mcp.WithNumber("team_id", mcp.Description("Team ID — enables member-level move/EV analysis")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		teamID, _ := args["team_id"].(float64)
		result, err := handlers.EvaluatePokemon(svc, getString(args, "pokemon_name"), int(teamID))
		if err != nil {
			return textResult("Error: " + err.Error()), nil
		}
		switch result.Kind {
		case handlers.EvalMember:
			return textResult(team.FormatMemberEval(result.MemberEval)), nil
		case handlers.EvalNotOnTeam:
			return textResult(fmt.Sprintf("%s is not on team %d; showing species-level evaluation.", result.SpeciesName, result.TeamID)), nil
		default:
			return textResult(team.FormatSpeciesEval(result.SpeciesEval)), nil
		}
	})
}
