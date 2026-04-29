package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/user/pokemon-team-manager/internal/pokemon"
)

func registerTeamTools(s *server.MCPServer, svc *Services) {
	s.AddTool(mcp.NewTool("create_team",
		mcp.WithDescription("Create a new empty VGC team."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Team name")),
		mcp.WithString("regulation", mcp.Required(), mcp.Description("Regulation set ID (e.g. 'M-A')")),
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
		mcp.WithDescription("Add a Pokemon to a team. Placed in the next open slot (max 6)."),
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

		abilities, err := svc.Pokemon.GetAbilitiesForSpecies(sp.Slug)
		if err != nil || len(abilities) == 0 {
			return errResult(sp.Name + " has no abilities configured — use add_species_ability first"), nil
		}
		abilitySlug := abilities[0].Slug
		if abName := getString(args, "ability"); abName != "" {
			ab, err := svc.Pokemon.GetAbilityByName(abName)
			if err != nil {
				return errResult(err.Error()), nil
			}
			if ok, _ := svc.Pokemon.HasAbility(sp.Slug, ab.Slug); !ok {
				return errResult(abName + " is not a valid ability for " + sp.Name), nil
			}
			abilitySlug = ab.Slug
		}

		configID, err := svc.Team.AddMember(teamID, sp.Slug, abilitySlug)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Added %s to team %d (config ID %d).", sp.Name, teamID, configID)), nil
	})

	s.AddTool(mcp.NewTool("remove_pokemon",
		mcp.WithDescription("Remove a Pokemon from a team by species name."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name to remove")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		configID, err := svc.Team.GetConfigByTeamAndSpecies(getInt(args, "team_id"), getString(args, "pokemon_name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.RemoveMember(configID); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Removed %s from team.", getString(args, "pokemon_name"))), nil
	})

	s.AddTool(mcp.NewTool("set_ability",
		mcp.WithDescription("Set the ability for a Pokemon on a team."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("ability", mcp.Required(), mcp.Description("Ability name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		teamID := getInt(args, "team_id")
		pokeName := getString(args, "pokemon_name")

		configID, err := svc.Team.GetConfigByTeamAndSpecies(teamID, pokeName)
		if err != nil {
			return errResult(err.Error()), nil
		}
		sp, _ := svc.Pokemon.GetSpeciesByName(pokeName)
		ab, err := svc.Pokemon.GetAbilityByName(getString(args, "ability"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if sp != nil {
			if ok, _ := svc.Pokemon.HasAbility(sp.Slug, ab.Slug); !ok {
				return errResult(ab.Name + " is not a valid ability for " + pokeName), nil
			}
		}
		if err := svc.Team.SetAbility(configID, ab.Slug); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Set %s's ability to %s.", pokeName, ab.Name)), nil
	})

	s.AddTool(mcp.NewTool("set_nature",
		mcp.WithDescription("Set the nature for a Pokemon on a team."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("nature", mcp.Required(), mcp.Description("Nature name (e.g. 'Timid', 'Adamant')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nature := getString(args, "nature")
		if !pokemon.ValidNature(nature) {
			return errf("%q is not a valid nature in Pokemon Champions.", nature), nil
		}
		if strings.EqualFold(nature, "Serious") {
			return errResult("Serious is the default placeholder nature (free, no stat changes). Don't set it explicitly — leave the slot alone, or pick a real nature with a beneficial +/- spread (Timid, Modest, Adamant, Jolly, Bold, Calm, Quiet, Brave, etc.)."), nil
		}
		configID, err := svc.Team.GetConfigByTeamAndSpecies(getInt(args, "team_id"), getString(args, "pokemon_name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.SetNature(configID, nature); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Set nature to %s.", nature)), nil
	})

	s.AddTool(mcp.NewTool("set_item",
		mcp.WithDescription("Set the held item for a Pokemon on a team. Item clause enforced."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("item", mcp.Required(), mcp.Description("Item name (e.g. 'Focus Band')")),
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

		t, err := svc.Team.GetTeam(teamID)
		if err != nil {
			return errResult(err.Error()), nil
		}
		for _, m := range t.Members {
			if m.Config == nil || m.Config.Item == nil {
				continue
			}
			if m.Config.Item.Slug == item.Slug && m.Config.Species != nil && !strings.EqualFold(m.Config.Species.Name, pokeName) {
				return errResult("item clause: " + item.Name + " already held by " + m.Config.Species.Name), nil
			}
		}

		configID, err := svc.Team.GetConfigByTeamAndSpecies(teamID, pokeName)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.SetItem(configID, item.Slug); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Set %s's item to %s.", pokeName, item.Name)), nil
	})

	s.AddTool(mcp.NewTool("set_moves",
		mcp.WithDescription("Set up to 4 moves for a Pokemon. All moves must be in the species' Champions learnset."),
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

		var moveSlugs []string
		for _, key := range []string{"move1", "move2", "move3", "move4"} {
			name := getString(args, key)
			if name == "" {
				continue
			}
			mv, err := svc.Pokemon.GetMoveByName(name)
			if err != nil {
				return errResult(err.Error()), nil
			}
			ok, _ := svc.Pokemon.CanLearnMove(sp.Slug, mv.Slug)
			if !ok {
				return errResult(name + " is not in " + pokeName + "'s Champions learnset. Call get_moves to see what IS legal and pick an alternative. Do NOT call add_learnset_move — the data is fixed externally. If no legal alternative fits, return that fact to the caller."), nil
			}
			moveSlugs = append(moveSlugs, mv.Slug)
		}

		configID, err := svc.Team.GetConfigByTeamAndSpecies(teamID, pokeName)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.SetMoves(configID, moveSlugs); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(fmt.Sprintf("Set %d move(s) for %s.", len(moveSlugs), pokeName)), nil
	})

	s.AddTool(mcp.NewTool("set_stats",
		mcp.WithDescription("Set stat points for a Pokemon (Champions: 66 total, max 32/stat)."),
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
		evs := statSpreadFromArgs(args)
		for _, v := range []int{evs.HP, evs.Atk, evs.Def, evs.SpA, evs.SpD, evs.Spe} {
			if v < 0 || v > 32 {
				return errResult("each stat must be 0–32"), nil
			}
		}
		if t := evs.Total(); t > 66 {
			return errf("total %d exceeds 66-point pool", t), nil
		}
		configID, err := svc.Team.GetConfigByTeamAndSpecies(getInt(args, "team_id"), getString(args, "pokemon_name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.SetEVs(configID, evs); err != nil {
			return errResult(err.Error()), nil
		}
		remaining := 66 - evs.Total()
		return textResult(fmt.Sprintf("Set EVs: HP %d / Atk %d / Def %d / SpA %d / SpD %d / Spe %d (total %d, %d remaining).",
			evs.HP, evs.Atk, evs.Def, evs.SpA, evs.SpD, evs.Spe, evs.Total(), remaining)), nil
	})

	s.AddTool(mcp.NewTool("set_role",
		mcp.WithDescription("Set a strategic role label for a Pokemon (e.g. 'trick_room_setter', 'lead')."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("role", mcp.Required(), mcp.Description("Role label")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		configID, err := svc.Team.GetConfigByTeamAndSpecies(getInt(args, "team_id"), getString(args, "pokemon_name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.SetRole(configID, getString(args, "role")); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult("Role updated."), nil
	})

	s.AddTool(mcp.NewTool("set_notes",
		mcp.WithDescription("Set build notes on a team member (EV justification, matchup tips). Use set_team_notes for team-wide strategy."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("notes", mcp.Required(), mcp.Description("Notes text")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		configID, err := svc.Team.GetConfigByTeamAndSpecies(getInt(args, "team_id"), getString(args, "pokemon_name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.SetConfigNotes(configID, getString(args, "notes")); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult("Notes updated."), nil
	})

	s.AddTool(mcp.NewTool("set_team_notes",
		mcp.WithDescription("Set the team-level strategy: gameplan, key threats, synergy. Separate from per-member build notes."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("strategy", mcp.Required(), mcp.Description("Markdown strategy text")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		if err := svc.Team.UpdateTeamStrategy(getInt(args, "team_id"), getString(args, "strategy")); err != nil {
			return errResult(err.Error()), nil
		}
		return textResult("Team strategy updated."), nil
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

	s.AddTool(mcp.NewTool("set_nickname",
		mcp.WithDescription("Set a nickname for a Pokemon on a team."),
		mcp.WithNumber("team_id", mcp.Required(), mcp.Description("Team ID")),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Species name")),
		mcp.WithString("nickname", mcp.Required(), mcp.Description("Nickname (empty string to clear)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		configID, err := svc.Team.GetConfigByTeamAndSpecies(getInt(args, "team_id"), getString(args, "pokemon_name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if err := svc.Team.SetNickname(configID, getString(args, "nickname")); err != nil {
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

	// delete_team is intentionally NOT registered. Whole-team deletion is a
	// user-only operation; agents (in-process or external MCP clients) cannot
	// remove teams. The user removes teams via the /teams page in the web UI.
	// See the matching note in internal/handlers/agent_tools.go.
}
