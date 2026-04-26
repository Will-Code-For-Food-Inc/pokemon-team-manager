package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

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

func getBool(args map[string]any, key string) bool {
	if args == nil {
		return false
	}
	v, _ := args[key].(bool)
	return v
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
