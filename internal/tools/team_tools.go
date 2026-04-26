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
		evs := statSpreadFromArgs(args)
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
