// Package tools registers all MCP tool handlers for the ptm MCP server.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/user/pokemon-team-manager/internal/knowledge"
	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

// Services holds all domain repositories needed by the tool handlers.
type Services struct {
	Pokemon   *pokemon.Repo
	Team      *team.Repo
	Knowledge *knowledge.Repo
}

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
   search_pokemon → last-resort broad search
4. add_pokemon team_id=N pokemon_name="..." → adds to next slot (max 6)
5. set_ability / set_nature / set_item / set_moves / set_stats / set_role
6. validate_team team_id=N → check violations
7. analyse_team team_id=N → coverage & speed tiers
8. export_team team_id=N → markdown output

OWNERSHIP: Use set_owned to mark a Pokemon or item as owned/unowned when the user tells you.
  set_owned type="pokemon" name="Tyranitar" owned=true
  set_owned type="item" name="Lum Berry" owned=true
The owned flag appears in search_pokemon and get_item results.

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
// prompt for the LLM that can optionally include team rosters and items in memory.
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

		// Optionally embed team rosters.
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

		// Optional extra sections.
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

// --- helpers ---

func textResult(text string) *mcp.CallToolResult {
	return mcp.NewToolResultText(text)
}

func jsonResult(v any) *mcp.CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultText("error marshalling result: " + err.Error())
	}
	return mcp.NewToolResultText(string(b))
}

func errResult(msg string) *mcp.CallToolResult {
	return mcp.NewToolResultText("Error: " + msg)
}

func errf(msg string, args ...any) *mcp.CallToolResult {
	return errResult(fmt.Sprintf(msg, args...))
}

func getString(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	v, _ := args[key].(string)
	return strings.TrimSpace(v)
}

func getBool(args map[string]any, key string) bool {
	if args == nil {
		return false
	}
	v, _ := args[key].(bool)
	return v
}

