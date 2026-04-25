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
3. search_pokemon query="name" → verify species name
4. add_pokemon team_id=N pokemon_name="..." → adds to next slot (max 6)
5. set_ability / set_nature / set_item / set_moves / set_stats / set_role
6. validate_team team_id=N → check violations
7. analyse_team team_id=N → coverage & speed tiers
8. export_team team_id=N → markdown output

RULES: 6 Pokemon, no duplicate species or items, final evos only,
Stat points: 66 total, max 32/stat (Pokemon Champions system). 0 restricted Legendaries (Reg I2).

STATS for set_stats: hp attack defense sp_attack sp_defense speed
Use search_knowledge for strategy docs and tier lists.`

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
			for _, idStr := range strings.Split(teamIDs, ",") {
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
		sections := strings.Split(include, ",")
		for _, sec := range sections {
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
	s.AddTool(mcp.NewTool("search_pokemon",
		mcp.WithDescription("Search for Pokemon species by name. Returns matching species with base stats and types."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Name or partial name to search for")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 10)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		results, err := svc.Pokemon.SearchSpecies(getString(args, "query"), getInt(args, "limit"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if len(results) == 0 {
			return textResult("No Pokemon found matching that query."), nil
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
		return jsonResult(moves), nil
	})

	s.AddTool(mcp.NewTool("search_moves",
		mcp.WithDescription("Search moves by name, type, or category."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Name or description fragment to search")),
		mcp.WithString("type", mcp.Description("Filter by type (e.g. 'fire', 'water')")),
		mcp.WithString("category", mcp.Description("Filter by category: physical, special, or status")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		moves, err := svc.Pokemon.SearchMoves(
			getString(args, "query"),
			getString(args, "type"),
			getString(args, "category"),
			getInt(args, "limit"),
		)
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
		mcp.WithDescription("Search held items by name or effect description."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search term")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		items, err := svc.Pokemon.SearchItems(getString(args, "query"), getInt(args, "limit"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		return jsonResult(items), nil
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
		if err != nil || len(abilities) == 0 {
			return errResult(err.Error()), nil
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
		mcp.WithDescription("Set up to 4 moves for a Pokemon. All moves must be in the species' learnset."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("move1", mcp.Required(), mcp.Description("First move name")),
		mcp.WithString("move2", mcp.Description("Second move name")),
		mcp.WithString("move3", mcp.Description("Third move name")),
		mcp.WithString("move4", mcp.Description("Fourth move name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		teamID := getInt(args, "team_id")
		pokeName := getString(args, "pokemon_name")

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
			ok, lerr := svc.Pokemon.CanLearnMove(sp.ID, mv.ID)
			if !ok {
				msg := name + " is not in " + pokeName + "'s learnset"
				if lerr != nil {
					msg = lerr.Error()
				}
				return errResult(msg), nil
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
		for i := 0; i < 6; i++ {
			inner := 2*bases[i] + 31 + spread[i]*2
			var final int
			if statKeys[i] == pokemon.StatHP {
				final = int(math.Floor(float64(inner)*50.0/100.0)) + 60
			} else {
				mult := 1.0
				if statKeys[i] == boosted {
					mult = 1.1
				} else if statKeys[i] == reduced {
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
		fmt.Fprintf(&b, "%-16s %8s %9s %8s\n", "Pokemon", "SP VP", "Nature VP", "Total VP")
		fmt.Fprintf(&b, "%-16s %8s %9s %8s\n", "----------------", "--------", "---------", "--------")

		teamTotal := 0
		for _, m := range t.Members {
			if m.Species == nil {
				continue
			}
			// SP cost: each stat point costs 2 VP.
			spTotal := m.EVs.HP + m.EVs.Atk + m.EVs.Def + m.EVs.SpA + m.EVs.SpD + m.EVs.Spe
			spVP := spTotal * 2

			// Nature cost: 200 VP unless nature is Serious (neutral) or unset.
			natureVP := 0
			if m.Nature != "" && !strings.EqualFold(m.Nature, "Serious") {
				natureVP = 200
			}

			memberTotal := spVP + natureVP
			teamTotal += memberTotal

			fmt.Fprintf(&b, "%-16s %8d %9d %8d\n", m.Species.Name, spVP, natureVP, memberTotal)
		}

		fmt.Fprintf(&b, "%-16s %8s %9s %8s\n", "----------------", "--------", "---------", "--------")
		fmt.Fprintf(&b, "%-16s %35d VP\n", "TEAM TOTAL", teamTotal)
		return textResult(b.String()), nil
	})
}

// buildEVSpread distributes 66 stat points (Pokemon Champions system) equally
// across the chosen stats. Max 32 per stat, 66 total.
// 2 stats: 32/32 + 2 remainder on first. 3 stats: 22/22/22.
