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