func getInt(args map[string]any, key string) int {
	if args == nil {
		return 0
	}
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

// --- Pokemon tools ---

func registerPokemonTools(s *server.MCPServer, svc *Services) {
	s.AddTool(mcp.NewTool("find_pokemon_by_name",
		mcp.WithDescription("Look up owned Pokemon by name. Use when you already know the name. For discovery use find_pokemon_by_filters."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Species name or partial name (fuzzy match)")),
		mcp.WithBoolean("owned", mcp.Description("Default true (owned only); false to search all")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 10)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		defaultOwned := true
		f := pokemon.SpeciesFilter{}
		if v, ok := args["owned"]; ok && v != nil {
			b := getBool(args, "owned")
			f.Owned = &b
		} else {
			f.Owned = &defaultOwned
		}
		limit := getInt(args, "limit")
		if limit <= 0 {
			limit = 10
		}
		results, err := svc.Pokemon.SearchSpecies(getString(args, "name"), limit, f)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if len(results) == 0 {
			return textResult("No owned Pokemon found with that name. If you're looking for candidates to fill a role, use find_pokemon_by_filters instead."), nil
		}
		return jsonResult(results), nil
	})

	s.AddTool(mcp.NewTool("find_pokemon_by_filters",
		mcp.WithDescription("Discover owned Pokemon candidates by type, role, and speed tier. Use this for coverage gaps and team building — never guess names."),
		mcp.WithString("type", mcp.Description("Filter by type (e.g. 'fire', 'steel')")),
		mcp.WithString("role", mcp.Description("'physical attacker'|'special attacker'|'mixed attacker'|'support'|'tank'")),
		mcp.WithString("speed_tier", mcp.Description("'fast' (>100 base)|'mid' (70-100)|'slow' (<70)")),
		mcp.WithBoolean("legendary", mcp.Description("Filter by legendary status")),
		mcp.WithBoolean("final_evo_only", mcp.Description("Only final evolutions")),
		mcp.WithBoolean("owned", mcp.Description("Default true (owned only); false to search all")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		f := pokemon.SpeciesFilter{
			Type:         getString(args, "type"),
			Role:         getString(args, "role"),
			SpeedTier:    getString(args, "speed_tier"),
			FinalEvoOnly: getBool(args, "final_evo_only"),
		}
		defaultOwned := true
		if v, ok := args["owned"]; ok && v != nil {
			b := getBool(args, "owned")
			f.Owned = &b
		} else {
			f.Owned = &defaultOwned
		}
		if v, ok := args["legendary"]; ok && v != nil {
			b := getBool(args, "legendary")
			f.Legendary = &b
		}
		results, err := svc.Pokemon.SearchSpecies("", getInt(args, "limit"), f)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if len(results) == 0 {
			return textResult("No owned Pokemon match those filters. Try broadening: remove role or speed_tier constraints."), nil
		}
		return jsonResult(results), nil
	})

	s.AddTool(mcp.NewTool("get_pokemon",
		mcp.WithDescription("Get full details for a Pokemon species including base stats, types, and available abilities."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Pokemon name (e.g. 'Garchomp')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := getString(req.GetArguments(), "name")
		sp, err := svc.Pokemon.GetSpeciesByName(name)
		if err != nil {
			return errResult(err.Error()), nil
		}
		abilities, _ := svc.Pokemon.GetAbilitiesForSpecies(sp.ID)
		return jsonResult(map[string]any{"species": sp, "abilities": abilities}), nil
	})

	s.AddTool(mcp.NewTool("get_moves",
		mcp.WithDescription("Get the full learnset (all moves a Pokemon can learn) for a species."),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Pokemon name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := getString(req.GetArguments(), "pokemon_name")
		sp, err := svc.Pokemon.GetSpeciesByName(name)
		if err != nil {
			return errResult(err.Error()), nil
		}
		moves, err := svc.Pokemon.GetLearnset(sp.ID)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if len(moves) == 0 {
			return textResult(fmt.Sprintf("No learnset data for %s. Use add_learnset_move to populate it as you discover moves in-game.", sp.Name)), nil
		}
		return jsonResult(moves), nil
	})

	s.AddTool(mcp.NewTool("add_learnset_move",
		mcp.WithDescription("Add a move to a species' Champions learnset. Use this when the user confirms a move is available in-game."),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("move_name", mcp.Required(), mcp.Description("Move name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		sp, err := svc.Pokemon.GetSpeciesByName(getString(args, "pokemon_name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		mv, err := svc.Pokemon.GetMoveByName(getString(args, "move_name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Pokemon.AddLearnsetMove(sp.ID, mv.ID); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Added %s to %s's learnset.", mv.Name, sp.Name)), nil
	})

	s.AddTool(mcp.NewTool("search_moves",
		mcp.WithDescription("Search moves by name/description with optional filters for type, category, power, and priority."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Name or description fragment (use empty string to list all)")),
		mcp.WithString("type", mcp.Description("Filter by type (e.g. 'fire', 'water')")),
		mcp.WithString("category", mcp.Description("Filter by category: physical, special, or status")),
		mcp.WithNumber("min_power", mcp.Description("Minimum base power")),
		mcp.WithNumber("max_power", mcp.Description("Maximum base power")),
		mcp.WithNumber("min_accuracy", mcp.Description("Minimum accuracy")),
		mcp.WithNumber("priority", mcp.Description("Exact priority value (e.g. 1 for Quick Attack, -1 for Trick Room)")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		f := pokemon.MoveFilter{
			Type:        getString(args, "type"),
			Category:    getString(args, "category"),
			MinPower:    getInt(args, "min_power"),
			MaxPower:    getInt(args, "max_power"),
			MinAccuracy: getInt(args, "min_accuracy"),
		}
		if v, ok := args["priority"]; ok && v != nil {
			p := getInt(args, "priority")
			f.Priority = &p
		}
		moves, err := svc.Pokemon.SearchMoves(getString(args, "query"), getInt(args, "limit"), f)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return jsonResult(moves), nil
	})

	s.AddTool(mcp.NewTool("get_item",
		mcp.WithDescription("Get details for a held item by name."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Item name (e.g. 'Choice Specs')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		item, err := svc.Pokemon.GetItemByName(getString(req.GetArguments(), "name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		return jsonResult(item), nil
	})

	s.AddTool(mcp.NewTool("search_items",
		mcp.WithDescription("Search held items by name or effect description, optionally filtered by owned or banned status."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search term (use empty string to list all)")),
		mcp.WithBoolean("owned", mcp.Description("If true/false, filter to owned or unowned items only")),
		mcp.WithBoolean("banned", mcp.Description("If true/false, filter to banned or legal items only")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		f := pokemon.ItemFilter{}
		if v, ok := args["owned"]; ok && v != nil {
			b := getBool(args, "owned")
			f.Owned = &b
		}
		if v, ok := args["banned"]; ok && v != nil {
			b := getBool(args, "banned")
			f.Banned = &b
		}
		items, err := svc.Pokemon.SearchItems(getString(args, "query"), getInt(args, "limit"), f)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return jsonResult(items), nil
	})

	s.AddTool(mcp.NewTool("set_owned",
		mcp.WithDescription("Mark a Pokemon or item as owned (or unowned). Use this when the user says they have or don't have a specific Pokemon or item."),
		mcp.WithString("type", mcp.Required(), mcp.Description("'pokemon' or 'item'")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Species or item name")),
		mcp.WithBoolean("owned", mcp.Required(), mcp.Description("true to mark owned, false to unmark")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		kind := getString(args, "type")
		name := getString(args, "name")
		owned := getBool(args, "owned")
		switch kind {
		case "pokemon":
			sp, err := svc.Pokemon.GetSpeciesByName(name)
			if err != nil {
				return errResult("Pokemon not found: " + name), nil
			}
			if err := svc.Pokemon.SetSpeciesOwned(sp.ID, owned); err != nil {
				return errResult(err.Error()), nil
			}
			status := "unowned"
			if owned {
				status = "owned"
			}
			return textResult(sp.Name + " marked as " + status), nil
		case "item":
			it, err := svc.Pokemon.GetItemByName(name)
			if err != nil {
				return errResult("Item not found: " + name), nil
			}
			if err := svc.Pokemon.SetItemOwned(it.ID, owned); err != nil {
				return errResult(err.Error()), nil
			}
			status := "unowned"
			if owned {
				status = "owned"
			}
			return textResult(it.Name + " marked as " + status), nil
		default:
			return errResult("type must be 'pokemon' or 'item'"), nil
		}
	})
}

// --- Team tools ---

func registerTeamTools(s *server.MCPServer, svc *Services) {
	s.AddTool(mcp.NewTool("create_team",
		mcp.WithDescription("Create a new empty VGC team."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Team name")),
		mcp.WithString("regulation", mcp.Required(), mcp.Description("Regulation set ID (e.g. 'I2')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		id, err := svc.Team.CreateTeam(getString(args, "name"), getString(args, "regulation"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Team created with ID %d.", id)), nil
	})

	s.AddTool(mcp.NewTool("list_teams",
		mcp.WithDescription("List all teams with their member counts."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		teams, err := svc.Team.ListTeams()
		if err != nil {
			return errResult(err.Error()), nil
		}
		if len(teams) == 0 {
			return textResult("No teams found. Use create_team to make one."), nil
		}
		return jsonResult(teams), nil
	})

	s.AddTool(mcp.NewTool("get_team",
		mcp.WithDescription("Get full team details including all members, moves, items, and abilities."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		t, err := svc.Team.GetTeam(getInt(req.GetArguments(), "team_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		return jsonResult(t), nil
	})

	s.AddTool(mcp.NewTool("add_pokemon",
		mcp.WithDescription("Add a Pokemon to a team by species name. The Pokemon is placed in the next open slot (max 6). The first legal ability for the species is assigned automatically."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name (e.g. 'Garchomp')")),
		mcp.WithString("ability", mcp.Description("Ability name — uses first available if omitted")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		teamID := getInt(args, "team_id")
		name := getString(args, "pokemon_name")

		sp, err := svc.Pokemon.GetSpeciesByName(name)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if !sp.IsFinalEvo {
			return errResult(sp.Name + " is not a final evolution"), nil
		}

		abilities, err := svc.Pokemon.GetAbilitiesForSpecies(sp.ID)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if len(abilities) == 0 {
			return errResult(sp.Name + " has no abilities configured — use add_learnset_move or seed ability data first"), nil
		}
		abilityID := abilities[0].ID
		if abName := getString(args, "ability"); abName != "" {
			ab, err := svc.Pokemon.GetAbilityByName(abName)
			if err != nil {
				return errResult(err.Error()), nil
			}
			if ok, _ := svc.Pokemon.HasAbility(sp.ID, ab.ID); !ok {
				return errResult(abName + " is not a valid ability for " + sp.Name), nil
			}
			abilityID = ab.ID
		}

		memberID, err := svc.Team.AddMember(teamID, sp.ID, abilityID)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Added %s to team %d as member ID %d.", sp.Name, teamID, memberID)), nil
	})

	s.AddTool(mcp.NewTool("remove_pokemon",
		mcp.WithDescription("Remove a Pokemon from a team by species name."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name to remove")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		memberID, err := svc.Team.GetMemberByTeamAndSpecies(getInt(args, "team_id"), getString(args, "pokemon_name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.RemoveMember(memberID); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Removed %s from team.", getString(args, "pokemon_name"))), nil
	})

	s.AddTool(mcp.NewTool("set_ability",
		mcp.WithDescription("Set the ability for a Pokemon on a team. The ability must be legal for that species."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("ability", mcp.Required(), mcp.Description("Ability name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		teamID := getInt(args, "team_id")
		pokeName := getString(args, "pokemon_name")

		memberID, err := svc.Team.GetMemberByTeamAndSpecies(teamID, pokeName)
		if err != nil {
			return errResult(err.Error()), nil
		}
		sp, _ := svc.Pokemon.GetSpeciesByName(pokeName)
		ab, err := svc.Pokemon.GetAbilityByName(getString(args, "ability"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if sp != nil {
			if ok, _ := svc.Pokemon.HasAbility(sp.ID, ab.ID); !ok {
				return errResult(ab.Name + " is not a valid ability for " + pokeName), nil
			}
		}
		if err := svc.Team.SetAbility(memberID, ab.ID); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Set %s's ability to %s.", pokeName, ab.Name)), nil
	})

	s.AddTool(mcp.NewTool("set_nature",
		mcp.WithDescription("Set the nature for a Pokemon on a team. Nature boosts one stat by 10% and reduces another by 10%."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("nature", mcp.Required(), mcp.Description("Nature name (e.g. 'Timid', 'Adamant')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nature := getString(args, "nature")
		removed := map[string]bool{"hardy": true, "docile": true, "bashful": true, "quirky": true}
		if removed[strings.ToLower(nature)] {
			return errf("%s is not a valid Stat Alignment in Pokemon Champions", nature), nil
		}
		if !pokemon.ValidNature(nature) {
			return errf("%q is not a valid nature.", nature), nil
		}
		memberID, err := svc.Team.GetMemberByTeamAndSpecies(getInt(args, "team_id"), getString(args, "pokemon_name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.SetNature(memberID, nature); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Set nature to %s.", nature)), nil
	})

	s.AddTool(mcp.NewTool("set_item",
		mcp.WithDescription("Set the held item for a Pokemon on a team. No two Pokemon on the same team can hold the same item."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("item", mcp.Required(), mcp.Description("Item name (e.g. 'Life Orb')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		teamID := getInt(args, "team_id")
		pokeName := getString(args, "pokemon_name")

		item, err := svc.Pokemon.GetItemByName(getString(args, "item"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if item.IsBanned {
			return errResult(item.Name + " is banned"), nil
		}

		// Check item clause: no other member on this team can hold the same item.
		t, err := svc.Team.GetTeam(teamID)
		if err != nil {
			return errResult(err.Error()), nil
		}
		for _, m := range t.Members {
			if m.Item != nil && m.Item.ID == item.ID && !strings.EqualFold(m.Species.Name, pokeName) {
				return errResult("item clause: " + item.Name + " already held by " + m.Species.Name), nil
			}
		}

		memberID, err := svc.Team.GetMemberByTeamAndSpecies(teamID, pokeName)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.SetItem(memberID, item.ID); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Set %s's item to %s.", pokeName, item.Name)), nil
	})

	s.AddTool(mcp.NewTool("set_moves",
		mcp.WithDescription("Set up to 4 moves for a Pokemon. All moves must be in the species' learnset. Use force=true to bypass learnset validation when the user has confirmed a move is available in-game."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("move1", mcp.Required(), mcp.Description("First move name")),
		mcp.WithString("move2", mcp.Description("Second move name")),
		mcp.WithString("move3", mcp.Description("Third move name")),
		mcp.WithString("move4", mcp.Description("Fourth move name")),
		mcp.WithBoolean("force", mcp.Description("Skip learnset validation (use when user confirms move is available in-game)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		teamID := getInt(args, "team_id")
		pokeName := getString(args, "pokemon_name")
		force := getBool(args, "force")

		sp, err := svc.Pokemon.GetSpeciesByName(pokeName)
		if err != nil {
			return errResult(err.Error()), nil
		}

		moveNames := []string{getString(args, "move1"), getString(args, "move2"), getString(args, "move3"), getString(args, "move4")}
		var moveIDs []int
		for _, name := range moveNames {
			if name == "" {
				continue
			}
			mv, err := svc.Pokemon.GetMoveByName(name)
			if err != nil {
				return errResult(err.Error()), nil
			}
			if !force {
				ok, lerr := svc.Pokemon.CanLearnMove(sp.ID, mv.ID)
				if !ok {
					msg := name + " is not in " + pokeName + "'s learnset"
					if lerr != nil {
						msg = lerr.Error()
					}
					return errResult(msg), nil
				}
			}
			moveIDs = append(moveIDs, mv.ID)
		}

		memberID, err := svc.Team.GetMemberByTeamAndSpecies(teamID, pokeName)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.SetMoves(memberID, moveIDs); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Set %d move(s) for %s.", len(moveIDs), pokeName)), nil
	})

	s.AddTool(mcp.NewTool("set_stats",
		mcp.WithDescription("Set stat points for a Pokemon (Champions: 66 total, max 32/stat). Provide explicit points per stat; omitted stats get 0."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithNumber("hp", mcp.Description("HP points (0-32)")),
		mcp.WithNumber("attack", mcp.Description("Attack points (0-32)")),
		mcp.WithNumber("defense", mcp.Description("Defense points (0-32)")),
		mcp.WithNumber("sp_attack", mcp.Description("Sp. Atk points (0-32)")),
		mcp.WithNumber("sp_defense", mcp.Description("Sp. Def points (0-32)")),
		mcp.WithNumber("speed", mcp.Description("Speed points (0-32)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		evs := team.StatSpread{
			HP:  getInt(args, "hp"),
			Atk: getInt(args, "attack"),
			Def: getInt(args, "defense"),
			SpA: getInt(args, "sp_attack"),
			SpD: getInt(args, "sp_defense"),
			Spe: getInt(args, "speed"),
		}
		for _, v := range []int{evs.HP, evs.Atk, evs.Def, evs.SpA, evs.SpD, evs.Spe} {
			if v < 0 || v > 32 {
				return errResult("each stat must be 0–32"), nil
			}
		}
		if t := evs.Total(); t > 66 {
			return errf("total %d exceeds 66-point pool", t), nil
		}
		memberID, err := svc.Team.GetMemberByTeamAndSpecies(getInt(args, "team_id"), getString(args, "pokemon_name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.SetEVs(memberID, evs); err != nil {
			return errResult(err.Error()), nil
		}
		remaining := 66 - evs.Total()
		return textResult(fmt.Sprintf("Set EVs: HP %d / Atk %d / Def %d / SpA %d / SpD %d / Spe %d (total %d, %d remaining).",
			evs.HP, evs.Atk, evs.Def, evs.SpA, evs.SpD, evs.Spe, evs.Total(), remaining)), nil
	})

	s.AddTool(mcp.NewTool("set_role",
		mcp.WithDescription("Set a strategic role label for a Pokemon (e.g. 'special_attacker', 'trick_room_setter', 'lead')."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("role", mcp.Required(), mcp.Description("Role label")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		memberID, err := svc.Team.GetMemberByTeamAndSpecies(getInt(args, "team_id"), getString(args, "pokemon_name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.SetRole(memberID, getString(args, "role")); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult("Role updated."), nil
	})

	s.AddTool(mcp.NewTool("set_notes",
		mcp.WithDescription("Set free-text notes on a team member (strategy tips, EV justification, etc.)."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("notes", mcp.Required(), mcp.Description("Notes text")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		memberID, err := svc.Team.GetMemberByTeamAndSpecies(getInt(args, "team_id"), getString(args, "pokemon_name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.SetMemberNotes(memberID, getString(args, "notes")); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult("Notes updated."), nil
	})

	s.AddTool(mcp.NewTool("validate_team",
		mcp.WithDescription("Check a team against all VGC rules. Returns a list of violations, or confirms the team is legal."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		t, err := svc.Team.GetTeam(getInt(req.GetArguments(), "team_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		reg, _ := svc.Team.GetRegulation(t.Regulation)
		violations := team.Validate(t, reg, svc.Pokemon)
		if len(violations) == 0 {
			return textResult("Team is legal — no violations found."), nil
		}
		return jsonResult(violations), nil
	})

	s.AddTool(mcp.NewTool("analyse_team",
		mcp.WithDescription("Analyse a team's type coverage, defensive weaknesses, speed tiers, and detected archetypes."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		t, err := svc.Team.GetTeam(getInt(req.GetArguments(), "team_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		return jsonResult(team.Analyse(t)), nil
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

	s.AddTool(mcp.NewTool("delete_team",
		mcp.WithDescription("Delete a team and all its members permanently."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID to delete")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := getInt(req.GetArguments(), "team_id")
		if err := svc.Team.DeleteTeam(id); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Team %d deleted.", id)), nil
	})

	s.AddTool(mcp.NewTool("set_team_notes",
		mcp.WithDescription("Set the strategy notes for a team (markdown supported)."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("notes", mcp.Required(), mcp.Description("Markdown notes text")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		if err := svc.Team.UpdateTeamNotes(getInt(args, "team_id"), getString(args, "notes")); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult("Team notes updated."), nil
	})

	s.AddTool(mcp.NewTool("rename_team",
		mcp.WithDescription("Rename a team."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("name", mcp.Required(), mcp.Description("New team name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		if err := svc.Team.RenameTeam(getInt(args, "team_id"), getString(args, "name")); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult("Team renamed to " + getString(args, "name") + "."), nil
	})

	s.AddTool(mcp.NewTool("set_tera_type",
		mcp.WithDescription("Set the Tera Type for a Pokemon on a team."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("tera_type", mcp.Required(), mcp.Description("Type name (e.g. 'fire', 'fairy')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		memberID, err := svc.Team.GetMemberByTeamAndSpecies(getInt(args, "team_id"), getString(args, "pokemon_name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.SetTeraType(memberID, getString(args, "tera_type")); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Set Tera Type to %s.", getString(args, "tera_type"))), nil
	})

	s.AddTool(mcp.NewTool("set_nickname",
		mcp.WithDescription("Set a nickname for a Pokemon on a team."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("nickname", mcp.Required(), mcp.Description("Nickname (empty string to clear)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		memberID, err := svc.Team.GetMemberByTeamAndSpecies(getInt(args, "team_id"), getString(args, "pokemon_name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.SetNickname(memberID, getString(args, "nickname")); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult("Nickname updated."), nil
	})

	s.AddTool(mcp.NewTool("swap_slots",
		mcp.WithDescription("Swap two Pokemon slots within a team."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithNumber("slot_a", mcp.Required(), mcp.Description("First slot number (1-6)")),
		mcp.WithNumber("slot_b", mcp.Required(), mcp.Description("Second slot number (1-6)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		if err := svc.Team.SwapSlots(getInt(args, "team_id"), getInt(args, "slot_a"), getInt(args, "slot_b")); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Swapped slots %d and %d.", getInt(args, "slot_a"), getInt(args, "slot_b"))), nil
	})

	s.AddTool(mcp.NewTool("copy_team",
		mcp.WithDescription("Duplicate a team with all members, moves, items and stats under a new name."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Source team ID")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name for the new team")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		newID, err := svc.Team.CopyTeam(getInt(args, "team_id"), getString(args, "name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Team copied as \"%s\" (ID %d).", getString(args, "name"), newID)), nil
	})

	s.AddTool(mcp.NewTool("set_team_notes",
		mcp.WithDescription("Set free-text notes on a team (strategy overview, tournament notes, etc.)."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("notes", mcp.Required(), mcp.Description("Notes text (markdown supported)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		if err := svc.Team.UpdateTeamNotes(getInt(args, "team_id"), getString(args, "notes")); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult("Team notes updated."), nil
	})
}

// --- Knowledge tools ---

func registerKnowledgeTools(s *server.MCPServer, svc *Services) {
	s.AddTool(mcp.NewTool("search_knowledge",
		mcp.WithDescription("Search the strategy knowledge base (VGC rules, tier lists, guides) using full-text search."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search terms (e.g. 'trick room setters', 'Regulation H restricted')")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 5)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		results, err := svc.Knowledge.Search(getString(args, "query"), getInt(args, "limit"))
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

// --- Regulation tools ---

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

// registerAnalysisTools adds calc_stats and training_cost tools.
func registerAnalysisTools(s *server.MCPServer, svc *Services) {
	// calc_stats: calculate final Level 50 stats for a species + spread + nature.
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
		sp, err := svc.Pokemon.GetSpeciesByName(getString(args, "species"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		spread := [6]int{
			getInt(args, "hp"),
			getInt(args, "attack"),
			getInt(args, "defense"),
			getInt(args, "sp_attack"),
			getInt(args, "sp_defense"),
			getInt(args, "speed"),
		}
		bases := [6]int{sp.HP, sp.Attack, sp.Defense, sp.SpAttack, sp.SpDefense, sp.Speed}
		statNames := [6]string{"HP", "Attack", "Defense", "Sp.Atk", "Sp.Def", "Speed"}
		statKeys := [6]pokemon.Stat{pokemon.StatHP, pokemon.StatAtk, pokemon.StatDef, pokemon.StatSpA, pokemon.StatSpD, pokemon.StatSpe}

		// Resolve nature multipliers.
		boosted := pokemon.Stat("")
		reduced := pokemon.Stat("")
		natureName := getString(args, "nature")
		if natureName != "" {
			n, ok := pokemon.NatureByName(natureName)
			if !ok {
				return errf("%q is not a valid nature", natureName), nil
			}
			boosted = n.Boosted
			reduced = n.Reduced
		}

		// Formula (IVs=31, Lv50):
		//   HP:     floor((2*B + 31 + SP*2) * 50/100) + 60
		//   Other:  floor((2*B + 31 + SP*2) * 50/100 + 5) * NatureMult
		var b strings.Builder
		fmt.Fprintf(&b, "%-9s %6s %4s %7s\n", "Stat", "Base", "SP", "Final")
		fmt.Fprintf(&b, "%-9s %6s %4s %7s\n", "---------", "------", "----", "-------")
		for i := range 6 {
			inner := 2*bases[i] + 31 + spread[i]*2
			var final int
			if statKeys[i] == pokemon.StatHP {
				final = int(math.Floor(float64(inner)*50.0/100.0)) + 60
			} else {
				mult := 1.0
				switch statKeys[i] {
				case boosted:
					mult = 1.1
				case reduced:
					mult = 0.9
				}
				final = int(math.Floor((math.Floor(float64(inner)*50.0/100.0) + 5) * mult))
			}
			fmt.Fprintf(&b, "%-9s %6d %4d %7d\n", statNames[i], bases[i], spread[i], final)
		}
		if natureName != "" {
			fmt.Fprintf(&b, "\nNature: %s", natureName)
			if boosted != "" {
				fmt.Fprintf(&b, " (+%s / -%s)", boosted, reduced)
			}
			b.WriteString("\n")
		}
		return textResult(b.String()), nil
	})

	// training_cost: calculate VP cost to train a team from scratch.
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
			if m.Species == nil {
				continue
			}
			// SP cost: 2 VP per stat point.
			spTotal := m.EVs.HP + m.EVs.Atk + m.EVs.Def + m.EVs.SpA + m.EVs.SpD + m.EVs.Spe
			spVP := spTotal * 2

			// Nature cost: 200 VP unless Serious (neutral) or unset.
			natureVP := 0
			if m.Nature != "" && !strings.EqualFold(m.Nature, "Serious") {
				natureVP = 200
			}

			// Move cost: 100 VP per move.
			moveVP := len(m.Moves) * 100

			// Ability cost: 400 VP if not the first/default ability for the species.
			abilityVP := 0
			if m.Ability != nil {
				if abilities, err := svc.Pokemon.GetAbilitiesForSpecies(m.Species.ID); err == nil && len(abilities) > 0 {
					if abilities[0].ID != m.Ability.ID {
						abilityVP = 400
					}
				}
			}

			memberTotal := spVP + natureVP + moveVP + abilityVP
			teamTotal += memberTotal
			fmt.Fprintf(&b, "%-16s %6d %7d %7d %7d %8d\n", m.Species.Name, spVP, natureVP, moveVP, abilityVP, memberTotal)
		}

		fmt.Fprintf(&b, "%-16s %6s %7s %7s %7s %8s\n", "----------------", "------", "-------", "-------", "-------", "--------")
		fmt.Fprintf(&b, "%-16s %44d VP\n", "TEAM TOTAL", teamTotal)
		return textResult(b.String()), nil
	})
}

// buildEVSpread distributes 66 stat points (Pokemon Champions system) equally
// across the chosen stats. Max 32 per stat, 66 total.
// 2 stats: 32/32 + 2 remainder on first. 3 stats: 22/22/22.

// registerEvaluateTools adds evaluate_pokemon.
func registerEvaluateTools(s *server.MCPServer, svc *Services) {
	s.AddTool(mcp.NewTool("evaluate_pokemon",
		mcp.WithDescription("Evaluate a Pokemon: type chart, offensive coverage, stat role, bulk, speed tier. Pass team_id for move/EV analysis."),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name (e.g. 'Garchomp')")),
		mcp.WithNumber("team_id", mcp.Description("Team ID — enables member-level move/EV analysis")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		spName := getString(args, "pokemon_name")
		sp, err := svc.Pokemon.GetSpeciesByName(spName)
		if err != nil {
			return textResult("Error: " + err.Error()), nil
		}

		// Check for optional team_id for member-level evaluation.
		teamIDFloat, hasTeam := args["team_id"].(float64)
		if hasTeam && teamIDFloat > 0 {
			teamID := int(teamIDFloat)
			t, err := svc.Team.GetTeam(teamID)
			if err != nil {
				return textResult("Error loading team: " + err.Error()), nil
			}
			// Find the member matching the species name.
			var matched *team.Member
			for i := range t.Members {
				if t.Members[i].Species != nil &&
					strings.EqualFold(t.Members[i].Species.Name, sp.Name) {
					matched = &t.Members[i]
					break
				}
			}
			if matched == nil {
				return textResult(fmt.Sprintf("%s is not on team %d; showing species-level evaluation.", sp.Name, teamID)), nil
			}
			ev := team.EvaluateMember(matched)
			return textResult(formatMemberEval(ev)), nil
		}

		// Species-level evaluation — fetch learnset.
		learnset, err := svc.Pokemon.GetLearnset(sp.ID)
		if err != nil {
			learnset = nil // degrade gracefully
		}
		ev := team.EvaluateSpecies(sp, learnset)
		return textResult(formatSpeciesEval(ev)), nil
	})
}

func formatSpeciesEval(ev team.SpeciesEval) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s)\n", ev.Name, strings.Join(ev.Types, "/"))
	fmt.Fprintf(&b, "Role: %s | Speed: %s\n", ev.StatRole, ev.SpeedTier)
	fmt.Fprintf(&b, "Bulk: physical %.0f | special %.0f\n", ev.PhysicalBulk, ev.SpecialBulk)
	tm := ev.TypeMatchup
	if len(tm.Immune) > 0 {
		fmt.Fprintf(&b, "Immune (0×):     %s\n", strings.Join(tm.Immune, ", "))
	}
	if len(tm.Quarter) > 0 {
		fmt.Fprintf(&b, "Quarter (0.25×): %s\n", strings.Join(tm.Quarter, ", "))
	}
	if len(tm.Half) > 0 {
		fmt.Fprintf(&b, "Resists (0.5×):  %s\n", strings.Join(tm.Half, ", "))
	}
	if len(tm.Double) > 0 {
		fmt.Fprintf(&b, "Weak (2×):       %s\n", strings.Join(tm.Double, ", "))
	}
	if len(tm.Quadruple) > 0 {
		fmt.Fprintf(&b, "Very weak (4×):  %s\n", strings.Join(tm.Quadruple, ", "))
	}
	if len(ev.OffensiveCoverage) > 0 {
		fmt.Fprintf(&b, "Offensive coverage (SE): %s\n", strings.Join(ev.OffensiveCoverage, ", "))
	}
	return b.String()
}

func formatMemberEval(ev team.MemberEval) string {
	var b strings.Builder
	b.WriteString(formatSpeciesEval(ev.SpeciesEval))
	b.WriteString("--- member analysis ---\n")
	flags := []string{}
	if ev.HasPriorityMove {
		flags = append(flags, "priority move")
	}
	if ev.HasSetupMove {
		flags = append(flags, "setup move")
	}
	if ev.HasRecoveryMove {
		flags = append(flags, "recovery move")
	}
	if ev.HasRedirection {
		flags = append(flags, "redirection")
	}
	if len(flags) > 0 {
		fmt.Fprintf(&b, "Flags: %s\n", strings.Join(flags, ", "))
	}
	if ev.MoveStatMismatch {
		fmt.Fprintf(&b, "EV warning: %s\n", ev.EVEfficiency)
	} else {
		fmt.Fprintf(&b, "EV efficiency: %s\n", ev.EVEfficiency)
	}
	return b.String()
}

// registerLogTools adds add_team_log and get_team_logs.
func registerLogTools(s *server.MCPServer, svc *Services) {
	s.AddTool(mcp.NewTool("add_team_log",
		mcp.WithDescription("Append a combat/session log entry to a team. Use to record notable matchups, what worked, what didn't."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("entry", mcp.Required(), mcp.Description("Log entry text (markdown supported)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		id, err := svc.Team.AddLog(getInt(args, "team_id"), getString(args, "entry"))
		if err != nil {
			return textResult("Error: " + err.Error()), nil
		}
		return textResult(fmt.Sprintf("Log entry %d recorded.", id)), nil
	})

	s.AddTool(mcp.NewTool("get_team_logs",
		mcp.WithDescription("Retrieve all combat/session log entries for a team, newest first."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		logs, err := svc.Team.GetLogs(getInt(req.GetArguments(), "team_id"))
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
