package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
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
	ItemName    string `json:"item_name,omitempty"`
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
--- Sable baseline (always active) ---
You are Sable, a peppy female Ace Trainer and Pokemon Champions coach. You're sharp, enthusiastic, and competitive — you live for optimizing teams and love breaking down matchups. You speak with energy and confidence, like someone who's battled her way to the top and wants to bring everyone with her. You use battle metaphors naturally and get genuinely excited about good synergy or a clever tech pick. Be concise — you're a specialist coach, not a chatbot. Use tools to answer — never guess data.

Response style: be terse. The UI shows data cards automatically in the right panel whenever you call a data tool — do NOT re-list stats, moves, type matchups, or base numbers in your text. Provide only inference: your recommendation, the reason, and any tradeoffs. One to three sentences is ideal. If the user explicitly asks you to explain or list something, then do so.

Data views: calling get_pokemon, get_team, evaluate_pokemon, etc. automatically renders a card in the user's scrolling data panel. If the user asks to "show" or "look at" a Pokemon or team, just call the relevant tool and tell them to check the data panel — you don't need to describe the data yourself. Example: "I've pulled up Tyranitar in the data panel on the right."

Key tools:
- find_pokemon_by_filters type="steel" role="support" speed_tier="slow" — discover owned candidates by type/role/speed. Use this first for any coverage or team-building search.
- find_pokemon_by_name name="Garcho" — fuzzy name lookup. Use only when you already know the name.
- set_owned type="pokemon"|"item" name="..." owned=true|false — mark collection ownership
- search_items / get_item — look up held items
- create_team / copy_team / delete_team / rename_team — team management
- add_pokemon / remove_pokemon / swap_slots — roster editing
- set_ability / set_nature / set_item / set_moves / set_stats — member config (mega evolution is implicit via held Mega Stone item; there is no Tera in Champions)
- set_notes (per-build notes) / set_team_notes (team gameplan/strategy) / set_role / set_nickname — annotations. When writing team strategy notes, first call search_knowledge with query "team strategy template" to load the canonical structure, then fill in each section for the specific team.
- validate_team / analyse_team / export_team — evaluate teams
- evaluate_pokemon pokemon_name="..." [team_id=N] — ALWAYS call on every candidate before recommending; verifies type chart, coverage, stat role
- add_team_log team_id=N entry="..." — record a session/combat experience
- search_knowledge query="..." — semantic search over strategy docs, meta guides, tier lists. Use natural language. Call this BEFORE making team recommendations.
- get_team_logs team_id=N — retrieve session log history for a team
- get_help — full tool reference

Search workflow: identify gap → find_pokemon_by_filters → evaluate_pokemon each result → recommend. If no results, loosen one filter at a time. Never loop on name guesses.

Learnset workflow: the Champions learnset data is curated but may have gaps.
- If set_moves fails with "not in learnset": call get_moves to see what IS available and pick a legal alternative.
- Do not attempt to add moves yourself — never call add_learnset_move. The data is fixed externally. If no legal alternative fits, return that fact to whoever called you (parent agent or user).

Pokemon Champions rules: 66 stat points total, max 32/stat, all IVs 31, 6-mon teams, double battles.

