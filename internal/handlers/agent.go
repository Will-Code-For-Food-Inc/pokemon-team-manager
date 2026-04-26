package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

// ── Ollama wire types ─────────────────────────────────────────────────────────

// OllamaMessage is a single message in an Ollama chat exchange.
type OllamaMessage struct {
	Role      string            `json:"role"`
	Content   string            `json:"content,omitempty"`
	ToolCalls []OllamaToolCall  `json:"tool_calls,omitempty"`
}

// OllamaToolCall is a single tool invocation returned by the model.
type OllamaToolCall struct {
	Function OllamaToolCallFn `json:"function"`
}

// OllamaToolCallFn holds the name and arguments of a tool call.
type OllamaToolCallFn struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// OlamaTool describes a tool for the Ollama API.
type OlamaTool struct {
	Type     string      `json:"type"`
	Function OlamaToolFn `json:"function"`
}

// OlamaToolFn is the function descriptor within an OlamaTool.
type OlamaToolFn struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// AgentTool pairs an OlamaTool schema with its execute function.
type AgentTool struct {
	Schema  OlamaTool
	Execute func(args map[string]any) string
}

// ChatMessage is a display message produced during an agent turn.
type ChatMessage struct {
	Role    string `json:"role"`    // "user" | "assistant" | "tool" | "error"
	Content string `json:"content"`
}

// AgentView carries UI panel state surfaced during an agent turn.
type AgentView struct {
	Type        string `json:"type"`
	TeamID      int    `json:"team_id,omitempty"`
	PokemonName string `json:"pokemon_name,omitempty"`
}

// ── Ollama client ─────────────────────────────────────────────────────────────

// OllamaClient sends chat requests to an Ollama server.
type OllamaClient struct {
	baseURL string
	model   string
	cfg     AgentConfig
	http    *http.Client
}

// NewOllamaClient creates an OllamaClient from an AgentConfig.
func NewOllamaClient(cfg AgentConfig) *OllamaClient {
	return &OllamaClient{
		baseURL: cfg.OllamaURL,
		model:   cfg.Model,
		cfg:     cfg,
		http:    &http.Client{Timeout: 120 * time.Second},
	}
}

