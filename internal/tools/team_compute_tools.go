package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/user/pokemon-team-manager/internal/handlers"
	"github.com/user/pokemon-team-manager/internal/team"
)

// statSpreadFromArgs extracts a team.StatSpread from MCP args.
func statSpreadFromArgs(args map[string]any) team.StatSpread {
	return team.StatSpread{
		HP:  getInt(args, "hp"),
		Atk: getInt(args, "attack"),
		Def: getInt(args, "defense"),
		SpA: getInt(args, "sp_attack"),
		SpD: getInt(args, "sp_defense"),
		Spe: getInt(args, "speed"),
	}
}

func registerAnalysisTools(s *server.MCPServer, svc *Services) {
	s.AddTool(mcp.NewTool("validate_team",
		mcp.WithDescription("Check a team against all VGC rules. Returns a list of violations, or confirms the team is legal."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		violations, err := handlers.ValidateTeam(svc, getInt(req.GetArguments(), "team_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if len(violations) == 0 {
			return textResult("Team is legal — no violations found."), nil
		}
		return jsonResult(violations), nil
	})

	s.AddTool(mcp.NewTool("analyse_team",
		mcp.WithDescription("Analyse a team's type coverage, defensive weaknesses, speed tiers, and detected archetypes."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_, analysis, err := handlers.AnalyseTeam(svc, getInt(req.GetArguments(), "team_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		return jsonResult(analysis), nil
	})

	s.AddTool(mcp.NewTool("export_team",
		mcp.WithDescription("Export a team as a formatted markdown document."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		t, err := svc.Team.GetTeam(getInt(req.GetArguments(), "team_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(team.ExportMarkdown(t)), nil
	})

	s.AddTool(mcp.NewTool("calc_stats",
		mcp.WithDescription("Calculate final Lv50 stats for a species given stat points and nature."),
		mcp.WithString("species", mcp.Required(), mcp.Description("Species name")),
		mcp.WithNumber("hp", mcp.Description("HP stat points (0-32, default 0)")),
		mcp.WithNumber("attack", mcp.Description("Attack stat points (0-32, default 0)")),
		mcp.WithNumber("defense", mcp.Description("Defense stat points (0-32, default 0)")),
		mcp.WithNumber("sp_attack", mcp.Description("Sp. Atk stat points (0-32, default 0)")),
		mcp.WithNumber("sp_defense", mcp.Description("Sp. Def stat points (0-32, default 0)")),
		mcp.WithNumber("speed", mcp.Description("Speed stat points (0-32, default 0)")),
		mcp.WithString("nature", mcp.Description("Nature name to apply multipliers (optional)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		result, err := handlers.CalcStats(svc, getString(args, "species"), statSpreadFromArgs(args), getString(args, "nature"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%-9s %6s %4s %7s\n", "Stat", "Base", "SP", "Final")
		fmt.Fprintf(&b, "%-9s %6s %4s %7s\n", "---------", "------", "----", "-------")
		for _, row := range result.Rows {
			fmt.Fprintf(&b, "%-9s %6d %4d %7d\n", row.Name, row.Base, row.SP, row.Final)
		}
		if result.Nature != "" {
			fmt.Fprintf(&b, "\nNature: %s", result.Nature)
			if result.Boosted != "" {
				fmt.Fprintf(&b, " (+%s / -%s)", result.Boosted, result.Reduced)
			}
			b.WriteString("\n")
		}
		return textResult(b.String()), nil
	})

	s.AddTool(mcp.NewTool("training_cost",
		mcp.WithDescription("Calculate VP cost to build a team from scratch (stat points + nature + hidden ability)."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		t, err := svc.Team.GetTeam(getInt(req.GetArguments(), "team_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if len(t.Members) == 0 {
			return textResult("Team has no members."), nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "VP Cost Breakdown — Team %d: %s\n\n", t.ID, t.Name)
		fmt.Fprintf(&b, "%-16s %6s %7s %7s %7s %8s\n", "Pokemon", "SP", "Nature", "Moves", "Ability", "Total")
		fmt.Fprintf(&b, "%-16s %6s %7s %7s %7s %8s\n", "----------------", "------", "-------", "-------", "-------", "--------")

		teamTotal := 0
		for _, m := range t.Members {
			if m.Config == nil || m.Config.Species == nil {
				continue
			}
			c := m.Config
			spTotal := c.EVs.HP + c.EVs.Atk + c.EVs.Def + c.EVs.SpA + c.EVs.SpD + c.EVs.Spe
			spVP := spTotal * 5

			natureVP := 0
			if c.Nature != "" && !strings.EqualFold(c.Nature, "Serious") {
				natureVP = 500
			}

			moveVP := len(c.Moves) * 250

			abilityVP := 0
			if c.Ability != nil {
				if abilities, err := svc.Pokemon.GetAbilitiesForSpecies(c.Species.Slug); err == nil && len(abilities) > 0 {
					if abilities[0].Slug != c.Ability.Slug {
						abilityVP = 500
					}
				}
			}

			memberTotal := 800 + spVP + natureVP + moveVP + abilityVP
			teamTotal += memberTotal
			fmt.Fprintf(&b, "%-16s %6d %7d %7d %7d %8d\n", c.Species.Name, spVP, natureVP, moveVP, abilityVP, memberTotal)
		}

		fmt.Fprintf(&b, "%-16s %6s %7s %7s %7s %8s\n", "----------------", "------", "-------", "-------", "-------", "--------")
		fmt.Fprintf(&b, "%-16s %44d VP\n", "TEAM TOTAL", teamTotal)
		return textResult(b.String()), nil
	})
}
