package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

// BuildPtmTools returns the agent tool list for the given services.
// Does NOT include chat_agent — that tool is MCP-only.
func BuildPtmTools(svc *Services) []AgentTool {
	str := func(args map[string]any, k string) string {
		v, _ := args[k].(string)
		return strings.TrimSpace(v)
	}
	num := func(args map[string]any, k string) int {
		switch v := args[k].(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
		return 0
	}
	prop := func(typ, desc string) map[string]any {
		return map[string]any{"type": typ, "description": desc}
	}
	schema := func(name, desc string, props map[string]any, required []string) OlamaTool {
		return OlamaTool{
			Type: "function",
			Function: OlamaToolFn{
				Name:        name,
				Description: desc,
				Parameters: map[string]any{
					"type":       "object",
					"properties": props,
					"required":   required,
				},
			},
		}
	}

	formatItem := func(it *pokemon.Item) string {
		flags := []string{}
		if it.IsBanned {
			flags = append(flags, "BANNED")
		}
		if it.Owned {
			flags = append(flags, "owned")
		}
		flagStr := ""
		if len(flags) > 0 {
			flagStr = " [" + strings.Join(flags, ", ") + "]"
		}
		return fmt.Sprintf("%s (vp %d)%s — %s", it.Name, it.VPCost, flagStr, it.Description)
	}

	return []AgentTool{
		{
			Schema: schema("create_team", "Create a new empty VGC team.",
				map[string]any{
					"name":       prop("string", "Team name"),
					"regulation": prop("string", "Regulation set ID (e.g. 'H')"),
				},
				[]string{"name", "regulation"}),
			Execute: func(args map[string]any) string {
				id, err := svc.Team.CreateTeam(str(args, "name"), str(args, "regulation"))
				if err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Team created with ID %d.", id)
			},
		},
		{
			Schema: schema("copy_team", "Duplicate a team with all members, moves, items and stats under a new name.",
				map[string]any{
					"team_id": prop("integer", "Source team ID"),
					"name":    prop("string", "Name for the new team"),
				},
				[]string{"team_id", "name"}),
			Execute: func(args map[string]any) string {
				newID, err := svc.Team.CopyTeam(num(args, "team_id"), str(args, "name"))
				if err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Team copied as \"%s\" (ID %d).", str(args, "name"), newID)
			},
		},
		{
			Schema: schema("add_pokemon", "Add a Pokemon to a team. Placed in the next open slot (max 6).",
				map[string]any{
					"team_id":      prop("integer", "Team ID"),
					"pokemon_name": prop("string", "Species name (e.g. 'Garchomp')"),
					"ability":      prop("string", "Ability name — uses first available if omitted"),
				},
				[]string{"team_id", "pokemon_name"}),
			Execute: func(args map[string]any) string {
				teamID := num(args, "team_id")
				name := str(args, "pokemon_name")
				sp, err := svc.Pokemon.GetSpeciesByName(name)
				if err != nil {
					return "error: " + err.Error()
				}
				abilities, err := svc.Pokemon.GetAbilitiesForSpecies(sp.Slug)
				if err != nil || len(abilities) == 0 {
					return "error: " + sp.Name + " has no abilities in the database. Use add_species_ability to add its abilities first (e.g. Mirror Armor, Pressure), then retry add_pokemon."
				}
				abilitySlug := abilities[0].Slug
				if abName := str(args, "ability"); abName != "" {
					ab, err := svc.Pokemon.GetAbilityByName(abName)
					if err != nil {
						return "error: " + err.Error()
					}
					if ok, _ := svc.Pokemon.HasAbility(sp.Slug, ab.Slug); !ok {
						return "error: " + abName + " is not a valid ability for " + sp.Name
					}
					abilitySlug = ab.Slug
				}
				memberID, err := svc.Team.AddMember(teamID, sp.Slug, abilitySlug)
				if err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Added %s to team %d as member ID %d.", sp.Name, teamID, memberID)
			},
		},
		{
			Schema: schema("remove_pokemon", "Remove a Pokemon from a team by species name.",
				map[string]any{
					"team_id":      prop("integer", "Team ID"),
					"pokemon_name": prop("string", "Species name to remove"),
				},
				[]string{"team_id", "pokemon_name"}),
			Execute: func(args map[string]any) string {
				memberID, err := svc.Team.GetConfigByTeamAndSpecies(num(args, "team_id"), str(args, "pokemon_name"))
				if err != nil {
					return "error: " + err.Error()
				}
				if err := svc.Team.RemoveMember(memberID); err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Removed %s from team.", str(args, "pokemon_name"))
			},
		},
		{
			Schema: schema("set_ability", "Set the ability for a Pokemon on a team.",
				map[string]any{
					"team_id":      prop("integer", "Team ID"),
					"pokemon_name": prop("string", "Species name"),
					"ability":      prop("string", "Ability name"),
				},
				[]string{"team_id", "pokemon_name", "ability"}),
			Execute: func(args map[string]any) string {
				teamID := num(args, "team_id")
				pokeName := str(args, "pokemon_name")
				memberID, err := svc.Team.GetConfigByTeamAndSpecies(teamID, pokeName)
				if err != nil {
					return "error: " + err.Error()
				}
				sp, _ := svc.Pokemon.GetSpeciesByName(pokeName)
				ab, err := svc.Pokemon.GetAbilityByName(str(args, "ability"))
				if err != nil {
					return "error: " + err.Error()
				}
				if sp != nil {
					if ok, _ := svc.Pokemon.HasAbility(sp.Slug, ab.Slug); !ok {
						return "error: " + ab.Name + " is not a valid ability for " + pokeName
					}
				}
				if err := svc.Team.SetAbility(memberID, ab.Slug); err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Set %s's ability to %s.", pokeName, ab.Name)
			},
		},
		{
			Schema: schema("set_nature", "Set the nature for a Pokemon on a team.",
				map[string]any{
					"team_id":      prop("integer", "Team ID"),
					"pokemon_name": prop("string", "Species name"),
					"nature":       prop("string", "Nature name (e.g. 'Timid', 'Adamant')"),
				},
				[]string{"team_id", "pokemon_name", "nature"}),
			Execute: func(args map[string]any) string {
				nature := str(args, "nature")
				removed := map[string]bool{"hardy": true, "docile": true, "bashful": true, "quirky": true}
				if removed[strings.ToLower(nature)] {
					return fmt.Sprintf("error: %s is not a valid Stat Alignment in Pokemon Champions", nature)
				}
				if strings.EqualFold(nature, "Serious") {
					return "error: Serious is the default placeholder nature (free, no stat changes). Don't set it explicitly — leave the slot alone, or pick a real nature with a beneficial +/- spread (Timid, Modest, Adamant, Jolly, Bold, Calm, Quiet, Brave, etc.)."
				}
				memberID, err := svc.Team.GetConfigByTeamAndSpecies(num(args, "team_id"), str(args, "pokemon_name"))
				if err != nil {
					return "error: " + err.Error()
				}
				if err := svc.Team.SetNature(memberID, nature); err != nil {
					return "error: " + err.Error()
				}
				return "Set nature to " + nature + "."
			},
		},
		{
			Schema: schema("set_item", "Set the held item for a Pokemon on a team. Item clause enforced.",
				map[string]any{
					"team_id":      prop("integer", "Team ID"),
					"pokemon_name": prop("string", "Species name"),
					"item":         prop("string", "Item name (e.g. 'Focus Band')"),
				},
				[]string{"team_id", "pokemon_name", "item"}),
			Execute: func(args map[string]any) string {
				teamID := num(args, "team_id")
				pokeName := str(args, "pokemon_name")
				itemName := str(args, "item")
				// Hard-block known mainline items that don't exist in Champions.
				// The system prompt's general guidance gets diluted under load,
				// so we name the absent items explicitly with a curt rejection.
				lower := strings.ToLower(itemName)
				absent := map[string]bool{
					"life orb": true, "eviolite": true, "assault vest": true,
					"choice specs": true, "choice band": true,
					"booster energy": true, "clear amulet": true,
					"mirror herb": true, "loaded dice": true,
					"covert cloak": true, "safety goggles": true,
				}
				if absent[lower] {
					return fmt.Sprintf("error: %s does NOT exist in Pokemon Champions. STOP trying mainline items. Use one of: Choice Scarf, Focus Sash, Focus Band, Leftovers, Sitrus Berry, Lum Berry, Light Ball, Scope Lens, Mental Herb, Black Glasses, Black Belt, or a Mega Stone. If unsure, call search_items with an empty query.", itemName)
				}
				item, err := svc.Pokemon.GetItemByName(itemName)
				if err != nil {
					return fmt.Sprintf("error: %q is not in Champions. Try one of: Choice Scarf, Focus Sash, Leftovers, Sitrus Berry, Lum Berry, Light Ball, Scope Lens, Mental Herb. Call search_items('') for the full list.", itemName)
				}
				if item.IsBanned {
					return "error: " + item.Name + " is banned"
				}
				t, err := svc.Team.GetTeam(teamID)
				if err != nil {
					return "error: " + err.Error()
				}
				for _, m := range t.Members {
					if m.Config != nil && m.Config.Item != nil && m.Config.Item.Slug == item.Slug && !strings.EqualFold(m.Config.Species.Name, pokeName) {
						return "error: item clause: " + item.Name + " already held by " + m.Config.Species.Name
					}
				}
				memberID, err := svc.Team.GetConfigByTeamAndSpecies(teamID, pokeName)
				if err != nil {
					return "error: " + err.Error()
				}
				if err := svc.Team.SetItem(memberID, item.Slug); err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Set %s's item to %s.", pokeName, item.Name)
			},
		},
		{
			Schema: schema("set_moves", "Set up to 4 moves for a Pokemon. All moves must be in the species' Champions learnset.",
				map[string]any{
					"team_id":      prop("integer", "Team ID"),
					"pokemon_name": prop("string", "Species name"),
					"move1":        prop("string", "First move name"),
					"move2":        prop("string", "Second move name"),
					"move3":        prop("string", "Third move name"),
					"move4":        prop("string", "Fourth move name"),
				},
				[]string{"team_id", "pokemon_name", "move1"}),
			Execute: func(args map[string]any) string {
				teamID := num(args, "team_id")
				pokeName := str(args, "pokemon_name")
				sp, err := svc.Pokemon.GetSpeciesByName(pokeName)
				if err != nil {
					return "error: " + err.Error()
				}
				var moveSlugs []string
				for _, key := range []string{"move1", "move2", "move3", "move4"} {
					name := str(args, key)
					if name == "" {
						continue
					}
					mv, err := svc.Pokemon.GetMoveByName(name)
					if err != nil {
						return fmt.Sprintf("error: %s. Verify spelling, or note that Champions has a curated move set that may exclude some moves from the broader Pokemon games.", err.Error())
					}
					ok, lerr := svc.Pokemon.CanLearnMove(sp.Slug, mv.Slug)
					if !ok {
						msg := name + " is not in " + pokeName + "'s Champions learnset. Do NOT call add_learnset_move — the data is fixed externally. Call get_moves to see what IS available and pick a legal alternative. If no legal alternative fits, return that fact to the parent agent."
						if lerr != nil {
							msg = lerr.Error()
						}
						return "error: " + msg
					}
					moveSlugs = append(moveSlugs, mv.Slug)
				}
				memberID, err := svc.Team.GetConfigByTeamAndSpecies(teamID, pokeName)
				if err != nil {
					return "error: " + err.Error()
				}
				if err := svc.Team.SetMoves(memberID, moveSlugs); err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Set %d move(s) for %s.", len(moveSlugs), pokeName)
			},
		},
		{
			Schema: schema("set_stats", "Set stat points for a Pokemon (66 total, max 32/stat).",
				map[string]any{
					"team_id":      prop("integer", "Team ID"),
					"pokemon_name": prop("string", "Species name"),
					"hp":           prop("integer", "HP points (0-32)"),
					"attack":       prop("integer", "Attack points (0-32)"),
					"defense":      prop("integer", "Defense points (0-32)"),
					"sp_attack":    prop("integer", "Sp. Atk points (0-32)"),
					"sp_defense":   prop("integer", "Sp. Def points (0-32)"),
					"speed":        prop("integer", "Speed points (0-32)"),
				},
				[]string{"team_id", "pokemon_name"}),
			Execute: func(args map[string]any) string {
				evs := team.StatSpread{
					HP:  num(args, "hp"),
					Atk: num(args, "attack"),
					Def: num(args, "defense"),
					SpA: num(args, "sp_attack"),
					SpD: num(args, "sp_defense"),
					Spe: num(args, "speed"),
				}
				for _, v := range []int{evs.HP, evs.Atk, evs.Def, evs.SpA, evs.SpD, evs.Spe} {
					if v < 0 || v > 32 {
						return "error: each stat must be 0–32"
					}
				}
				if t := evs.Total(); t > 66 {
					return fmt.Sprintf("error: total %d exceeds 66-point pool", t)
				}
				memberID, err := svc.Team.GetConfigByTeamAndSpecies(num(args, "team_id"), str(args, "pokemon_name"))
				if err != nil {
					return "error: " + err.Error()
				}
				if err := svc.Team.SetEVs(memberID, evs); err != nil {
					return "error: " + err.Error()
				}
				remaining := 66 - evs.Total()
				return fmt.Sprintf("Set EVs: HP %d / Atk %d / Def %d / SpA %d / SpD %d / Spe %d (total %d, %d remaining).",
					evs.HP, evs.Atk, evs.Def, evs.SpA, evs.SpD, evs.Spe, evs.Total(), remaining)
			},
		},
		{
			Schema: schema("set_notes", "Set free-text notes on a team member.",
				map[string]any{
					"team_id":      prop("integer", "Team ID"),
					"pokemon_name": prop("string", "Species name"),
					"notes":        prop("string", "Notes text"),
				},
				[]string{"team_id", "pokemon_name", "notes"}),
			Execute: func(args map[string]any) string {
				memberID, err := svc.Team.GetConfigByTeamAndSpecies(num(args, "team_id"), str(args, "pokemon_name"))
				if err != nil {
					return "error: " + err.Error()
				}
				if err := svc.Team.SetConfigNotes(memberID, str(args, "notes")); err != nil {
					return "error: " + err.Error()
				}
				return "Notes updated."
			},
		},
		{
			Schema: schema("set_team_notes", "Set strategy notes for a team (markdown supported).",
				map[string]any{
					"team_id": prop("integer", "Team ID"),
					"notes":   prop("string", "Markdown notes text"),
				},
				[]string{"team_id", "notes"}),
			Execute: func(args map[string]any) string {
				if err := svc.Team.UpdateTeamStrategy(num(args, "team_id"), str(args, "notes")); err != nil {
					return "error: " + err.Error()
				}
				return "Team notes updated."
			},
		},
		{
			Schema: schema("set_role", "Set a strategic role label for a team member.",
				map[string]any{
					"team_id":      prop("integer", "Team ID"),
					"pokemon_name": prop("string", "Species name"),
					"role":         prop("string", "Role label (e.g. 'lead', 'trick_room_setter')"),
				},
				[]string{"team_id", "pokemon_name", "role"}),
			Execute: func(args map[string]any) string {
				memberID, err := svc.Team.GetConfigByTeamAndSpecies(num(args, "team_id"), str(args, "pokemon_name"))
				if err != nil {
					return "error: " + err.Error()
				}
				if err := svc.Team.SetRole(memberID, str(args, "role")); err != nil {
					return "error: " + err.Error()
				}
				return "Role updated."
			},
		},
		{
			Schema: schema("set_nickname", "Set a nickname for a team member.",
				map[string]any{
					"team_id":      prop("integer", "Team ID"),
					"pokemon_name": prop("string", "Species name"),
					"nickname":     prop("string", "Nickname (empty string to clear)"),
				},
				[]string{"team_id", "pokemon_name", "nickname"}),
			Execute: func(args map[string]any) string {
				memberID, err := svc.Team.GetConfigByTeamAndSpecies(num(args, "team_id"), str(args, "pokemon_name"))
				if err != nil {
					return "error: " + err.Error()
				}
				if err := svc.Team.SetNickname(memberID, str(args, "nickname")); err != nil {
					return "error: " + err.Error()
				}
				return "Nickname updated."
			},
		},
		{
			Schema: schema("rename_team", "Rename a team.",
				map[string]any{
					"team_id": prop("integer", "Team ID"),
					"name":    prop("string", "New team name"),
				},
				[]string{"team_id", "name"}),
			Execute: func(args map[string]any) string {
				if err := svc.Team.RenameTeam(num(args, "team_id"), str(args, "name")); err != nil {
					return "error: " + err.Error()
				}
				return "Team renamed to " + str(args, "name") + "."
			},
		},
		{
			Schema: schema("swap_slots", "Swap two Pokemon slots within a team.",
				map[string]any{
					"team_id": prop("integer", "Team ID"),
					"slot_a":  prop("integer", "First slot (1-6)"),
					"slot_b":  prop("integer", "Second slot (1-6)"),
				},
				[]string{"team_id", "slot_a", "slot_b"}),
			Execute: func(args map[string]any) string {
				if err := svc.Team.SwapSlots(num(args, "team_id"), num(args, "slot_a"), num(args, "slot_b")); err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Swapped slots %d and %d.", num(args, "slot_a"), num(args, "slot_b"))
			},
		},
		{
			Schema: schema("replace_pokemon", "Replace the species at a slot in one atomic call. Clears the prior config (none of ability/moves/items transfer cleanly).",
				map[string]any{
					"team_id":          prop("integer", "Team ID"),
					"slot":             prop("integer", "Slot 1-6 to replace"),
					"new_pokemon_name": prop("string", "Species name to put at this slot"),
					"ability":          prop("string", "Ability name — uses first available if omitted"),
				},
				[]string{"team_id", "slot", "new_pokemon_name"}),
			Execute: func(args map[string]any) string {
				teamID := num(args, "team_id")
				slot := num(args, "slot")
				if slot < 1 || slot > 6 {
					return "error: slot must be 1-6"
				}
				name := str(args, "new_pokemon_name")
				sp, err := svc.Pokemon.GetSpeciesByName(name)
				if err != nil {
					return "error: " + err.Error()
				}
				abilities, err := svc.Pokemon.GetAbilitiesForSpecies(sp.Slug)
				if err != nil || len(abilities) == 0 {
					return "error: " + sp.Name + " has no abilities in the database. Use add_species_ability to add its abilities first."
				}
				abilitySlug := abilities[0].Slug
				if abName := str(args, "ability"); abName != "" {
					ab, err := svc.Pokemon.GetAbilityByName(abName)
					if err != nil {
						return "error: " + err.Error()
					}
					if ok, _ := svc.Pokemon.HasAbility(sp.Slug, ab.Slug); !ok {
						return "error: " + abName + " is not a valid ability for " + sp.Name
					}
					abilitySlug = ab.Slug
				}
				newID, err := svc.Team.ReplaceMember(teamID, slot, sp.Slug, abilitySlug)
				if err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Replaced slot %d with %s (config %d). Prior config cleared — re-set nature/item/moves/stats as needed.", slot, sp.Name, newID)
			},
		},
		{
			Schema: schema("replace_move", "Replace ONE move at a specific 1-4 index, preserving the other three. Validates against the species' Champions learnset.",
				map[string]any{
					"team_id":       prop("integer", "Team ID"),
					"pokemon_name":  prop("string", "Species name on the team"),
					"slot":          prop("integer", "Move slot 1-4 to replace"),
					"new_move_name": prop("string", "New move name"),
				},
				[]string{"team_id", "pokemon_name", "slot", "new_move_name"}),
			Execute: func(args map[string]any) string {
				teamID := num(args, "team_id")
				pokeName := str(args, "pokemon_name")
				moveSlot := num(args, "slot")
				if moveSlot < 1 || moveSlot > 4 {
					return "error: move slot must be 1-4"
				}
				sp, err := svc.Pokemon.GetSpeciesByName(pokeName)
				if err != nil {
					return "error: " + err.Error()
				}
				newMv, err := svc.Pokemon.GetMoveByName(str(args, "new_move_name"))
				if err != nil {
					return "error: " + err.Error()
				}
				if ok, lerr := svc.Pokemon.CanLearnMove(sp.Slug, newMv.Slug); !ok {
					if lerr != nil {
						return "error: " + lerr.Error()
					}
					return "error: " + newMv.Name + " is not in " + sp.Name + "'s Champions learnset. Call get_moves to see what IS available."
				}
				configID, err := svc.Team.GetConfigByTeamAndSpecies(teamID, pokeName)
				if err != nil {
					return "error: " + err.Error()
				}
				cfg, err := svc.Team.GetConfig(configID)
				if err != nil {
					return "error: " + err.Error()
				}
				// Build the new 4-slot move list, preserving the others. Pad to
				// moveSlot length so a sparse build still gets the requested slot.
				slugs := make([]string, 0, 4)
				for i := 0; i < moveSlot-1; i++ {
					if i < len(cfg.Moves) && cfg.Moves[i] != nil {
						slugs = append(slugs, cfg.Moves[i].Slug)
					} else {
						return fmt.Sprintf("error: cannot set slot %d — slot %d is empty. Use set_moves to populate the build first.", moveSlot, i+1)
					}
				}
				slugs = append(slugs, newMv.Slug)
				for i := moveSlot; i < len(cfg.Moves); i++ {
					if cfg.Moves[i] != nil {
						slugs = append(slugs, cfg.Moves[i].Slug)
					}
				}
				if err := svc.Team.SetMoves(configID, slugs); err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Replaced %s's move slot %d with %s.", sp.Name, moveSlot, newMv.Name)
			},
		},
		{
			Schema: schema("list_teams", "List all teams in the database.",
				map[string]any{
					"limit":  prop("integer", "Max results in the page (default 10)"),
					"offset": prop("integer", "Pagination offset (default 0)"),
				},
				nil),
			Execute: func(args map[string]any) string {
				teams, err := ListTeams(svc)
				if err != nil {
					return "error: " + err.Error()
				}
				if len(teams) == 0 {
					return "No teams found."
				}
				rows := make([]map[string]any, 0, len(teams))
				for _, t := range teams {
					rows = append(rows, map[string]any{
						"id":         t.ID,
						"name":       t.Name,
						"regulation": t.Regulation,
						"members":    t.MemberCount,
					})
				}
				return toonMenu("teams", rows, len(teams),
					num(args, "offset"), num(args, "limit"),
					"call get_team team_id=N for the full roster. (No filters on this list — it's intentionally short.)")
			},
		},
		{
			Schema: schema("get_team", "Get full details of a team including all members, moves, items, and stats.",
				map[string]any{"team_id": prop("integer", "Team ID")},
				[]string{"team_id"}),
			Execute: func(args map[string]any) string {
				t, err := GetTeam(svc, num(args, "team_id"))
				if err != nil {
					return "error: " + err.Error()
				}
				// Narrative header (single record), then a TOON tabular member
				// list (each member is a record). Empty/missing fields render
				// as "-" so the column type stays uniform across rows.
				var b strings.Builder
				fmt.Fprintf(&b, "Team %d: %s (Regulation %s)\n", t.ID, t.Name, t.Regulation)
				if len(t.Members) == 0 {
					b.WriteString("(no members)\n")
					return b.String()
				}
				rows := make([]map[string]any, 0, len(t.Members))
				for _, m := range t.Members {
					if m.Config == nil || m.Config.Species == nil {
						continue
					}
					item := "-"
					if m.Config.Item != nil {
						item = m.Config.Item.Name
					}
					ability := "-"
					if m.Config.Ability != nil {
						ability = m.Config.Ability.Name
					}
					moves := []string{}
					for _, mv := range m.Config.Moves {
						if mv != nil {
							moves = append(moves, mv.Name)
						}
					}
					movesStr := "-"
					if len(moves) > 0 {
						movesStr = strings.Join(moves, "/")
					}
					ev := m.Config.EVs
					rows = append(rows, map[string]any{
						"slot":    m.Slot,
						"species": m.Config.Species.Name,
						"ability": ability,
						"nature":  m.Config.Nature,
						"item":    item,
						"moves":   movesStr,
						"sp":      fmt.Sprintf("%d/%d/%d/%d/%d/%d", ev.HP, ev.Atk, ev.Def, ev.SpA, ev.SpD, ev.Spe),
					})
				}
				encoded, err := toonRows("members", rows)
				if err != nil {
					return "error: TOON encode: " + err.Error()
				}
				b.WriteString(encoded)
				return b.String()
			},
		},
		{
			Schema: schema("find_pokemon_by_name",
				"Look up owned Pokemon by name. Use when you already know the name. For discovery use find_pokemon_by_filters.",
				map[string]any{
					"name":   prop("string", "Species name or partial name (fuzzy match)"),
					"owned":  prop("boolean", "Default true; set false to include unowned"),
					"limit":  prop("integer", "Max results in the page (default 10)"),
					"offset": prop("integer", "Pagination offset (default 0)"),
				},
				[]string{"name"}),
			Execute: func(args map[string]any) string {
				var owned *bool
				if v, ok := args["owned"]; ok && v != nil {
					b := v.(bool)
					owned = &b
				}
				// Pull more than one page so the model can ask for the next page
				// without re-querying. 50 is plenty for fuzzy name matches.
				results, err := FindPokemonByName(svc, str(args, "name"), owned, 50)
				if err != nil {
					return "error: " + err.Error()
				}
				if len(results) == 0 {
					return "No owned Pokemon found with that name. If you're looking for candidates to fill a role, use find_pokemon_by_filters instead."
				}
				return toonMenu("pokemon", speciesRows(results), len(results),
					num(args, "offset"), num(args, "limit"),
					"refine the call to narrow this list — e.g. find_pokemon_by_name name='Hatt' owned=true. Or call get_pokemon name='X' for full stats.")
			},
		},
		{
			Schema: schema("find_pokemon_by_filters",
				"Discover owned Pokemon candidates by type, role, and speed tier. Use for coverage gaps — never guess names.",
				map[string]any{
					"type":       prop("string", "Filter by type (e.g. 'fire', 'steel')"),
					"role":       prop("string", "'physical attacker'|'special attacker'|'mixed attacker'|'support'|'tank'"),
					"speed_tier": prop("string", "'fast' (>100 base)|'mid' (70-100)|'slow' (<70)"),
					"legendary":  prop("boolean", "Filter by legendary status"),
					"owned":      prop("boolean", "Default true; set false to include unowned"),
					"limit":      prop("integer", "Max results in the page (default 10)"),
					"offset":     prop("integer", "Pagination offset (default 0)"),
				},
				[]string{}),
			Execute: func(args map[string]any) string {
				f := pokemon.SpeciesFilter{
					Type:      str(args, "type"),
					Role:      str(args, "role"),
					SpeedTier: str(args, "speed_tier"),
				}
				if v, ok := args["owned"]; ok && v != nil {
					b := v.(bool)
					f.Owned = &b
				}
				if v, ok := args["legendary"]; ok && v != nil {
					b := v.(bool)
					f.Legendary = &b
				}
				results, err := FindPokemonByFilters(svc, f, 100)
				if err != nil {
					return "error: " + err.Error()
				}
				if len(results) == 0 {
					return "No owned Pokemon match those filters. Try broadening: remove role or speed_tier constraints."
				}
				return toonMenu("pokemon", speciesRows(results), len(results),
					num(args, "offset"), num(args, "limit"),
					"refine the call to narrow this list — e.g. find_pokemon_by_filters owned=true type='ground' role='tank' speed_tier='slow'. Or call evaluate_pokemon name='X' to score one candidate.")
			},
		},
		{
			Schema: schema("analyse_team", "Analyse a team for speed tiers, type coverage, and weaknesses.",
				map[string]any{"team_id": prop("integer", "Team ID")},
				[]string{"team_id"}),
			Execute: func(args map[string]any) string {
				t, a, err := AnalyseTeam(svc, num(args, "team_id"))
				if err != nil {
					return "error: " + err.Error()
				}
				var b strings.Builder
				fmt.Fprintf(&b, "Team: %s\n\nSpeed tiers:\n", t.Name)
				for _, st := range a.SpeedTiers {
					fmt.Fprintf(&b, "  Slot %d %s: %d\n", st.Slot, st.Name, st.StatSpeed)
				}
				if len(a.DefensiveWeaknesses) > 0 {
					fmt.Fprintf(&b, "\nDefensive weaknesses (≥2 mons weak):\n")
					for _, w := range a.DefensiveWeaknesses {
						fmt.Fprintf(&b, "  %s: %d mons\n", w.Type, w.Count)
					}
				}
				if len(a.OffensiveCoverage) > 0 {
					fmt.Fprintf(&b, "\nOffensive coverage: %s\n", strings.Join(a.OffensiveCoverage, ", "))
				}
				fmt.Fprintf(&b, "\nArchetypes: %s\n", strings.Join(a.Archetypes, ", "))
				return b.String()
			},
		},
		{
			Schema: schema("validate_team", "Check a team for VGC rule violations.",
				map[string]any{"team_id": prop("integer", "Team ID")},
				[]string{"team_id"}),
			Execute: func(args map[string]any) string {
				violations, err := ValidateTeam(svc, num(args, "team_id"))
				if err != nil {
					return "error: " + err.Error()
				}
				if len(violations) == 0 {
					return "Team is legal — no violations."
				}
				var b strings.Builder
				for _, v := range violations {
					fmt.Fprintf(&b, "  [%s] %s\n", v.Rule, v.Message)
				}
				return b.String()
			},
		},
		{
			Schema: schema("search_knowledge", "Search the VGC strategy knowledge base.",
				map[string]any{"query": prop("string", "Search query")},
				[]string{"query"}),
			Execute: func(args map[string]any) string {
				results, err := SearchKnowledge(svc, str(args, "query"), 3)
				if err != nil {
					return "error: " + err.Error()
				}
				if len(results) == 0 {
					return "No knowledge base results."
				}
				var b strings.Builder
				for _, r := range results {
					fmt.Fprintf(&b, "--- %s ---\n%s\n\n", r.Source, r.Content)
				}
				return b.String()
			},
		},
		{
			Schema: schema("calc_stats", "Calculate final Level 50 stats for a Pokemon given its base stats, stat points, and nature.",
				map[string]any{
					"species":    prop("string", "Species name"),
					"hp":         prop("integer", "HP stat points (0-32)"),
					"attack":     prop("integer", "Attack stat points (0-32)"),
					"defense":    prop("integer", "Defense stat points (0-32)"),
					"sp_attack":  prop("integer", "Sp. Atk stat points (0-32)"),
					"sp_defense": prop("integer", "Sp. Def stat points (0-32)"),
					"speed":      prop("integer", "Speed stat points (0-32)"),
					"nature":     prop("string", "Nature name (e.g. Modest, Jolly)"),
				},
				[]string{"species"}),
			Execute: func(args map[string]any) string {
				spread := team.StatSpread{
					HP:  num(args, "hp"),
					Atk: num(args, "attack"),
					Def: num(args, "defense"),
					SpA: num(args, "sp_attack"),
					SpD: num(args, "sp_defense"),
					Spe: num(args, "speed"),
				}
				result, err := CalcStats(svc, str(args, "species"), spread, str(args, "nature"))
				if err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("%s @ %s (SP: %d/66)\n"+
					"  HP:      %3d (base %d, SP %d)\n"+
					"  Attack:  %3d (base %d, SP %d)\n"+
					"  Defense: %3d (base %d, SP %d)\n"+
					"  Sp. Atk: %3d (base %d, SP %d)\n"+
					"  Sp. Def: %3d (base %d, SP %d)\n"+
					"  Speed:   %3d (base %d, SP %d)\n",
					result.SpeciesName, result.Nature, result.TotalSP,
					result.Rows[0].Final, result.Rows[0].Base, result.Rows[0].SP,
					result.Rows[1].Final, result.Rows[1].Base, result.Rows[1].SP,
					result.Rows[2].Final, result.Rows[2].Base, result.Rows[2].SP,
					result.Rows[3].Final, result.Rows[3].Base, result.Rows[3].SP,
					result.Rows[4].Final, result.Rows[4].Base, result.Rows[4].SP,
					result.Rows[5].Final, result.Rows[5].Base, result.Rows[5].SP,
				)
			},
		},
		{
			Schema: schema("set_owned",
				"Mark a Pokemon or item as owned (or unowned).",
				map[string]any{
					"type":  prop("string", "'pokemon' or 'item'"),
					"name":  prop("string", "Species or item name"),
					"owned": prop("boolean", "true to mark owned, false to unmark"),
				},
				[]string{"type", "name", "owned"}),
			Execute: func(args map[string]any) string {
				owned, _ := args["owned"].(bool)
				msg, err := SetOwned(svc, str(args, "type"), str(args, "name"), owned)
				if err != nil {
					return "error: " + err.Error()
				}
				return msg
			},
		},
		{
			Schema: schema("evaluate_pokemon",
				"Evaluate a Pokemon: type chart, offensive coverage, stat role, bulk, speed tier. Pass team_id for move/EV analysis.",
				map[string]any{
					"pokemon_name": prop("string", "Species name (e.g. 'Garchomp')"),
					"team_id":      prop("integer", "Team ID — enables member-level move/EV analysis"),
				},
				[]string{"pokemon_name"}),
			Execute: func(args map[string]any) string {
				var teamID int
				switch v := args["team_id"].(type) {
				case float64:
					teamID = int(v)
				case int:
					teamID = v
				}
				result, err := EvaluatePokemon(svc, str(args, "pokemon_name"), teamID)
				if err != nil {
					return "error: " + err.Error()
				}
				switch result.Kind {
				case EvalMember:
					return team.FormatMemberEval(result.MemberEval)
				case EvalNotOnTeam:
					return fmt.Sprintf("%s is not on team %d; showing species-level evaluation.", result.SpeciesName, result.TeamID)
				default:
					return team.FormatSpeciesEval(result.SpeciesEval)
				}
			},
		},
		{
			Schema: schema("get_help", "Returns a compact usage guide for all ptm tools.", nil, nil),
			Execute: func(args map[string]any) string {
				return PtmSystemPrompt
			},
		},
		{
			Schema: schema("get_pokemon", "Get full details for a Pokemon species including base stats, types, and available abilities.",
				map[string]any{"name": prop("string", "Pokemon name (e.g. 'Garchomp')")},
				[]string{"name"}),
			Execute: func(args map[string]any) string {
				sp, err := svc.Pokemon.GetSpeciesByName(str(args, "name"))
				if err != nil {
					return "error: " + err.Error()
				}
				abilities, _ := svc.Pokemon.GetAbilitiesForSpecies(sp.Slug)

				typ := string(sp.Type1)
				if sp.Type2 != "" {
					typ += "/" + string(sp.Type2)
				}
				flags := []string{}
				if sp.IsFinalEvo {
					flags = append(flags, "final evo")
				}
				if sp.IsLegendary {
					flags = append(flags, "legendary")
				}
				if sp.IsMythical {
					flags = append(flags, "mythical")
				}
				if sp.IsRestricted {
					flags = append(flags, "restricted")
				}
				flagStr := ""
				if len(flags) > 0 {
					flagStr = " (" + strings.Join(flags, ", ") + ")"
				}

				abilNames := make([]string, 0, len(abilities))
				for _, a := range abilities {
					if a.Description != "" {
						abilNames = append(abilNames, fmt.Sprintf("%s — %s", a.Name, a.Description))
					} else {
						abilNames = append(abilNames, a.Name)
					}
				}
				abilLine := "Abilities: (none)"
				if len(abilNames) > 0 {
					abilLine = "Abilities: " + strings.Join(abilNames, " | ")
				}

				owned := "no"
				if sp.Owned {
					owned = "yes"
				}

				return fmt.Sprintf(
					"%s #%d — %s%s\nBST %d — HP %d / Atk %d / Def %d / SpA %d / SpD %d / Spe %d\n%s\nOwned: %s",
					sp.Name, sp.DexID, typ, flagStr,
					sp.BST(), sp.HP, sp.Attack, sp.Defense, sp.SpAttack, sp.SpDefense, sp.Speed,
					abilLine, owned,
				)
			},
		},
		{
			Schema: schema("get_moves", "Get the Champions learnset for a species. Paginated.",
				map[string]any{
					"pokemon_name": prop("string", "Pokemon name"),
					"limit":        prop("integer", "Max results in the page (default 10)"),
					"offset":       prop("integer", "Pagination offset (default 0)"),
				},
				[]string{"pokemon_name"}),
			Execute: func(args map[string]any) string {
				sp, err := svc.Pokemon.GetSpeciesByName(str(args, "pokemon_name"))
				if err != nil {
					return "error: " + err.Error()
				}
				moves, err := svc.Pokemon.GetChampionsLearnset(sp.Slug)
				if err != nil {
					return "error: " + err.Error()
				}
				if len(moves) == 0 {
					return fmt.Sprintf("No Champions learnset data seeded for %s. The data is curated externally — return that fact to the caller (or to the user) and stop.", sp.Name)
				}
				return toonMenu("moves", moveRows(moves, 60), len(moves),
					num(args, "offset"), num(args, "limit"),
					"to narrow, use search_moves with type/category/min_power filters (e.g. type='psychic' category='status').")
			},
		},
		{
			Schema: schema("search_moves", "Search moves by name/description with optional filters.",
				map[string]any{
					"query":        prop("string", "Name or description fragment"),
					"type":         prop("string", "Filter by type"),
					"category":     prop("string", "physical, special, or status"),
					"min_power":    prop("integer", "Minimum base power"),
					"max_power":    prop("integer", "Maximum base power"),
					"min_accuracy": prop("integer", "Minimum accuracy"),
					"priority":     prop("integer", "Exact priority value"),
					"limit":        prop("integer", "Max results in the page (default 10)"),
					"offset":       prop("integer", "Pagination offset (default 0)"),
				},
				[]string{"query"}),
			Execute: func(args map[string]any) string {
				f := pokemon.MoveFilter{
					Type:        str(args, "type"),
					Category:    str(args, "category"),
					MinPower:    num(args, "min_power"),
					MaxPower:    num(args, "max_power"),
					MinAccuracy: num(args, "min_accuracy"),
				}
				if v, ok := args["priority"]; ok && v != nil {
					p := num(args, "priority")
					f.Priority = &p
				}
				moves, err := svc.Pokemon.SearchMoves(str(args, "query"), 100, f)
				if err != nil {
					return "error: " + err.Error()
				}
				if len(moves) == 0 {
					return fmt.Sprintf("No moves matched %q.", str(args, "query"))
				}
				return toonMenu("moves", moveRows(moves, 60), len(moves),
					num(args, "offset"), num(args, "limit"),
					"refine the call to narrow this list — e.g. search_moves query='wisp' type='fire' category='status' min_power=0.")
			},
		},
		{
			Schema: schema("get_item", "Get details for a held item by name.",
				map[string]any{"name": prop("string", "Item name (e.g. 'Choice Specs')")},
				[]string{"name"}),
			Execute: func(args map[string]any) string {
				item, err := svc.Pokemon.GetItemByName(str(args, "name"))
				if err != nil {
					return "error: " + err.Error()
				}
				return formatItem(item)
			},
		},
		{
			Schema: schema("search_items", "Search held items by name or effect description.",
				map[string]any{
					"query":  prop("string", "Search term (empty to list all)"),
					"owned":  prop("boolean", "Filter to owned/unowned items"),
					"banned": prop("boolean", "Filter to banned/legal items"),
					"limit":  prop("integer", "Max results in the page (default 10)"),
					"offset": prop("integer", "Pagination offset (default 0)"),
				},
				[]string{"query"}),
			Execute: func(args map[string]any) string {
				f := pokemon.ItemFilter{}
				if v, ok := args["owned"]; ok && v != nil {
					b := v.(bool)
					f.Owned = &b
				}
				if v, ok := args["banned"]; ok && v != nil {
					b := v.(bool)
					f.Banned = &b
				}
				items, err := svc.Pokemon.SearchItems(str(args, "query"), 100, f)
				if err != nil {
					return "error: " + err.Error()
				}
				if len(items) == 0 {
					return fmt.Sprintf("No items matched %q. Champions has a curated held-item set — try a broader query or empty string to list all.", str(args, "query"))
				}
				return toonMenu("items", itemRows(items, 80), len(items),
					num(args, "offset"), num(args, "limit"),
					"refine the call to narrow this list — e.g. search_items query='berry' owned=true. Or call get_item name='X' for full effect description.")
			},
		},
		{
			Schema: schema("export_team", "Export a team as a formatted markdown document.",
				map[string]any{"team_id": prop("integer", "Team ID")},
				[]string{"team_id"}),
			Execute: func(args map[string]any) string {
				t, err := svc.Team.GetTeam(num(args, "team_id"))
				if err != nil {
					return "error: " + err.Error()
				}
				return team.ExportMarkdown(t)
			},
		},
		// delete_team is intentionally NOT exposed to agents. Destructive
		// operations stay user-only — the user removes teams via the /teams
		// page in the web UI. See sable's system prompt for the user-facing
		// handoff Sable gives when asked to delete.
		{
			Schema: schema("list_regulations", "List all VGC regulation sets.",
				map[string]any{
					"limit":  prop("integer", "Max results in the page (default 10)"),
					"offset": prop("integer", "Pagination offset (default 0)"),
				},
				nil),
			Execute: func(args map[string]any) string {
				regs, err := svc.Team.ListRegulations()
				if err != nil {
					return "error: " + err.Error()
				}
				if len(regs) == 0 {
					return "No regulations defined."
				}
				rows := make([]map[string]any, 0, len(regs))
				for _, r := range regs {
					rows = append(rows, map[string]any{
						"id":             r.ID,
						"name":           r.Name,
						"max_restricted": r.MaxRestricted,
						"active":         r.Active,
					})
				}
				return toonMenu("regulations", rows, len(regs),
					num(args, "offset"), num(args, "limit"),
					"call get_regulation id='X' for full description and ban lists.")
			},
		},
		{
			Schema: schema("get_regulation", "Get details for a VGC regulation set including banned and restricted species.",
				map[string]any{"id": prop("string", "Regulation ID (e.g. 'H')")},
				[]string{"id"}),
			Execute: func(args map[string]any) string {
				id := strings.ToUpper(str(args, "id"))
				r, err := svc.Team.GetRegulation(id)
				if err != nil {
					return "error: " + err.Error()
				}
				var b strings.Builder
				fmt.Fprintf(&b, "Regulation %s — %s\n", r.ID, r.Name)
				if r.Description != "" {
					fmt.Fprintf(&b, "%s\n", r.Description)
				}
				fmt.Fprintf(&b, "Restricted Legendary cap: %d\n", r.MaxRestricted)
				if len(r.RestrictedSlugs) > 0 {
					fmt.Fprintf(&b, "Restricted species (%d): %s\n", len(r.RestrictedSlugs), strings.Join(r.RestrictedSlugs, ", "))
				}
				if len(r.BannedSlugs) > 0 {
					fmt.Fprintf(&b, "Banned species (%d): %s\n", len(r.BannedSlugs), strings.Join(r.BannedSlugs, ", "))
				}
				if len(r.BannedMoveSlugs) > 0 {
					fmt.Fprintf(&b, "Banned moves (%d): %s\n", len(r.BannedMoveSlugs), strings.Join(r.BannedMoveSlugs, ", "))
				}
				return b.String()
			},
		},
		{
			Schema: schema("training_cost", "Calculate VP cost to build a team from scratch.",
				map[string]any{"team_id": prop("integer", "Team ID")},
				[]string{"team_id"}),
			Execute: func(args map[string]any) string {
				t, err := svc.Team.GetTeam(num(args, "team_id"))
				if err != nil {
					return "error: " + err.Error()
				}
				if len(t.Members) == 0 {
					return "Team has no members."
				}
				var b strings.Builder
				total := 0
				for _, m := range t.Members {
					if m.Config == nil || m.Config.Species == nil {
						continue
					}
					c := m.Config
					spVP := (c.EVs.HP + c.EVs.Atk + c.EVs.Def + c.EVs.SpA + c.EVs.SpD + c.EVs.Spe) * 5
					natureVP := 0
					if c.Nature != "" && !strings.EqualFold(c.Nature, "Serious") {
						natureVP = 500
					}
					moveVP := len(c.Moves) * 250
					abilityVP := 0
					if c.Ability != nil {
						if abilities, err := svc.Pokemon.GetAbilitiesForSpecies(c.Species.Slug); err == nil && len(abilities) > 0 && abilities[0].Slug != c.Ability.Slug {
							abilityVP = 500
						}
					}
					memberTotal := 800 + spVP + natureVP + moveVP + abilityVP
					total += memberTotal
					fmt.Fprintf(&b, "%s: %d VP (recruit 800 + SP %d + nature %d + moves %d + ability %d)\n", c.Species.Name, memberTotal, spVP, natureVP, moveVP, abilityVP)
				}
				fmt.Fprintf(&b, "Total: %d VP", total)
				return b.String()
			},
		},
		{
			Schema: schema("ingest_document", "Add a document to the knowledge base.",
				map[string]any{
					"title":   prop("string", "Document title"),
					"source":  prop("string", "Source URL or file path"),
					"content": prop("string", "Full document content (markdown supported)"),
				},
				[]string{"title", "content"}),
			Execute: func(args map[string]any) string {
				if err := svc.Knowledge.Ingest(str(args, "title"), str(args, "source"), str(args, "content"), 1500); err != nil {
					return "error: " + err.Error()
				}
				return "Document ingested successfully."
			},
		},
		{
			Schema: schema("review_document", "Reset the TTL on a knowledge base document, marking it reviewed for another 6 months.",
				map[string]any{"title": prop("string", "Document title or path")},
				[]string{"title"}),
			Execute: func(args map[string]any) string {
				n, err := svc.Knowledge.Touch(str(args, "title"))
				if err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("TTL reset on %d chunks — valid for 6 more months.", n)
			},
		},
		{
			Schema: schema("add_team_log",
				"Append a combat/session log entry to a team.",
				map[string]any{
					"team_id": prop("integer", "Team ID"),
					"entry":   prop("string", "Log entry text (markdown supported)"),
				},
				[]string{"team_id", "entry"}),
			Execute: func(args map[string]any) string {
				id, err := AddTeamLog(svc, num(args, "team_id"), str(args, "entry"))
				if err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Log entry %d recorded.", id)
			},
		},
		{
			Schema: schema("get_team_logs",
				"Retrieve combat/session log entries for a team, newest first. Paginated.",
				map[string]any{
					"team_id": prop("integer", "Team ID"),
					"limit":   prop("integer", "Max results in the page (default 10)"),
					"offset":  prop("integer", "Pagination offset (default 0)"),
				},
				[]string{"team_id"}),
			Execute: func(args map[string]any) string {
				logs, err := GetTeamLogs(svc, num(args, "team_id"))
				if err != nil {
					return "error: " + err.Error()
				}
				if len(logs) == 0 {
					return "No log entries yet."
				}
				rows := make([]map[string]any, 0, len(logs))
				for _, l := range logs {
					entry := l.Entry
					if len(entry) > 200 {
						entry = entry[:199] + "…"
					}
					rows = append(rows, map[string]any{
						"id":      l.ID,
						"created": l.CreatedAt.Format("2006-01-02 15:04"),
						"entry":   entry,
					})
				}
				return toonMenu("logs", rows, len(logs),
					num(args, "offset"), num(args, "limit"),
					"increase limit to see more recent entries inline.")
			},
		},
		{
			Schema: schema("get_team_history",
				"List historical snapshots of a team (every successful mutation writes one). Newest first. Paginated.",
				map[string]any{
					"team_id": prop("integer", "Team ID"),
					"limit":   prop("integer", "Max results in the page (default 10)"),
					"offset":  prop("integer", "Pagination offset (default 0)"),
				},
				[]string{"team_id"}),
			Execute: func(args map[string]any) string {
				teamID := num(args, "team_id")
				total, err := svc.Team.CountTeamSnapshots(teamID)
				if err != nil {
					return "error: " + err.Error()
				}
				if total == 0 {
					return "No history yet for this team."
				}
				// Pull a wider window so toonMenu can paginate within it.
				snaps, err := svc.Team.ListTeamSnapshots(teamID, 100, 0)
				if err != nil {
					return "error: " + err.Error()
				}
				rows := make([]map[string]any, 0, len(snaps))
				for _, s := range snaps {
					rows = append(rows, map[string]any{
						"id":      s.ID,
						"kind":    s.Kind,
						"label":   s.Label,
						"when":    s.CreatedAt.Format("2006-01-02 15:04:05"),
						"by":      s.CreatedBy,
					})
				}
				return toonMenu("snapshots", rows, len(rows),
					num(args, "offset"), num(args, "limit"),
					"call get_team_snapshot snapshot_id=N to view a past version of the team.")
			},
		},
		{
			Schema: schema("get_team_snapshot",
				"Render a historical snapshot of a team — narrative header + TOON members of that past state.",
				map[string]any{"snapshot_id": prop("integer", "Snapshot ID from get_team_history")},
				[]string{"snapshot_id"}),
			Execute: func(args map[string]any) string {
				snapID := int64(num(args, "snapshot_id"))
				snap, err := svc.Team.GetTeamSnapshot(snapID)
				if err != nil {
					return "error: " + err.Error()
				}
				// Decode the JSON payload back into a Team for rendering.
				var t team.Team
				if err := json.Unmarshal([]byte(snap.Payload), &t); err != nil {
					return "error: snapshot payload corrupted: " + err.Error()
				}
				var b strings.Builder
				fmt.Fprintf(&b, "Snapshot %d (%s, %s, by %s) — Team %d: %s (Regulation %s)\n",
					snap.ID, snap.Kind, snap.CreatedAt.Format("2006-01-02 15:04:05"),
					snap.CreatedBy, t.ID, t.Name, t.Regulation)
				if snap.Label != "" {
					fmt.Fprintf(&b, "Triggered by: %s\n", snap.Label)
				}
				if len(t.Members) == 0 {
					b.WriteString("(no members at that time)\n")
					return b.String()
				}
				rows := make([]map[string]any, 0, len(t.Members))
				for _, m := range t.Members {
					if m.Config == nil || m.Config.Species == nil {
						continue
					}
					item := "-"
					if m.Config.Item != nil {
						item = m.Config.Item.Name
					}
					ability := "-"
					if m.Config.Ability != nil {
						ability = m.Config.Ability.Name
					}
					moves := []string{}
					for _, mv := range m.Config.Moves {
						if mv != nil {
							moves = append(moves, mv.Name)
						}
					}
					movesStr := "-"
					if len(moves) > 0 {
						movesStr = strings.Join(moves, "/")
					}
					ev := m.Config.EVs
					rows = append(rows, map[string]any{
						"slot":    m.Slot,
						"species": m.Config.Species.Name,
						"ability": ability,
						"nature":  m.Config.Nature,
						"item":    item,
						"moves":   movesStr,
						"sp":      fmt.Sprintf("%d/%d/%d/%d/%d/%d", ev.HP, ev.Atk, ev.Def, ev.SpA, ev.SpD, ev.Spe),
					})
				}
				encoded, err := toonRows("members", rows)
				if err != nil {
					return "error: TOON encode: " + err.Error()
				}
				b.WriteString(encoded)
				return b.String()
			},
		},
	}
}
