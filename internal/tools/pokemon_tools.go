package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/user/pokemon-team-manager/internal/handlers"
	"github.com/user/pokemon-team-manager/internal/pokemon"
)

func registerPokemonTools(s *server.MCPServer, svc *Services) {
	s.AddTool(mcp.NewTool("find_pokemon_by_name",
		mcp.WithDescription("Look up owned Pokemon by name. Use when you already know the name. For discovery use find_pokemon_by_filters."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Species name or partial name (fuzzy match)")),
		mcp.WithBoolean("owned", mcp.Description("Default true (owned only); false to search all")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 10)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		var owned *bool
		if v, ok := args["owned"]; ok && v != nil {
			b := getBool(args, "owned")
			owned = &b
		}
		results, err := handlers.FindPokemonByName(svc, getString(args, "name"), owned, getInt(args, "limit"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if len(results) == 0 {
			return textResult("No owned Pokemon found. For discovery use find_pokemon_by_filters."), nil
		}
		return jsonResult(results), nil
	})

	s.AddTool(mcp.NewTool("find_pokemon_by_filters",
		mcp.WithDescription("Discover owned Pokemon candidates by type, role, and speed tier."),
		mcp.WithString("type", mcp.Description("Filter by type (e.g. 'fire', 'steel')")),
		mcp.WithString("role", mcp.Description("'physical attacker'|'special attacker'|'mixed attacker'|'support'|'tank'")),
		mcp.WithString("speed_tier", mcp.Description("'fast' (>100 base)|'mid' (70-100)|'slow' (<70)")),
		mcp.WithBoolean("legendary", mcp.Description("Filter by legendary status")),
		mcp.WithBoolean("final_evo_only", mcp.Description("Only final evolutions")),
		mcp.WithBoolean("owned", mcp.Description("Default true; false to search all")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		f := pokemon.SpeciesFilter{
			Type:         getString(args, "type"),
			Role:         getString(args, "role"),
			SpeedTier:    getString(args, "speed_tier"),
			FinalEvoOnly: getBool(args, "final_evo_only"),
		}
		if v, ok := args["owned"]; ok && v != nil {
			b := getBool(args, "owned")
			f.Owned = &b
		}
		if v, ok := args["legendary"]; ok && v != nil {
			b := getBool(args, "legendary")
			f.Legendary = &b
		}
		results, err := handlers.FindPokemonByFilters(svc, f, getInt(args, "limit"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		if len(results) == 0 {
			return textResult("No owned Pokemon match those filters. Try broadening: remove role or speed_tier constraints."), nil
		}
		return jsonResult(results), nil
	})

	s.AddTool(mcp.NewTool("get_pokemon",
		mcp.WithDescription("Get full details for a Pokemon species including base stats, types, and available abilities."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Pokemon name (e.g. 'Garchomp')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := getString(req.GetArguments(), "name")
		sp, err := svc.Pokemon.GetSpeciesByName(name)
		if err != nil {
			return errResult(err.Error()), nil
		}
		abilities, _ := svc.Pokemon.GetAbilitiesForSpecies(sp.Slug)
		return jsonResult(map[string]any{"species": sp, "abilities": abilities}), nil
	})

	s.AddTool(mcp.NewTool("get_moves",
		mcp.WithDescription("Get the Champions-format learnset for a species (the curated legal move pool)."),
		mcp.WithString("pokemon_name", mcp.Required(), mcp.Description("Pokemon name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := getString(req.GetArguments(), "pokemon_name")
		sp, err := svc.Pokemon.GetSpeciesByName(name)
		if err != nil {
			return errResult(err.Error()), nil
		}
		moves, err := svc.Pokemon.GetChampionsLearnset(sp.Slug)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if len(moves) == 0 {
			return textResult(fmt.Sprintf("No Champions learnset data seeded for %s. The data is curated externally — surface this to the user and stop.", sp.Name)), nil
		}
		return jsonResult(moves), nil
	})

	s.AddTool(mcp.NewTool("search_moves",
		mcp.WithDescription("Search moves by name/description with optional type, category, power, and priority filters."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Name or description fragment")),
		mcp.WithString("type", mcp.Description("Filter by type (e.g. 'fire')")),
		mcp.WithString("category", mcp.Description("physical, special, or status")),
		mcp.WithNumber("min_power", mcp.Description("Minimum base power")),
		mcp.WithNumber("max_power", mcp.Description("Maximum base power")),
		mcp.WithNumber("min_accuracy", mcp.Description("Minimum accuracy")),
		mcp.WithNumber("priority", mcp.Description("Exact priority value")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		f := pokemon.MoveFilter{
			Type:        getString(args, "type"),
			Category:    getString(args, "category"),
			MinPower:    getInt(args, "min_power"),
			MaxPower:    getInt(args, "max_power"),
			MinAccuracy: getInt(args, "min_accuracy"),
		}
		if v, ok := args["priority"]; ok && v != nil {
			p := getInt(args, "priority")
			f.Priority = &p
		}
		moves, err := svc.Pokemon.SearchMoves(getString(args, "query"), getInt(args, "limit"), f)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return jsonResult(moves), nil
	})

	s.AddTool(mcp.NewTool("get_item",
		mcp.WithDescription("Get details for a held item by name."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Item name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		item, err := svc.Pokemon.GetItemByName(getString(req.GetArguments(), "name"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		return jsonResult(item), nil
	})

	s.AddTool(mcp.NewTool("search_items",
		mcp.WithDescription("Search held items by name or effect, optionally filtered by owned or banned status."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search term")),
		mcp.WithBoolean("owned", mcp.Description("Filter to owned/unowned items")),
		mcp.WithBoolean("banned", mcp.Description("Filter to banned/legal items")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		f := pokemon.ItemFilter{}
		if v, ok := args["owned"]; ok && v != nil {
			b := getBool(args, "owned")
			f.Owned = &b
		}
		if v, ok := args["banned"]; ok && v != nil {
			b := getBool(args, "banned")
			f.Banned = &b
		}
		items, err := svc.Pokemon.SearchItems(getString(args, "query"), getInt(args, "limit"), f)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return jsonResult(items), nil
	})

	s.AddTool(mcp.NewTool("set_owned",
		mcp.WithDescription("Mark a Pokemon or item as owned (or unowned)."),
		mcp.WithString("type", mcp.Required(), mcp.Description("'pokemon' or 'item'")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Species or item name")),
		mcp.WithBoolean("owned", mcp.Required(), mcp.Description("true to mark owned, false to unmark")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		msg, err := handlers.SetOwned(svc, getString(args, "type"), getString(args, "name"), getBool(args, "owned"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(msg), nil
	})
}
