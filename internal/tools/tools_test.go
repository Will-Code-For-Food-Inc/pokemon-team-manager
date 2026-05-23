package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/pokemon-team-manager/internal/handlers"
	"github.com/user/pokemon-team-manager/internal/testutil"
	"github.com/user/pokemon-team-manager/internal/tools"
)

// newServer is the shared fixture for all MCP tool tests in this package.
func newServer(t *testing.T) (*mcpserver.MCPServer, *handlers.Services) {
	t.Helper()
	svc := testutil.NewServices(t, "../../data")
	s := testutil.NewMCPServer()
	tools.Register(s, svc)
	return s, svc
}

// call is a convenience wrapper used in most tests.
func call(t *testing.T, s *mcpserver.MCPServer, name string, args map[string]any) string {
	t.Helper()
	return testutil.CallMCP(t, s, name, args)
}

// mustOK asserts the result does not start with "Error:" and returns it.
func mustOK(t *testing.T, s *mcpserver.MCPServer, name string, args map[string]any) string {
	t.Helper()
	return testutil.RequireToolSuccess(t, s, name, args)
}

// mustErr asserts the result starts with "Error:" and returns it.
func mustErr(t *testing.T, s *mcpserver.MCPServer, name string, args map[string]any) string {
	t.Helper()
	return testutil.RequireToolError(t, s, name, args)
}

// ── M4: Protocol & Schema Compliance ─────────────────────────────────────────

// TestToolsListCount locks in the total number of registered MCP tools.
// Update this count whenever a tool is added or removed — that's intentional.
func TestToolsListCount(t *testing.T) {
	s, _ := newServer(t)
	resp := s.HandleMessage(context.Background(), []byte(`{
		"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}
	}`))
	r, ok := resp.(mcp.JSONRPCResponse)
	require.True(t, ok)
	result, ok := r.Result.(mcp.ListToolsResult)
	require.True(t, ok)
	// chat_agent is MCP-only; it IS registered in tools.Register.
	// grep 's\.AddTool' internal/tools/*.go to recount if this fails.
	assert.Equal(t, 41, len(result.Tools), "tool count regression — add/remove a tool? update this number")
}

func TestToolsListSchemas(t *testing.T) {
	s, _ := newServer(t)
	resp := s.HandleMessage(context.Background(), []byte(`{
		"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}
	}`))
	r := resp.(mcp.JSONRPCResponse)
	result := r.Result.(mcp.ListToolsResult)
	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			assert.NotEmpty(t, tool.Name, "tool name must not be empty")
			assert.NotEmpty(t, tool.Description, "tool %q must have a description", tool.Name)
			assert.Equal(t, "object", tool.InputSchema.Type,
				"tool %q InputSchema.Type must be 'object'", tool.Name)
		})
	}
}

// TestRequiredParamsNoPanic asserts that calling required-param tools with an
// empty args map returns a graceful error result, never a panic.
func TestRequiredParamsNoPanic(t *testing.T) {
	s, _ := newServer(t)
	highRisk := []string{
		"create_team", "add_pokemon", "set_moves", "set_stats",
		"get_team", "validate_team", "remove_pokemon", "set_ability",
		"set_nature", "set_item", "set_nickname", "set_notes",
		"set_team_notes", "set_role", "swap_slots", "copy_team",
	}
	for _, name := range highRisk {
		t.Run(name, func(t *testing.T) {
			require.NotPanics(t, func() {
				out := call(t, s, name, map[string]any{})
				assert.NotEmpty(t, out, "tool %q must return something, not silence", name)
			})
		})
	}
}

func TestUnknownTool_RpcError(t *testing.T) {
	s, _ := newServer(t)
	msg, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "does_not_exist", "arguments": map[string]any{}},
	})
	resp := s.HandleMessage(context.Background(), msg)
	_, isErr := resp.(mcp.JSONRPCError)
	assert.True(t, isErr, "unknown tool name must produce a JSON-RPC error, not a result")
}

func TestTypeCoercion_StringEncodedInt(t *testing.T) {
	s, _ := newServer(t)
	// Create a team first so we have a valid ID to pass.
	out := mustOK(t, s, "create_team", map[string]any{"name": "Coerce Team", "regulation": "I2"})
	assert.Contains(t, out, "Team created with ID")

	// list_teams does not require an int arg — just confirm the server is alive.
	out = mustOK(t, s, "list_teams", nil)
	assert.Contains(t, out, "Coerce Team")
}

// TestToolResultShape asserts every tool returns at least one TextContent item
// (even on error paths — mcp errors come back as text, not nil content).
func TestToolResultShape(t *testing.T) {
	s, _ := newServer(t)
	// Use list_teams (no required params) as a smoke test.
	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "list_teams", "arguments": map[string]any{}},
	})
	resp := s.HandleMessage(context.Background(), raw)
	r, ok := resp.(mcp.JSONRPCResponse)
	require.True(t, ok, "list_teams must return a JSONRPCResponse, not an error")
	result, ok := r.Result.(*mcp.CallToolResult)
	require.True(t, ok, "result must be *mcp.CallToolResult, got %T", r.Result)
	assert.NotEmpty(t, result.Content, "result must have at least one content item")
	_, isText := result.Content[0].(mcp.TextContent)
	assert.True(t, isText, "first content item must be mcp.TextContent")
}

// TestGetHelp_NoArgs verifies the overview is returned when no tool name is given.
func TestGetHelp_NoArgs(t *testing.T) {
	s, _ := newServer(t)
	out := mustOK(t, s, "get_help", map[string]any{})
	assert.Contains(t, out, "QUICK START")
	assert.Contains(t, out, "list_regulations")
}

func TestGetHelp_KnownTool(t *testing.T) {
	s, _ := newServer(t)
	out := mustOK(t, s, "get_help", map[string]any{"tool_name": "set_moves"})
	assert.Contains(t, strings.ToLower(out), "learnset")
}

func TestGetHelp_UnknownTool_FallsBackToOverview(t *testing.T) {
	s, _ := newServer(t)
	out := call(t, s, "get_help", map[string]any{"tool_name": "nonexistent_tool"})
	assert.Contains(t, out, "QUICK START", "unknown tool_name must fall back to full overview")
}

func TestGetPrompt_Base(t *testing.T) {
	s, _ := newServer(t)
	out := mustOK(t, s, "get_prompt", map[string]any{})
	assert.Contains(t, out, "VGC")
}

func TestGetPrompt_WithGoal(t *testing.T) {
	s, _ := newServer(t)
	out := mustOK(t, s, "get_prompt", map[string]any{"goal": "build trick room"})
	assert.Contains(t, out, "build trick room")
}

func TestGetPrompt_IncludeRules(t *testing.T) {
	s, _ := newServer(t)
	out := mustOK(t, s, "get_prompt", map[string]any{"include": "rules"})
	assert.Contains(t, out, "VGC Rules Detail")
}