Curated data sets: Champions has no RPG layer, so the held-item and move sets are smaller than the broader Pokemon games. The following mainline items DO NOT exist here, so do NOT call set_item with them: Life Orb, Eviolite, Assault Vest, Choice Specs, Choice Band, Booster Energy, Clear Amulet, Mirror Herb, Loaded Dice, Covert Cloak, Safety Goggles. Items that DO exist: Choice Scarf, Focus Sash, Focus Band, Leftovers, Sitrus Berry, Lum Berry, Light Ball, Scope Lens, Mental Herb, Black Glasses, Black Belt, plus Mega Stones. If unsure, call search_items('') first.`

// ── Agent loop ────────────────────────────────────────────────────────────────

const (
	// agentMaxResumes caps auto-continuations after a state's MaxTurns budget
	// is exhausted without producing a final assistant message. Per-state.
	agentMaxResumes = 1
	// maxToolResultChars caps any single tool result fed back to the model.
	maxToolResultChars = 2000
	// keepRecentToolFulls — when compacting under context pressure, never stub
	// the most recent N tool results; the model needs them to reason next.
	keepRecentToolFulls = 3
)

// capToolResult truncates a single tool result if it exceeds the per-result
// budget, leaving a tail note so the model knows it was clipped.
func capToolResult(result string) string {
	if len(result) <= maxToolResultChars {
		return result
	}
	head := result[:maxToolResultChars-120]
	return fmt.Sprintf("%s\n... [truncated; full result was %d chars — refine your query for less data]", head, len(result))
}

// compactMessages keeps the messages slice within maxChars by replacing the
// oldest tool-result contents with short stubs. Structure is preserved: the
// assistant tool_call entries that reference these results stay in place, just
// with shorter follow-up content. Returns the new slice and how many tool
// results were stubbed.
func compactMessages(messages []OllamaMessage, maxChars int) ([]OllamaMessage, int) {
	total := 0
	for _, m := range messages {
		total += len(m.Content)
	}
	if total <= maxChars {
		return messages, 0
	}

	out := make([]OllamaMessage, len(messages))
	copy(out, messages)

	var toolIdx []int
	for i, m := range out {
		if m.Role == "tool" {
			toolIdx = append(toolIdx, i)
		}
	}

	cutoff := len(toolIdx) - keepRecentToolFulls
	if cutoff <= 0 {
		return out, 0
	}

	stubbed := 0
	for i := 0; i < cutoff; i++ {
		idx := toolIdx[i]
		orig := out[idx].Content
		if len(orig) <= 80 {
			continue
		}
		out[idx].Content = fmt.Sprintf("[older tool result compacted, was %d chars]", len(orig))
		stubbed++
		total -= len(orig) - len(out[idx].Content)
		if total <= maxChars {
			break
		}
	}
	return out, stubbed
}

// EmbedAndSave generates an embedding for a chat message and saves it.
// Runs best-effort — errors are silently ignored so they never block the agent.
func EmbedAndSave(ollama *OllamaClient, repo *ChatRepo, id int64, text string) {
	vec, err := ollama.Embed(text)
	if err != nil || len(vec) == 0 {
		return
	}
	_ = repo.SaveEmbedding(id, vec)
}

// RunAgent runs the Sable orchestrator for one user turn. Internally this
// dispatches into the recursive state machine: orchestrator (sable) at depth 0
// holds only delegate_* tools and routes to scout/team_builder, which in turn
// may delegate further (team_builder → pokebuilder). Sub-agents are stateless;
// they receive a self-contained task string.
//
// onMsg, if non-nil, is called for each surfaced message (tool indicators,
// view hints, the final assistant text). Messages are also accumulated and
// returned for persistence.
//
// Each call generates a stable run_id that's stamped on every structured-log
// event for the turn — the agent_graph.py script reconstructs the state
// machine flow by grouping log lines on this id.
func RunAgent(ollama *OllamaClient, svc *Services, history []OllamaMessage, userMsg string, onMsg func(ChatMessage)) ([]ChatMessage, []OllamaMessage, *AgentView) {
	orch, ok := agentStates["sable"]
	if !ok {
		m := ChatMessage{Role: "error", Content: "agent registry missing 'sable' orchestrator state"}
		if onMsg != nil {
			onMsg(m)
		}
		return []ChatMessage{m}, nil, nil
	}

	runID := strconv.FormatInt(time.Now().UnixMicro(), 36)
	slog.Info("agent.run_start",
		"run_id", runID,
		"top_state", orch.Name,
		"user_msg_len", len(userMsg),
		"history_len", len(history),
	)

	systemContent := strings.TrimSpace(ollama.cfg.Prompt) + "\n" + orch.SystemPrompt
	messages := []OllamaMessage{{Role: "system", Content: systemContent}}
	messages = append(messages, history...)
	messages = append(messages, OllamaMessage{Role: "user", Content: userMsg})

	produced, ctx, view := runStateLoop(ollama, svc, runID, "user", orch, messages, 0, defaultMaxAgentDepth, onMsg)

	viewType := ""
	if view != nil {
		viewType = view.Type
	}
	slog.Info("agent.run_end",
		"run_id", runID,
		"final_view_type", viewType,
		"produced_msgs", len(produced),
	)
	return produced, ctx, view
}

// runStateLoop runs one agent state's tool-call loop against the given message
// buffer. This is the shared core: same compaction, same loop-detection, same
// turn budget + 1-resume nudge as the original single-agent loop. The only
// per-state knobs are state.MaxTurns and state.ToolNames (with delegate_*
// tools auto-injected for AllowedChildren).
//
// runID and parent flow purely for structured logging — they let the
// agent_graph.py script render the call tree from journald output.
func runStateLoop(
	ollama *OllamaClient,
	svc *Services,
	runID, parent string,
	state AgentState,
	messages []OllamaMessage,
	depth, maxDepth int,
	onMsg func(ChatMessage),
) ([]ChatMessage, []OllamaMessage, *AgentView) {
	slog.Info("agent.state_enter",
		"run_id", runID,
		"state", state.Name,
		"depth", depth,
		"parent", parent,
	)
	stateStart := time.Now()
	exitReason := "completed"
	turnsUsed := 0
	defer func() {
		slog.Info("agent.state_exit",
			"run_id", runID,
			"state", state.Name,
			"depth", depth,
			"turns_used", turnsUsed,
			"exit_reason", exitReason,
			"duration_ms", time.Since(stateStart).Milliseconds(),
		)
	}()

	agentTools := buildStateTools(ollama, svc, runID, state, depth, maxDepth, onMsg)

	ollamaTools := make([]OlamaTool, len(agentTools))
	for i, t := range agentTools {
		ollamaTools[i] = t.Schema
	}
	toolMap := make(map[string]func(map[string]any) string, len(agentTools))
	for _, t := range agentTools {
		toolMap[t.Schema.Function.Name] = t.Execute
	}

	emit := func(m ChatMessage) {
		if onMsg != nil {
			onMsg(m)
		}
	}

	var produced []ChatMessage
	var view *AgentView
	lastSig := ""
	loopWarned := false
	completed := false
	resumesUsed := 0
	maxMessageChars := ollama.cfg.NumCtx * 2

resumeLoop:
	for {
	turnLoop:
		for range state.MaxTurns {
			turnsUsed++
			if compacted, stubbed := compactMessages(messages, maxMessageChars); stubbed > 0 {
				slog.Warn("agent.compaction",
					"run_id", runID, "state", state.Name, "depth", depth,
					"stubbed", stubbed,
				)
				messages = compacted
				m := ChatMessage{Role: "tool", Content: fmt.Sprintf("⌂ context compacted (%d older tool results stubbed)", stubbed)}
				emit(m)
				produced = append(produced, m)
			}
			chatStart := time.Now()
			reply, err := ollama.Chat(messages, ollamaTools)
			chatDur := time.Since(chatStart).Milliseconds()
			if err != nil {
				slog.Error("agent.ollama_error",
					"run_id", runID, "state", state.Name, "depth", depth,
					"err", err.Error(), "duration_ms", chatDur,
				)
				m := ChatMessage{Role: "error", Content: err.Error()}
				emit(m)
				produced = append(produced, m)
				exitReason = "ollama_error"
				completed = true
				break turnLoop
			}
			messages = append(messages, reply)

			if len(reply.ToolCalls) == 0 {
				if reply.Content != "" {
					m := ChatMessage{Role: "assistant", Content: reply.Content}
					emit(m)
					produced = append(produced, m)
				}
				completed = true
				break turnLoop
			}

			for _, tc := range reply.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Function.Arguments)
				sig := tc.Function.Name + string(argsJSON)
				if sig == lastSig {
					if loopWarned {
						slog.Warn("agent.loop_detected",
							"run_id", runID, "state", state.Name, "depth", depth,
							"tool", tc.Function.Name, "phase", "bail",
						)
						m := ChatMessage{
							Role:    "assistant",
							Content: "I'm stuck in a loop and can't make progress on this. Please stand by — you may need to rephrase or break the request into smaller steps.",
						}
						emit(m)
						produced = append(produced, m)
						exitReason = "loop_detected"
						completed = true
						break turnLoop
					}
					slog.Warn("agent.loop_detected",
						"run_id", runID, "state", state.Name, "depth", depth,
						"tool", tc.Function.Name, "phase", "warn",
					)
					loopWarned = true
					guidance := fmt.Sprintf(
						"You just called %s with the same arguments twice in a row. "+
							"Do not repeat that call. Try a different tool, different arguments, or explain why you cannot proceed.",
						tc.Function.Name,
					)
					messages = append(messages, OllamaMessage{Role: "tool", Content: guidance})
					m := ChatMessage{Role: "tool", Content: "⚠ loop detected — guiding model"}
					emit(m)
					produced = append(produced, m)
					break
				}
				loopWarned = false
				lastSig = sig

				isDelegate := strings.HasPrefix(tc.Function.Name, "delegate_")
				slog.Info("agent.tool_call",
					"run_id", runID, "state", state.Name, "depth", depth,
					"tool", tc.Function.Name, "is_delegate", isDelegate,
					"args_chars", len(argsJSON),
				)

				m1 := ChatMessage{Role: "tool", Content: fmt.Sprintf("→ %s(%s)", tc.Function.Name, string(argsJSON))}
				emit(m1)
				produced = append(produced, m1)

				fn, ok := toolMap[tc.Function.Name]
				var result string
				toolStart := time.Now()
				if !ok {
					result = "unknown tool: " + tc.Function.Name
				} else {
					result = fn(tc.Function.Arguments)
				}
				toolDur := time.Since(toolStart).Milliseconds()
				slog.Info("agent.tool_result",
					"run_id", runID, "state", state.Name, "depth", depth,
					"tool", tc.Function.Name, "is_delegate", isDelegate,
					"duration_ms", toolDur,
					"result_chars", len(result),
					"is_error", strings.HasPrefix(result, "error:"),
				)

				m2 := ChatMessage{Role: "tool", Content: "  ← " + truncate(result, 150)}
				emit(m2)
				produced = append(produced, m2)
				messages = append(messages, OllamaMessage{Role: "tool", Content: capToolResult(result)})

				if v := inferView(tc.Function.Name, tc.Function.Arguments); v != nil {
					if view == nil {
						view = v
					}
					if onMsg != nil {
						if b, err := json.Marshal(map[string]any{"type": "view_hint", "view": v}); err == nil {
							onMsg(ChatMessage{Role: "view", Content: string(b)})
						}
					}
				}
			}
		}

		if completed {
			break resumeLoop
		}
		if resumesUsed >= agentMaxResumes {
			total := state.MaxTurns * (resumesUsed + 1)
			slog.Warn("agent.turn_exhausted",
				"run_id", runID, "state", state.Name, "depth", depth,
				"total_turns", total, "resumes_used", resumesUsed, "phase", "abandon",
			)
			m := ChatMessage{
				Role:    "error",
				Content: fmt.Sprintf("%s ran out of turns (%d) without finishing. Type 'continue' to resume, or rephrase into smaller steps.", state.Title, total),
			}
			emit(m)
			produced = append(produced, m)
			exitReason = "exhausted"
			break resumeLoop
		}
		slog.Warn("agent.turn_exhausted",
			"run_id", runID, "state", state.Name, "depth", depth,
			"resumes_used", resumesUsed, "phase", "resume",
		)
		nudge := "You ran out of turns. Your ONLY valid next action is an assistant text message — do not call any tool. " +
			"Report honestly per slot/item: list each Pokemon as 'Slot N <species>: ability=X|MISSING nature=Y|MISSING item=Z|MISSING moves=[..]|INVALID:<err> sp=[..]|MISSING'. " +
			"List any add_pokemon attempts that were rejected. Do not claim success when fields are missing or invalid. A summary that overstates completion is worse than no summary."
		messages = append(messages, OllamaMessage{Role: "user", Content: nudge})
		m := ChatMessage{Role: "tool", Content: fmt.Sprintf("⟳ %s resumed — turn budget exhausted, nudging to wrap up", state.Title)}
		emit(m)
		produced = append(produced, m)
		resumesUsed++
		lastSig = ""
		loopWarned = false
	}

	ctx := trimHistory(messages)
	return produced, ctx, view
}

// runSubAgent invokes a child state with a self-contained task string. The
// child gets a fresh message buffer (system + user[task]); it does NOT see the
// parent's conversation. The child's final assistant text is returned to the
// parent as the tool result for the delegate_<child>(...) call.
//
// Tool indicators from the child still stream to onMsg with a [Title] prefix
// so the user can see the chain of reasoning; the child's final assistant
// message is suppressed from the chat (it surfaces as the tool result instead).
func runSubAgent(
	ollama *OllamaClient,
	svc *Services,
	runID, parentName string,
	state AgentState,
	task string,
	depth, maxDepth int,
	parentOnMsg func(ChatMessage),
) string {
	childOnMsg := func(m ChatMessage) {
		if parentOnMsg == nil {
			return
		}
		// Suppress the child's assistant message — it becomes the tool result.
		// Keep error messages visible (they're failure signals the user wants).
		if m.Role == "assistant" {
			return
		}
		// Prefix tool indicators with the child's title so the user can trace
		// which agent did what.
		if m.Role == "tool" && state.Title != "" {
			m.Content = "[" + state.Title + "] " + m.Content
		}
		parentOnMsg(m)
	}

	messages := []OllamaMessage{
		{Role: "system", Content: state.SystemPrompt},
		{Role: "user", Content: task},
	}
	produced, _, _ := runStateLoop(ollama, svc, runID, parentName, state, messages, depth, maxDepth, childOnMsg)

	// Pluck the final assistant text. If no assistant message was produced
	// (turn exhaustion + no resume completion), fall back to error or stub.
	for i := len(produced) - 1; i >= 0; i-- {
		if produced[i].Role == "assistant" && produced[i].Content != "" {
			return produced[i].Content
		}
	}
	for i := len(produced) - 1; i >= 0; i-- {
		if produced[i].Role == "error" && produced[i].Content != "" {
			return "sub-agent error: " + produced[i].Content
		}
	}
	return "[" + state.Name + " produced no final answer]"
}

// buildStateTools resolves the concrete tool list for `state`: the named
// subset of BuildPtmTools, plus auto-injected delegate_<child>(task) tools
// for each AllowedChild. Delegation tools enforce the depth cap and route
// back into runSubAgent for the recursive child invocation.
func buildStateTools(
	ollama *OllamaClient,
	svc *Services,
	runID string,
	state AgentState,
	depth, maxDepth int,
	onMsg func(ChatMessage),
) []AgentTool {
	all := BuildPtmTools(svc)
	tools := subsetTools(all, state.ToolNames)

	for _, childName := range state.AllowedChildren {
		child, ok := agentStates[childName]
		if !ok {
			continue
		}
		// Each delegate tool captures the child state and recursion bookkeeping.
		childCopy := child
		tools = append(tools, AgentTool{
			Schema: OlamaTool{
				Type: "function",
				Function: OlamaToolFn{
					Name:        "delegate_" + childCopy.Name,
					Description: fmt.Sprintf("Delegate to the %s sub-agent. Pass a self-contained task string — the sub-agent does NOT see this conversation, so include team_id, current state, and exact specifications.", childCopy.Title),
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"task": map[string]any{"type": "string", "description": "Self-contained task description for the sub-agent"},
						},
						"required": []string{"task"},
					},
				},
			},
			Execute: func(args map[string]any) string {
				if depth+1 >= maxDepth {
					slog.Warn("agent.depth_exceeded",
						"run_id", runID, "from_state", state.Name,
						"to_state", childCopy.Name, "depth", depth, "max_depth", maxDepth,
					)
					return fmt.Sprintf("error: max delegation depth (%d) reached. Answer with current information instead of delegating further.", maxDepth)
				}
				task, _ := args["task"].(string)
				if task == "" {
					return "error: delegate_" + childCopy.Name + " requires a non-empty task string"
				}
				return runSubAgent(ollama, svc, runID, state.Name, childCopy, task, depth+1, maxDepth, onMsg)
			},
		})
	}
	return tools
}

func inferView(toolName string, args map[string]any) *AgentView {
	intArg := func(key string) int {
		switch v := args[key].(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
		return 0
	}
	strArg := func(key string) string {
		s, _ := args[key].(string)
		return s
	}

	switch toolName {
	case "get_team", "analyse_team", "validate_team", "export_team", "training_cost":
		if tid := intArg("team_id"); tid > 0 {
			return &AgentView{Type: "team", TeamID: tid}
		}
	case "add_pokemon", "remove_pokemon", "set_ability", "set_nature", "set_item",
		"set_moves", "set_stats", "set_role", "set_notes", "set_nickname",
		"replace_pokemon", "replace_move":
		if tid := intArg("team_id"); tid > 0 {
			return &AgentView{Type: "team", TeamID: tid}
		}
	case "get_pokemon":
		if name := strArg("name"); name != "" {
			return &AgentView{Type: "pokemon", PokemonName: name}
		}
	case "find_pokemon_by_name":
		if name := strArg("name"); name != "" {
			return &AgentView{Type: "pokemon", PokemonName: name}
		}
	case "find_pokemon_by_filters":
		// No single species name — use type as a hint label only; client renders the raw results
		if t := strArg("type"); t != "" {
			return &AgentView{Type: "pokemon", PokemonName: t}
		}
	case "evaluate_pokemon":
		if name := strArg("pokemon_name"); name != "" {
			v := &AgentView{Type: "evaluate", PokemonName: name}
			if tid := intArg("team_id"); tid > 0 {
				v.TeamID = tid
			}
			return v
		}
	case "get_item":
		if name := strArg("name"); name != "" {
			return &AgentView{Type: "item", ItemName: name}
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