// Chat sends a chat turn to Ollama and returns the model's reply.
func (c *OllamaClient) Chat(messages []OllamaMessage, tools []OlamaTool) (OllamaMessage, error) {
	body, _ := json.Marshal(map[string]any{
		"model":      c.model,
		"messages":   messages,
		"tools":      tools,
		"stream":     false,
		"think":      false,
		"keep_alive": c.cfg.KeepAlive,
		"options": map[string]any{
			"num_ctx":        c.cfg.NumCtx,
			"temperature":    c.cfg.Temperature,
			"top_p":          c.cfg.TopP,
			"top_k":          c.cfg.TopK,
			"num_predict":    1024,
			"repeat_penalty": c.cfg.Repeat,
		},
	})
	resp, err := c.http.Post(c.baseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return OllamaMessage{}, fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		Message OllamaMessage `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return OllamaMessage{}, fmt.Errorf("ollama decode: %w", err)
	}
	return out.Message, nil
}

// Embed generates an embedding vector for the given text using Ollama's embed API.
// It uses nomic-embed-text by default, falling back to the chat model.
func (c *OllamaClient) Embed(text string) ([]float32, error) {
	model := "nomic-embed-text"
	body, _ := json.Marshal(map[string]any{
		"model": model,
		"input": text,
	})
	resp, err := c.http.Post(c.baseURL+"/api/embed", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("ollama embed decode: %w", err)
	}
	if len(out.Embeddings) == 0 || len(out.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("ollama embed: empty response")
	}
	return out.Embeddings[0], nil
}

// ── Tool builder ──────────────────────────────────────────────────────────────

// PtmSystemPrompt is the baseline system prompt appended to every agent session.
const PtmSystemPrompt = `
--- ptm assistant baseline (always active) ---
You are a Pokemon VGC team building assistant. Use tools to answer — never guess data.

Key tools:
- find_pokemon_by_filters type="steel" role="support" speed_tier="slow" — discover owned candidates by type/role/speed. Use this first for any coverage or team-building search.
- find_pokemon_by_name name="Garcho" — fuzzy name lookup. Use only when you already know the name.
- set_owned type="pokemon"|"item" name="..." owned=true|false — mark collection ownership
- search_items / get_item — look up held items
- create_team / add_pokemon / set_ability / set_nature / set_item / set_moves / set_stats — build teams
- validate_team / analyse_team / export_team — evaluate teams
- evaluate_pokemon pokemon_name="..." [team_id=N] — ALWAYS call on every candidate before recommending; verifies type chart, coverage, stat role
- add_team_log team_id=N entry="..." — record a session/combat experience
- search_knowledge query="..." — semantic search over strategy docs, meta guides, tier lists. Use natural language. Call this BEFORE making team recommendations.
- get_team_logs team_id=N — retrieve session log history for a team
- get_help — full tool reference

Search workflow: identify gap → find_pokemon_by_filters → evaluate_pokemon each result → recommend. If no results, loosen one filter at a time. Never loop on name guesses.
Pokemon Champions rules: 66 stat points total, max 32/stat, all IVs 31, 6-mon teams, double battles.`

// BuildPtmTools returns the agent tool list for the given services.
// This list does NOT include chat_agent — that tool is MCP-only.
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

	formatSpecies := func(results []pokemon.Species) string {
		if len(results) == 0 {
			return ""
		}
		var b strings.Builder
		for _, s := range results {
			t2 := ""
			if s.Type2 != "" {
				t2 = "/" + string(s.Type2)
			}
			fmt.Fprintf(&b, "%s (ID %d) %s%s — BST %d\n", s.Name, s.ID, s.Type1, t2, s.BST())
		}
		return b.String()
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
				if !sp.IsFinalEvo {
					return "error: " + sp.Name + " is not a final evolution"
				}
				abilities, err := svc.Pokemon.GetAbilitiesForSpecies(sp.ID)
				if err != nil || len(abilities) == 0 {
					return "error: no abilities configured for " + sp.Name
				}
				abilityID := abilities[0].ID
				if abName := str(args, "ability"); abName != "" {
					ab, err := svc.Pokemon.GetAbilityByName(abName)
					if err != nil {
						return "error: " + err.Error()
					}
					if ok, _ := svc.Pokemon.HasAbility(sp.ID, ab.ID); !ok {
						return "error: " + abName + " is not a valid ability for " + sp.Name
					}
					abilityID = ab.ID
				}
				memberID, err := svc.Team.AddMember(teamID, sp.ID, abilityID)
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
				memberID, err := svc.Team.GetMemberByTeamAndSpecies(num(args, "team_id"), str(args, "pokemon_name"))
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
				memberID, err := svc.Team.GetMemberByTeamAndSpecies(teamID, pokeName)
				if err != nil {
					return "error: " + err.Error()
				}
				sp, _ := svc.Pokemon.GetSpeciesByName(pokeName)
				ab, err := svc.Pokemon.GetAbilityByName(str(args, "ability"))
				if err != nil {
					return "error: " + err.Error()
				}
				if sp != nil {
					if ok, _ := svc.Pokemon.HasAbility(sp.ID, ab.ID); !ok {
						return "error: " + ab.Name + " is not a valid ability for " + pokeName
					}
				}
				if err := svc.Team.SetAbility(memberID, ab.ID); err != nil {
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
				memberID, err := svc.Team.GetMemberByTeamAndSpecies(num(args, "team_id"), str(args, "pokemon_name"))
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
				item, err := svc.Pokemon.GetItemByName(str(args, "item"))
				if err != nil {
					return "error: " + err.Error()
				}
				if item.IsBanned {
					return "error: " + item.Name + " is banned"
				}
				t, err := svc.Team.GetTeam(teamID)
				if err != nil {
					return "error: " + err.Error()
				}
				for _, m := range t.Members {
					if m.Item != nil && m.Item.ID == item.ID && !strings.EqualFold(m.Species.Name, pokeName) {
						return "error: item clause: " + item.Name + " already held by " + m.Species.Name
					}
				}
				memberID, err := svc.Team.GetMemberByTeamAndSpecies(teamID, pokeName)
				if err != nil {
					return "error: " + err.Error()
				}
				if err := svc.Team.SetItem(memberID, item.ID); err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Set %s's item to %s.", pokeName, item.Name)
			},
		},
		{
			Schema: schema("set_moves", "Set up to 4 moves for a Pokemon. All moves must be in the species' learnset unless force=true.",
				map[string]any{
					"team_id":      prop("integer", "Team ID"),
					"pokemon_name": prop("string", "Species name"),
					"move1":        prop("string", "First move name"),
					"move2":        prop("string", "Second move name"),
					"move3":        prop("string", "Third move name"),
					"move4":        prop("string", "Fourth move name"),
					"force":        prop("boolean", "Skip learnset validation"),
				},
				[]string{"team_id", "pokemon_name", "move1"}),
			Execute: func(args map[string]any) string {
				teamID := num(args, "team_id")
				pokeName := str(args, "pokemon_name")
				force, _ := args["force"].(bool)
				sp, err := svc.Pokemon.GetSpeciesByName(pokeName)
				if err != nil {
					return "error: " + err.Error()
				}
				var moveIDs []int
				for _, key := range []string{"move1", "move2", "move3", "move4"} {
					name := str(args, key)
					if name == "" {
						continue
					}
					mv, err := svc.Pokemon.GetMoveByName(name)
					if err != nil {
						return "error: " + err.Error()
					}
					if !force {
						ok, lerr := svc.Pokemon.CanLearnMove(sp.ID, mv.ID)
						if !ok {
							msg := name + " is not in " + pokeName + "'s learnset"
							if lerr != nil {
								msg = lerr.Error()
							}
							return "error: " + msg
						}
					}
					moveIDs = append(moveIDs, mv.ID)
				}
				memberID, err := svc.Team.GetMemberByTeamAndSpecies(teamID, pokeName)
				if err != nil {
					return "error: " + err.Error()
				}
				if err := svc.Team.SetMoves(memberID, moveIDs); err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Set %d move(s) for %s.", len(moveIDs), pokeName)
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
				memberID, err := svc.Team.GetMemberByTeamAndSpecies(num(args, "team_id"), str(args, "pokemon_name"))
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
				memberID, err := svc.Team.GetMemberByTeamAndSpecies(num(args, "team_id"), str(args, "pokemon_name"))
				if err != nil {
					return "error: " + err.Error()
				}
				if err := svc.Team.SetMemberNotes(memberID, str(args, "notes")); err != nil {
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
				if err := svc.Team.UpdateTeamNotes(num(args, "team_id"), str(args, "notes")); err != nil {
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
				memberID, err := svc.Team.GetMemberByTeamAndSpecies(num(args, "team_id"), str(args, "pokemon_name"))
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
				memberID, err := svc.Team.GetMemberByTeamAndSpecies(num(args, "team_id"), str(args, "pokemon_name"))
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
			Schema: schema("list_teams", "List all teams in the database.", nil, nil),
			Execute: func(args map[string]any) string {
				teams, err := ListTeams(svc)
				if err != nil {
					return "error: " + err.Error()
				}
				if len(teams) == 0 {
					return "No teams found."
				}
				var b strings.Builder
				for _, t := range teams {
					fmt.Fprintf(&b, "Team %d: %s (Regulation %s, %d members)\n", t.ID, t.Name, t.Regulation, t.MemberCount)
				}
				return b.String()
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
				var b strings.Builder
				fmt.Fprintf(&b, "Team %d: %s (Regulation %s)\n", t.ID, t.Name, t.Regulation)
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
					var moves []string
					for _, mv := range m.Moves {
						if mv != nil {
							moves = append(moves, mv.Name)
						}
					}
					fmt.Fprintf(&b, "  [%d] %s | %s | %s | %s | EVs: HP%d Atk%d Def%d SpA%d SpD%d Spe%d\n",
						m.Slot, m.Species.Name, ability, item, m.Nature,
						m.EVs.HP, m.EVs.Atk, m.EVs.Def, m.EVs.SpA, m.EVs.SpD, m.EVs.Spe)
					if len(moves) > 0 {
						fmt.Fprintf(&b, "       Moves: %s\n", strings.Join(moves, " / "))
					}
				}
				return b.String()
			},
		},
		{
			Schema: schema("find_pokemon_by_name",
				"Look up owned Pokemon by name. Use when you already know the name. For discovery use find_pokemon_by_filters.",
				map[string]any{
					"name":  prop("string", "Species name or partial name (fuzzy match)"),
					"owned": prop("boolean", "Default true; set false to include unowned"),
					"limit": prop("integer", "Max results (default 10)"),
				},
				[]string{"name"}),
			Execute: func(args map[string]any) string {
				var owned *bool
				if v, ok := args["owned"]; ok && v != nil {
					b := v.(bool)
					owned = &b
				}
				results, err := FindPokemonByName(svc, str(args, "name"), owned, num(args, "limit"))
				if err != nil {
					return "error: " + err.Error()
				}
				if len(results) == 0 {
					return "No owned Pokemon found with that name. If you're looking for candidates to fill a role, use find_pokemon_by_filters instead."
				}
				return formatSpecies(results)
			},
		},
		{
			Schema: schema("find_pokemon_by_filters",
				"Discover owned Pokemon candidates by type, role, and speed tier. Use for coverage gaps — never guess names.",
				map[string]any{
					"type":           prop("string", "Filter by type (e.g. 'fire', 'steel')"),
					"role":           prop("string", "'physical attacker'|'special attacker'|'mixed attacker'|'support'|'tank'"),
					"speed_tier":     prop("string", "'fast' (>100 base)|'mid' (70-100)|'slow' (<70)"),
					"legendary":      prop("boolean", "Filter by legendary status"),
					"final_evo_only": prop("boolean", "Only final evolutions"),
					"owned":          prop("boolean", "Default true; set false to include unowned"),
					"limit":          prop("integer", "Max results (default 20)"),
				},
				[]string{}),
			Execute: func(args map[string]any) string {
				f := pokemon.SpeciesFilter{
					Type:         str(args, "type"),
					Role:         str(args, "role"),
					SpeedTier:    str(args, "speed_tier"),
					FinalEvoOnly: func() bool { b, _ := args["final_evo_only"].(bool); return b }(),
				}
				if v, ok := args["owned"]; ok && v != nil {
					b := v.(bool)
					f.Owned = &b
				}
				if v, ok := args["legendary"]; ok && v != nil {
					b := v.(bool)
					f.Legendary = &b
				}
				results, err := FindPokemonByFilters(svc, f, num(args, "limit"))
				if err != nil {
					return "error: " + err.Error()
				}
				if len(results) == 0 {
					return "No owned Pokemon match those filters. Try broadening: remove role or speed_tier constraints."
				}
				return formatSpecies(results)
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
				abilities, _ := svc.Pokemon.GetAbilitiesForSpecies(sp.ID)
				out, _ := json.Marshal(map[string]any{"species": sp, "abilities": abilities})
				return string(out)
			},
		},
		{
			Schema: schema("get_moves", "Get the full learnset for a species.",
				map[string]any{"pokemon_name": prop("string", "Pokemon name")},
				[]string{"pokemon_name"}),
			Execute: func(args map[string]any) string {
				sp, err := svc.Pokemon.GetSpeciesByName(str(args, "pokemon_name"))
				if err != nil {
					return "error: " + err.Error()
				}
				moves, err := svc.Pokemon.GetLearnset(sp.ID)
				if err != nil {
					return "error: " + err.Error()
				}
				if len(moves) == 0 {
					return fmt.Sprintf("No learnset data for %s. Use add_learnset_move to populate.", sp.Name)
				}
				out, _ := json.Marshal(moves)
				return string(out)
			},
		},
		{
			Schema: schema("add_learnset_move", "Add a move to a species' learnset. Use when the user confirms a move is available in-game.",
				map[string]any{
					"pokemon_name": prop("string", "Species name"),
					"move_name":    prop("string", "Move name"),
				},
				[]string{"pokemon_name", "move_name"}),
			Execute: func(args map[string]any) string {
				sp, err := svc.Pokemon.GetSpeciesByName(str(args, "pokemon_name"))
				if err != nil {
					return "error: " + err.Error()
				}
				mv, err := svc.Pokemon.GetMoveByName(str(args, "move_name"))
				if err != nil {
					return "error: " + err.Error()
				}
				if err := svc.Pokemon.AddLearnsetMove(sp.ID, mv.ID); err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Added %s to %s's learnset.", mv.Name, sp.Name)
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
					"limit":        prop("integer", "Max results (default 20)"),
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
				moves, err := svc.Pokemon.SearchMoves(str(args, "query"), num(args, "limit"), f)
				if err != nil {
					return "error: " + err.Error()
				}
				out, _ := json.Marshal(moves)
				return string(out)
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
				out, _ := json.Marshal(item)
				return string(out)
			},
		},
		{
			Schema: schema("search_items", "Search held items by name or effect description.",
				map[string]any{
					"query": prop("string", "Search term (empty to list all)"),
					"owned": prop("boolean", "Filter to owned/unowned items"),
					"banned": prop("boolean", "Filter to banned/legal items"),
					"limit": prop("integer", "Max results (default 20)"),
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
				items, err := svc.Pokemon.SearchItems(str(args, "query"), num(args, "limit"), f)
				if err != nil {
					return "error: " + err.Error()
				}
				out, _ := json.Marshal(items)
				return string(out)
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
		{
			Schema: schema("delete_team", "Delete a team and all its members permanently.",
				map[string]any{"team_id": prop("integer", "Team ID to delete")},
				[]string{"team_id"}),
			Execute: func(args map[string]any) string {
				if err := svc.Team.DeleteTeam(num(args, "team_id")); err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Team %d deleted.", num(args, "team_id"))
			},
		},
		{
			Schema: schema("set_tera_type", "Set the Tera Type for a Pokemon on a team.",
				map[string]any{
					"team_id":      prop("integer", "Team ID"),
					"pokemon_name": prop("string", "Species name"),
					"tera_type":    prop("string", "Type name (e.g. 'fire', 'fairy')"),
				},
				[]string{"team_id", "pokemon_name", "tera_type"}),
			Execute: func(args map[string]any) string {
				memberID, err := svc.Team.GetMemberByTeamAndSpecies(num(args, "team_id"), str(args, "pokemon_name"))
				if err != nil {
					return "error: " + err.Error()
				}
				if err := svc.Team.SetTeraType(memberID, str(args, "tera_type")); err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Set Tera Type to %s.", str(args, "tera_type"))
			},
		},
		{
			Schema: schema("list_regulations", "List all VGC regulation sets.", nil, nil),
			Execute: func(args map[string]any) string {
				regs, err := svc.Team.ListRegulations()
				if err != nil {
					return "error: " + err.Error()
				}
				out, _ := json.Marshal(regs)
				return string(out)
			},
		},
		{
			Schema: schema("get_regulation", "Get details for a VGC regulation set including banned and restricted species.",
				map[string]any{"id": prop("string", "Regulation ID (e.g. 'H')")},
				[]string{"id"}),
			Execute: func(args map[string]any) string {
				regs, err := svc.Team.ListRegulations()
				if err != nil {
					return "error: " + err.Error()
				}
				id := strings.ToUpper(str(args, "id"))
				for _, r := range regs {
					if r.ID == id {
						out, _ := json.Marshal(r)
						return string(out)
					}
				}
				return "error: regulation not found: " + id
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
					if m.Species == nil {
						continue
					}
					spVP := (m.EVs.HP + m.EVs.Atk + m.EVs.Def + m.EVs.SpA + m.EVs.SpD + m.EVs.Spe) * 2
					natureVP := 0
					if m.Nature != "" && !strings.EqualFold(m.Nature, "Serious") {
						natureVP = 200
					}
					moveVP := len(m.Moves) * 100
					abilityVP := 0
					if m.Ability != nil {
						if abilities, err := svc.Pokemon.GetAbilitiesForSpecies(m.Species.ID); err == nil && len(abilities) > 0 && abilities[0].ID != m.Ability.ID {
							abilityVP = 400
						}
					}
					memberTotal := spVP + natureVP + moveVP + abilityVP
					total += memberTotal
					fmt.Fprintf(&b, "%s: %d VP (SP %d + nature %d + moves %d + ability %d)\n", m.Species.Name, memberTotal, spVP, natureVP, moveVP, abilityVP)
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
				"Retrieve all combat/session log entries for a team, newest first.",
				map[string]any{"team_id": prop("integer", "Team ID")},
				[]string{"team_id"}),
			Execute: func(args map[string]any) string {
				logs, err := GetTeamLogs(svc, num(args, "team_id"))
				if err != nil {
					return "error: " + err.Error()
				}
				if len(logs) == 0 {
					return "No log entries yet."
				}
				var b strings.Builder
				for _, l := range logs {
					fmt.Fprintf(&b, "[%s]\n%s\n\n", l.CreatedAt.Format("2006-01-02 15:04"), l.Entry)
				}
				return b.String()
			},
		},
	}
}

// ── Agent loop ────────────────────────────────────────────────────────────────

const agentMaxTurns = 10

// RunAgent runs the Ollama agent loop for one user turn.
// Returns produced display messages, updated context history, and any UI view.
func RunAgent(ollama *OllamaClient, svc *Services, history []OllamaMessage, userMsg string) ([]ChatMessage, []OllamaMessage, *AgentView) {
	agentTools := BuildPtmTools(svc)

	ollamaTools := make([]OlamaTool, len(agentTools))
	for i, t := range agentTools {
		ollamaTools[i] = t.Schema
	}
	toolMap := make(map[string]func(map[string]any) string, len(agentTools))
	for _, t := range agentTools {
		toolMap[t.Schema.Function.Name] = t.Execute
	}

	systemContent := strings.TrimSpace(ollama.cfg.Prompt) + "\n" + PtmSystemPrompt
	messages := []OllamaMessage{{Role: "system", Content: systemContent}}
	messages = append(messages, history...)
	messages = append(messages, OllamaMessage{Role: "user", Content: userMsg})

	var produced []ChatMessage
	var view *AgentView
	lastSig := ""

	for range agentMaxTurns {
		reply, err := ollama.Chat(messages, ollamaTools)
		if err != nil {
			produced = append(produced, ChatMessage{Role: "error", Content: err.Error()})
			break
		}
		messages = append(messages, reply)

		if len(reply.ToolCalls) == 0 {
			if reply.Content != "" {
				produced = append(produced, ChatMessage{Role: "assistant", Content: reply.Content})
			}
			break
		}

		for _, tc := range reply.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Function.Arguments)
			sig := tc.Function.Name + string(argsJSON)
			if sig == lastSig {
				produced = append(produced, ChatMessage{Role: "error", Content: "loop detected, stopping"})
				goto done
			}
			lastSig = sig

			produced = append(produced, ChatMessage{
				Role:    "tool",
				Content: fmt.Sprintf("→ %s(%s)", tc.Function.Name, string(argsJSON)),
			})

			fn, ok := toolMap[tc.Function.Name]
			var result string
			if !ok {
				result = "unknown tool: " + tc.Function.Name
			} else {
				result = fn(tc.Function.Arguments)
			}

			if view == nil {
				view = inferView(tc.Function.Name, tc.Function.Arguments)
			}

			produced = append(produced, ChatMessage{Role: "tool", Content: "  ← " + truncate(result, 150)})
			messages = append(messages, OllamaMessage{Role: "tool", Content: result})
		}
	}

done:
	ctx := trimHistory(messages)
	return produced, ctx, view
}

func inferView(toolName string, args map[string]any) *AgentView {
	switch toolName {
	case "get_team", "analyse_team", "validate_team":
		if id, ok := args["team_id"]; ok {
			var tid int
			switch v := id.(type) {
			case float64:
				tid = int(v)
			case int:
				tid = v
			}
			if tid > 0 {
				return &AgentView{Type: "team", TeamID: tid}
			}
		}
	case "get_pokemon":
		if name, ok := args["name"].(string); ok && name != "" {
			return &AgentView{Type: "pokemon", PokemonName: name}
		}
	case "find_pokemon_by_name":
		if n, ok := args["name"].(string); ok && n != "" {
			return &AgentView{Type: "pokemon", PokemonName: n}
		}
	case "find_pokemon_by_filters":
		if t, ok := args["type"].(string); ok && t != "" {
			return &AgentView{Type: "pokemon", PokemonName: t}
		}
	case "evaluate_pokemon":
		if name, ok := args["pokemon_name"].(string); ok && name != "" {
			v := &AgentView{Type: "evaluate", PokemonName: name}
			if id, ok := args["team_id"]; ok {
				switch val := id.(type) {
				case float64:
					v.TeamID = int(val)
				case int:
					v.TeamID = val
				}
			}
			return v
		}
	}
	return nil
}

func trimHistory(messages []OllamaMessage) []OllamaMessage {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i:]
		}
	}
	return nil
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
