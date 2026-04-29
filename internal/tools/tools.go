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
	registerAgentTool(s, svc)
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

	toolHelp := map[string]string{
		"get_moves": `get_moves pokemon_name="..."
Returns the Champions-format learnset — the curated list of moves that species can use in-game.
This is the authoritative source for set_moves validation.
If a move you expect is missing, the learnset data may have a gap.
Fix: call add_learnset_move to register it, then set_moves will accept it.`,

		"add_learnset_move": `add_learnset_move pokemon_name="..." move_name="..."
Adds a move to a species' Champions learnset permanently.
Use this when the user confirms a move IS available in-game but our database is missing it.
After adding, set_moves will accept the move.
The addition persists for this session and future ones.`,

		"set_moves": `set_moves team_id=N pokemon_name="..." move1="..." [move2 move3 move4]
Sets up to 4 moves for a Pokemon. Each move is checked against the Champions learnset.
If a move fails: call get_moves to see what IS legal and pick an alternative.
The learnset data is fixed externally — never call add_learnset_move. If no legal alternative fits, return that fact to the caller.`,

		"set_stats": `set_stats team_id=N pokemon_name="..." hp=N attack=N defense=N sp_attack=N sp_defense=N speed=N
Sets stat points (SP) for a Pokemon. Champions rules: 66 total, max 32 per stat.
Omitted stats default to 0. All six stats are replaced atomically.
VP cost: 5 VP per stat point.`,

		"training_cost": `training_cost team_id=N
Calculates total VP cost to build the team from scratch.
VP breakdown per Pokemon: recruit 800 + stat points (×5 each) + nature (500 if not Serious) + moves (250 each) + hidden ability (500).
Item VP costs: 400 (resist berries), 700 (common), 1000 (special), 2000 (mega stone).`,

		"evaluate_pokemon": `evaluate_pokemon pokemon_name="..." [team_id=N]
Always call this before recommending any Pokemon.
Without team_id: species-level analysis — type chart, offensive coverage, stat role, bulk, speed tier.
With team_id: member-level analysis — adds move flags (priority/setup/recovery/redirection), EV efficiency check, move/stat mismatch detection.`,

		"find_pokemon_by_filters": `find_pokemon_by_filters [type="..."] [role="..."] [speed_tier="..."] [legendary=true|false] [final_evo_only=true] [owned=true|false] [limit=N]
Discover owned Pokemon by role and type — the primary search tool.
role options: "physical attacker", "special attacker", "mixed attacker", "support", "tank"
speed_tier options: "fast" (>100 base), "mid" (70-100), "slow" (<70)
Always call evaluate_pokemon on each result before recommending.`,

		"search_knowledge": `search_knowledge query="..."
Semantic search over ingested strategy docs, meta guides, and tier lists.
Use natural language queries. Call this BEFORE making team recommendations.
Example: "steel type support for trick room" or "best physical attackers regulation H"`,

		"add_team_log": `add_team_log team_id=N entry="..."
Appends a battle/session journal entry. Use for: notable matchups, what worked, what didn't, opponent teams seen.
These are separate from team strategy notes — logs are experiential and append-only.
Retrieve with get_team_logs team_id=N.`,
	}

	s.AddTool(mcp.NewTool("get_help",
		mcp.WithDescription("Returns usage docs for ptm tools. Call with no args for the full overview, or tool_name=... for detailed help on a specific tool."),
		mcp.WithString("tool_name", mcp.Description("Optional: name of a specific tool to get detailed help for (e.g. 'set_moves', 'get_moves', 'add_learnset_move')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if name := getString(req.GetArguments(), "tool_name"); name != "" {
			if detail, ok := toolHelp[strings.TrimSpace(strings.ToLower(name))]; ok {
				return textResult(detail), nil
			}
			// Unknown tool name — return overview with note
			return textResult(fmt.Sprintf("No detailed help for %q. Available: %s\n\n---\n%s",
				name, strings.Join(func() []string {
					keys := make([]string, 0, len(toolHelp))
					for k := range toolHelp {
						keys = append(keys, k)
					}
					return keys
				}(), ", "),
				helpText,
			)), nil
		}
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
					if m.Config == nil || m.Config.Species == nil {
						continue
					}
					c := m.Config
					item := "-"
					if c.Item != nil {
						item = c.Item.Name
					}
					ability := "-"
					if c.Ability != nil {
						ability = c.Ability.Name
					}
					var moveNames []string
					for _, mv := range c.Moves {
						if mv != nil {
							moveNames = append(moveNames, mv.Name)
						}
					}
					fmt.Fprintf(&b, "- Slot %d: %s | %s | %s | %s | EVs: HP%d Atk%d Def%d SpA%d SpD%d Spe%d | Moves: %s\n",
						m.Slot, c.Species.Name, ability, item, c.Nature,
						c.EVs.HP, c.EVs.Atk, c.EVs.Def, c.EVs.SpA, c.EVs.SpD, c.EVs.Spe,
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
