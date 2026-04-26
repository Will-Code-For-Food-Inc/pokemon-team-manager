// Package tools registers all MCP tool handlers for the ptm MCP server.
package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/user/pokemon-team-manager/internal/handlers"
)

// Services holds all domain repositories needed by the tool handlers.
type Services = handlers.Services

// Register adds all ptm tools to the MCP server.
func Register(s *server.MCPServer, svc *Services) {
	registerHelpTool(s)
	registerPromptTool(s, svc)
	registerPokemonTools(s, svc)
	registerTeamTools(s, svc)
	registerLogTools(s, svc)
	registerEvaluateTools(s, svc)
	registerKnowledgeTools(s, svc)
	registerRegulationTools(s, svc)
	registerAnalysisTools(s, svc)
}

// registerHelpTool adds a get_help tool that returns compact usage docs.
// Call this first to orient a small model before issuing other tool calls.
func registerHelpTool(s *server.MCPServer) {
	const helpText = `ptm — Pokemon Team Manager (VGC format only)

QUICK START:
1. list_regulations → pick one (current: I2)
2. create_team name="..." regulation="I2" → get team_id
3. find_pokemon_by_filters type="fire" role="tank" speed_tier="slow" → discover owned candidates (no name guessing)
   find_pokemon_by_name name="Garcho" → fuzzy name lookup when you already know the name
4. add_pokemon team_id=N pokemon_name="..." → adds to next slot (max 6)
5. set_ability / set_nature / set_item / set_moves / set_stats / set_role
6. validate_team team_id=N → check violations
7. analyse_team team_id=N → coverage & speed tiers
8. export_team team_id=N → markdown output

OWNERSHIP: Use set_owned to mark a Pokemon or item as owned/unowned when the user tells you.
  set_owned type="pokemon" name="Tyranitar" owned=true
  set_owned type="item" name="Lum Berry" owned=true
The owned flag appears in find_pokemon_by_name, find_pokemon_by_filters, and get_item results.

RULES: 6 Pokemon, no duplicate species or items, final evos only,
Stat points: 66 total, max 32/stat (Pokemon Champions system). 0 restricted Legendaries (Reg I2).

STATS for set_stats: hp attack defense sp_attack sp_defense speed
Use search_knowledge for strategy docs and tier lists.

EVALUATE: Always call evaluate_pokemon before recommending any Pokemon as a candidate or replacement.
  evaluate_pokemon pokemon_name="..." [team_id=N] → type chart, offensive coverage, stat role, bulk, speed tier; add team_id for move/EV analysis

COMBAT LOGS: Record session experiences separate from team notes.
  add_team_log team_id=N entry="..." → append a log entry (notable matchups, what worked/didn't)
  get_team_logs team_id=N           → retrieve all entries newest-first`

	s.AddTool(mcp.NewTool("get_help",
		mcp.WithDescription("Returns a compact usage guide for all ptm tools. Call this first if you are unsure what tools are available or how to build a team."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return textResult(helpText), nil
	})
}

// registerPromptTool adds get_prompt, which generates a concise session-start
// prompt for the LLM that can optionally include team rosters in context.
func registerPromptTool(s *server.MCPServer, svc *Services) {
	s.AddTool(mcp.NewTool("get_prompt",
		mcp.WithDescription("Generate a compact session prompt for the LLM. Optionally loads one or more team rosters into the prompt so the model has them in context."),
		mcp.WithString("goal", mcp.Description("What you want the model to do, e.g. 'build a trick room team for Regulation H'")),
		mcp.WithString("team_ids", mcp.Description("Comma-separated team IDs to preload into context, e.g. '1,2'")),
		mcp.WithString("include", mcp.Description("Extra sections to include: 'rules', 'natures', 'archetypes' (comma-separated)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goal := getString(args, "goal")
		teamIDs := getString(args, "team_ids")
		include := getString(args, "include")

		var b strings.Builder
		b.WriteString("You are a competitive Pokemon VGC team builder assistant.\n")
		b.WriteString("Use the available MCP tools to manage teams. Call get_help if unsure.\n\n")
		b.WriteString("VGC rules: 6 Pokemon, bring 4, double battles, Lv50, no duplicate species or items, final evos only.\n")

		if goal != "" {
			fmt.Fprintf(&b, "\nCurrent goal: %s\n", goal)
		}

		if teamIDs != "" {
			b.WriteString("\n## Preloaded Teams\n")
			for idStr := range strings.SplitSeq(teamIDs, ",") {
				idStr = strings.TrimSpace(idStr)
				var id int
				fmt.Sscan(idStr, &id)
				if id == 0 {
					continue
				}
				t, err := svc.Team.GetTeam(id)
				if err != nil {
					fmt.Fprintf(&b, "\nTeam %d: not found.\n", id)
					continue
				}
				fmt.Fprintf(&b, "\n### Team %d: %s (Regulation %s)\n", t.ID, t.Name, t.Regulation)
				for _, m := range t.Members {
					if m.Species == nil {
						continue
					}
					item := "-"
					if m.Item != nil {
						item = m.Item.Name
					}
					ability := "-"
					if m.Ability != nil {
						ability = m.Ability.Name
					}
					var moveNames []string
					for _, mv := range m.Moves {
						if mv != nil {
							moveNames = append(moveNames, mv.Name)
						}
					}
					fmt.Fprintf(&b, "- Slot %d: %s | %s | %s | %s | EVs: HP%d Atk%d Def%d SpA%d SpD%d Spe%d | Moves: %s\n",
						m.Slot, m.Species.Name, ability, item, m.Nature,
						m.EVs.HP, m.EVs.Atk, m.EVs.Def, m.EVs.SpA, m.EVs.SpD, m.EVs.Spe,
						strings.Join(moveNames, ", "))
				}
			}
		}

		for sec := range strings.SplitSeq(include, ",") {
			switch strings.TrimSpace(strings.ToLower(sec)) {
			case "rules":
				b.WriteString("\n## VGC Rules Detail\nStat points: 66 total, max 32/stat (Pokemon Champions). Reg H: 0 restricted Legendaries.\nBanned moves: Swagger.\n")
			case "natures":
				b.WriteString("\n## Key Natures\nTimid (+Spe/-SpA) Jolly (+Spe/-Atk) Modest (+SpA/-Atk) Adamant (+Atk/-SpA)\nBold (+Def/-Atk) Calm (+SpD/-Atk) Quiet (+SpA/-Spe) Brave (+Atk/-Spe)\n")
			case "archetypes":
				b.WriteString("\n## Common Archetypes\nTrick Room: slow Pokemon, TR setters (Indeedee-F, Porygon2, Dusclops)\nTailwind: fast Pokemon, support (Murkrow, Tornadus, Flutter Mane)\nWeather: Sun (Groudon+Zard), Rain (Kyogre+Swift Swim), Sand (Excadrill), Snow (Chien-Pao)\nHyper Offense: 6 fast attackers, no setup\n")
			}
		}

		b.WriteString("\nUse validate_team to check legality before finalising. Use analyse_team for coverage gaps.\n")
		return textResult(b.String()), nil
	})
}
