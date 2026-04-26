package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/user/pokemon-team-manager/internal/handlers"
	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

// ── Ollama types ──────────────────────────────────────────────────────────────

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaToolCallFn `json:"function"`
}

type ollamaToolCallFn struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ollamaTool struct {
	Type     string       `json:"type"`
	Function ollamaToolFn `json:"function"`
}

type ollamaToolFn struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type ollamaClient struct {
	baseURL string
	model   string
	cfg     agentConfig
	http    *http.Client
}

func newOllamaClient(cfg agentConfig) *ollamaClient {
	return &ollamaClient{
		baseURL: cfg.OllamaURL,
		model:   cfg.Model,
		cfg:     cfg,
		http:    &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *ollamaClient) chat(messages []ollamaMessage, tools []ollamaTool) (ollamaMessage, error) {
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
		return ollamaMessage{}, fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		Message ollamaMessage `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ollamaMessage{}, fmt.Errorf("ollama decode: %w", err)
	}
	return out.Message, nil
}

// ── Tool definitions ───────────────────────────────────────────────────────────

type agentTool struct {
	schema  ollamaTool
	execute func(args map[string]any) string
}

func buildPtmTools(svc *Services) []agentTool {
	hsvc := svc.Handlers()

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
	schema := func(name, desc string, props map[string]any, required []string) ollamaTool {
		return ollamaTool{
			Type: "function",
			Function: ollamaToolFn{
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

	return []agentTool{
		{
			schema: schema("list_teams", "List all teams in the database.", nil, nil),
			execute: func(args map[string]any) string {
				teams, err := handlers.ListTeams(hsvc)
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
			schema: schema("get_team", "Get full details of a team including all members, moves, items, and stats.",
				map[string]any{"team_id": prop("integer", "Team ID")},
				[]string{"team_id"}),
			execute: func(args map[string]any) string {
				t, err := handlers.GetTeam(hsvc, num(args, "team_id"))
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
			schema: schema("find_pokemon_by_name",
				"Look up owned Pokemon by name. Use when you already know the name. For discovery use find_pokemon_by_filters.",
				map[string]any{
					"name":  prop("string", "Species name or partial name (fuzzy match)"),
					"owned": prop("boolean", "Default true; set false to include unowned"),
					"limit": prop("integer", "Max results (default 10)"),
				},
				[]string{"name"}),
			execute: func(args map[string]any) string {
				var owned *bool
				if v, ok := args["owned"]; ok && v != nil {
					b := v.(bool)
					owned = &b
				}
				results, err := handlers.FindPokemonByName(hsvc, str(args, "name"), owned, num(args, "limit"))
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
			schema: schema("find_pokemon_by_filters",
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
			execute: func(args map[string]any) string {
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
				results, err := handlers.FindPokemonByFilters(hsvc, f, num(args, "limit"))
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
			schema: schema("analyse_team", "Analyse a team for speed tiers, type coverage, and weaknesses.",
				map[string]any{"team_id": prop("integer", "Team ID")},
				[]string{"team_id"}),
			execute: func(args map[string]any) string {
				t, a, err := handlers.AnalyseTeam(hsvc, num(args, "team_id"))
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
			schema: schema("validate_team", "Check a team for VGC rule violations.",
				map[string]any{"team_id": prop("integer", "Team ID")},
				[]string{"team_id"}),
			execute: func(args map[string]any) string {
				violations, err := handlers.ValidateTeam(hsvc, num(args, "team_id"))
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
			schema: schema("search_knowledge", "Search the VGC strategy knowledge base.",
				map[string]any{"query": prop("string", "Search query")},
				[]string{"query"}),
			execute: func(args map[string]any) string {
				results, err := handlers.SearchKnowledge(hsvc, str(args, "query"), num(args, "limit"))
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
			schema: schema("calc_stats", "Calculate final Level 50 stats for a Pokemon given its base stats, stat points, and nature.",
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
			execute: func(args map[string]any) string {
				spread := team.StatSpread{
					HP:  num(args, "hp"),
					Atk: num(args, "attack"),
					Def: num(args, "defense"),
					SpA: num(args, "sp_attack"),
					SpD: num(args, "sp_defense"),
					Spe: num(args, "speed"),
				}
				result, err := handlers.CalcStats(hsvc, str(args, "species"), spread, str(args, "nature"))
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
			schema: schema("set_owned",
				"Mark a Pokemon or item as owned (or unowned). Call this when the user says they have or don't have a specific Pokemon or item.",
				map[string]any{
					"type":  prop("string", "'pokemon' or 'item'"),
					"name":  prop("string", "Species or item name"),
					"owned": prop("boolean", "true to mark owned, false to unmark"),
				},
				[]string{"type", "name", "owned"}),
			execute: func(args map[string]any) string {
				owned, _ := args["owned"].(bool)
				msg, err := handlers.SetOwned(hsvc, str(args, "type"), str(args, "name"), owned)
				if err != nil {
					return "error: " + err.Error()
				}
				return msg
			},
		},
		{
			schema: schema("evaluate_pokemon",
				"Evaluate a Pokemon: type chart, offensive coverage, stat role, bulk, speed tier. Pass team_id for move/EV analysis.",
				map[string]any{
					"pokemon_name": prop("string", "Species name (e.g. 'Garchomp')"),
					"team_id":      prop("integer", "Team ID — enables member-level move/EV analysis"),
				},
				[]string{"pokemon_name"}),
			execute: func(args map[string]any) string {
				var teamID int
				switch v := args["team_id"].(type) {
				case float64:
					teamID = int(v)
				case int:
					teamID = v
				}
				result, err := handlers.EvaluatePokemon(hsvc, str(args, "pokemon_name"), teamID)
				if err != nil {
					return "error: " + err.Error()
				}
				switch result.Kind {
				case handlers.EvalMember:
					return team.FormatMemberEval(result.MemberEval)
				case handlers.EvalNotOnTeam:
					return fmt.Sprintf("%s is not on team %d; showing species-level evaluation.", result.SpeciesName, result.TeamID)
				default:
					return team.FormatSpeciesEval(result.SpeciesEval)
				}
			},
		},
		{
			schema: schema("add_team_log",
				"Append a combat/session log entry to a team. Use to record notable matchups, what worked, what didn't.",
				map[string]any{
					"team_id": prop("integer", "Team ID"),
					"entry":   prop("string", "Log entry text (markdown supported)"),
				},
				[]string{"team_id", "entry"}),
			execute: func(args map[string]any) string {
				id, err := handlers.AddTeamLog(hsvc, num(args, "team_id"), str(args, "entry"))
				if err != nil {
					return "error: " + err.Error()
				}
				return fmt.Sprintf("Log entry %d recorded.", id)
			},
		},
		{
			schema: schema("get_team_logs",
				"Retrieve all combat/session log entries for a team, newest first.",
				map[string]any{
					"team_id": prop("integer", "Team ID"),
				},
				[]string{"team_id"}),
			execute: func(args map[string]any) string {
				logs, err := handlers.GetTeamLogs(hsvc, num(args, "team_id"))
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

// ── Agent loop ─────────────────────────────────────────────────────────────────

// ptmSystemPrompt is always appended to the user-configured prompt so core
// tool guidance is never lost regardless of what the user sets.
const ptmSystemPrompt = `
--- ptm assistant baseline (always active) ---
You are a Pokemon VGC team building assistant. Use tools to answer — never guess data.

Key tools:
- find_pokemon_by_filters type="steel" role="support" speed_tier="slow" — discover owned candidates by type/role/speed. Use this first for any coverage or team-building search.
- find_pokemon_by_name name="Garcho" — fuzzy name lookup. Use only when you already know the name.
- search_pokemon — last resort; returns broad results. Prefer the two tools above.
- set_owned type="pokemon"|"item" name="..." owned=true|false — mark collection ownership
- search_items / get_item — look up held items
- create_team / add_pokemon / set_ability / set_nature / set_item / set_moves / set_stats — build teams
- validate_team / analyse_team / export_team — evaluate teams
- evaluate_pokemon pokemon_name="..." [team_id=N] — ALWAYS call on every candidate before recommending; verifies type chart, coverage, stat role
- add_team_log team_id=N entry="..." — record a session/combat experience
- get_team_logs team_id=N — retrieve session log history for a team
- get_help — full tool reference

Search workflow: identify gap → find_pokemon_by_filters → evaluate_pokemon each result → recommend. If no results, loosen one filter at a time. Never loop on name guesses.
Pokemon Champions rules: 66 stat points total, max 32/stat, all IVs 31, 6-mon teams, double battles.`

const agentMaxTurns = 10

type chatMessage struct {
	Role    string `json:"role"`    // "user" | "assistant" | "tool" | "error"
	Content string `json:"content"`
}

// agentView carries UI state the agent explicitly requested during its turn.
type agentView struct {
	Type        string `json:"type"`                   // "team" | "pokemon"
	TeamID      int    `json:"team_id,omitempty"`      // set for type="team"
	PokemonName string `json:"pokemon_name,omitempty"` // set for type="pokemon"
}

// runAgent runs the agent loop and returns produced display messages, updated
// context history, and any view the agent requested (nil if none).
func runAgent(ollama *ollamaClient, svc *Services, history []ollamaMessage, userMsg string) ([]chatMessage, []ollamaMessage, *agentView) {
	tools := buildPtmTools(svc)

	ollamaTools := make([]ollamaTool, len(tools))
	for i, t := range tools {
		ollamaTools[i] = t.schema
	}
	toolMap := make(map[string]func(map[string]any) string, len(tools))
	for _, t := range tools {
		toolMap[t.schema.Function.Name] = t.execute
	}

	systemContent := strings.TrimSpace(ollama.cfg.Prompt) + "\n" + ptmSystemPrompt
	messages := []ollamaMessage{{Role: "system", Content: systemContent}}
	messages = append(messages, history...)
	messages = append(messages, ollamaMessage{Role: "user", Content: userMsg})

	var produced []chatMessage
	var view *agentView
	lastSig := ""

	for range agentMaxTurns {
		reply, err := ollama.chat(messages, ollamaTools)
		if err != nil {
			produced = append(produced, chatMessage{Role: "error", Content: err.Error()})
			break
		}
		messages = append(messages, reply)

		if len(reply.ToolCalls) == 0 {
			if reply.Content != "" {
				produced = append(produced, chatMessage{Role: "assistant", Content: reply.Content})
			}
			break
		}

		for _, tc := range reply.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Function.Arguments)
			sig := tc.Function.Name + string(argsJSON)
			if sig == lastSig {
				produced = append(produced, chatMessage{Role: "error", Content: "loop detected, stopping"})
				goto done
			}
			lastSig = sig

			produced = append(produced, chatMessage{
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

			// Auto-surface panel based on which tool was called.
			switch tc.Function.Name {
			case "get_team", "analyse_team", "validate_team":
				if id, ok := tc.Function.Arguments["team_id"]; ok {
					var tid int
					switch v := id.(type) {
					case float64:
						tid = int(v)
					case int:
						tid = v
					}
					if tid > 0 {
						view = &agentView{Type: "team", TeamID: tid}
					}
				}
			case "get_pokemon":
				if name, ok := tc.Function.Arguments["name"].(string); ok && name != "" {
					view = &agentView{Type: "pokemon", PokemonName: name}
				}
			case "find_pokemon_by_name":
				if n, ok := tc.Function.Arguments["name"].(string); ok && n != "" {
					view = &agentView{Type: "pokemon", PokemonName: n}
				}
			case "find_pokemon_by_filters":
				if t, ok := tc.Function.Arguments["type"].(string); ok && t != "" {
					view = &agentView{Type: "pokemon", PokemonName: t}
				}
			case "evaluate_pokemon":
				if name, ok := tc.Function.Arguments["pokemon_name"].(string); ok && name != "" {
					view = &agentView{Type: "evaluate", PokemonName: name}
					if id, ok := tc.Function.Arguments["team_id"]; ok {
						switch v := id.(type) {
						case float64:
							view.TeamID = int(v)
						case int:
							view.TeamID = v
						}
					}
				}
			}

			produced = append(produced, chatMessage{Role: "tool", Content: "  ← " + truncate(result, 150)})
			messages = append(messages, ollamaMessage{Role: "tool", Content: result})
		}
	}

done:
	ctx := trimHistory(messages)
	return produced, ctx, view
}

func trimHistory(messages []ollamaMessage) []ollamaMessage {
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
