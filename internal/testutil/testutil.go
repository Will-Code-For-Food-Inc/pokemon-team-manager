// Package testutil provides shared test fixtures for ptm test suites.
// It does not import internal/tools to avoid circular dependency — callers
// are responsible for registering tools on the returned MCPServer.
package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/user/pokemon-team-manager/internal/db"
	"github.com/user/pokemon-team-manager/internal/handlers"
	"github.com/user/pokemon-team-manager/internal/knowledge"
	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

// NewServices creates an in-memory SQLite DB, seeds it from dataDir, and
// returns a fully-constructed *handlers.Services.  The DB is closed in
// t.Cleanup so callers never need to manage the lifetime themselves.
//
// Typical use from a package two levels deep (e.g. internal/tools):
//
//	svc := testutil.NewServices(t, "../../data")
func NewServices(t *testing.T, dataDir string) *handlers.Services {
	t.Helper()
	sqlDB, err := db.OpenMemory()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.Seed(sqlDB, dataDir))
	pr := pokemon.NewRepo(sqlDB)
	tr := team.NewRepo(sqlDB, pr)
	kr := knowledge.NewRepo(sqlDB)
	return &handlers.Services{DB: sqlDB, Pokemon: pr, Team: tr, Knowledge: kr}
}

// NewMCPServer creates an empty MCPServer suitable for tool registration.
// Callers must call tools.Register(s, svc) before invoking CallMCP.
func NewMCPServer() *mcpserver.MCPServer {
	return mcpserver.NewMCPServer("ptm-test", "0.0.0")
}

// CallMCP sends a tools/call JSON-RPC request to s and returns the text of
// the first TextContent item in the result.  If the server returns a
// protocol-level error (not a tool-level "Error: …" result), the returned
// string starts with "rpc-error: ".
func CallMCP(t *testing.T, s *mcpserver.MCPServer, name string, args map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": name, "arguments": args},
	})
	require.NoError(t, err)
	return extractText(t, name, s.HandleMessage(context.Background(), payload))
}

// RequireToolSuccess calls CallMCP and fatals if the result starts with "Error:".
func RequireToolSuccess(t *testing.T, s *mcpserver.MCPServer, name string, args map[string]any) string {
	t.Helper()
	out := CallMCP(t, s, name, args)
	if strings.HasPrefix(out, "Error:") || strings.HasPrefix(out, "rpc-error:") {
		t.Fatalf("tool %q returned an unexpected error: %s", name, out)
	}
	return out
}

// RequireToolError calls CallMCP and fatals if the result does NOT start with "Error:".
func RequireToolError(t *testing.T, s *mcpserver.MCPServer, name string, args map[string]any) string {
	t.Helper()
	out := CallMCP(t, s, name, args)
	if !strings.HasPrefix(out, "Error:") {
		t.Fatalf("tool %q expected an error result but got: %s", name, out)
	}
	return out
}

// CallAgentTool finds the named tool in the BuildPtmTools list and executes it.
// Fatals if the tool is not registered.
func CallAgentTool(t *testing.T, tools []handlers.AgentTool, name string, args map[string]any) string {
	t.Helper()
	for _, tool := range tools {
		if tool.Schema.Function.Name == name {
			return tool.Execute(args)
		}
	}
	t.Fatalf("agent tool %q not found", name)
	return ""
}

func extractText(t *testing.T, toolName string, resp mcp.JSONRPCMessage) string {
	t.Helper()
	switch v := resp.(type) {
	case mcp.JSONRPCResponse:
		result, ok := v.Result.(*mcp.CallToolResult)
		if !ok {
			t.Fatalf("tool %q: unexpected result type %T (want *mcp.CallToolResult)", toolName, v.Result)
		}
		var parts []string
		for _, c := range result.Content {
			if tc, ok := c.(mcp.TextContent); ok {
				parts = append(parts, tc.Text)
			}
		}
		return strings.Join(parts, "\n")
	case mcp.JSONRPCError:
		return fmt.Sprintf("rpc-error: %s", v.Error.Message)
	default:
		t.Fatalf("tool %q: unexpected response type %T", toolName, resp)
		return ""
	}
}
