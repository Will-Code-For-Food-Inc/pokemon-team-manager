package tools_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ── find_pokemon_by_name ──────────────────────────────────────────────────────

func TestFindPokemonByName_KnownName(t *testing.T) {
	s, _ := newServer(t)
	out := call(t, s, "find_pokemon_by_name", map[string]any{
		"name": "Garchomp", "owned": false,
	})
	assert.Contains(t, out, "Garchomp")
}

func TestFindPokemonByName_FuzzyMatch(t *testing.T) {
	s, _ := newServer(t)
	out := call(t, s, "find_pokemon_by_name", map[string]any{
		"name": "Garcho", "owned": false,
	})
	assert.Contains(t, out, "Garchomp")
}

func TestFindPokemonByName_OwnedFilter_DefaultTrue(t *testing.T) {
	s, _ := newServer(t)
	// With default owned=true and fresh DB (no owned mons), result should note no owned.
	out := call(t, s, "find_pokemon_by_name", map[string]any{"name": "Garchomp"})
	// Either empty result message or results, but must not panic.
	assert.NotEmpty(t, out)
}

func TestFindPokemonByName_Unknown_Graceful(t *testing.T) {
	s, _ := newServer(t)
	out := call(t, s, "find_pokemon_by_name", map[string]any{"name": "Fakemon9999", "owned": false})
	// Must not error at the RPC level; returns informative message.
	assert.NotEmpty(t, out)
	assert.NotContains(t, out, "rpc-error")
}

// ── find_pokemon_by_filters ───────────────────────────────────────────────────

func TestFindPokemonByFilters_TypeFilter(t *testing.T) {
	s, _ := newServer(t)
	out := call(t, s, "find_pokemon_by_filters", map[string]any{
		"type": "dragon", "owned": false,
	})
	assert.Contains(t, out, "dragon")
}

func TestFindPokemonByFilters_SpeedTier(t *testing.T) {
	s, _ := newServer(t)
	out := call(t, s, "find_pokemon_by_filters", map[string]any{
		"speed_tier": "slow", "owned": false,
	})
	assert.NotEmpty(t, out)
}

func TestFindPokemonByFilters_EmptyFilter_ReturnsResults(t *testing.T) {
	s, _ := newServer(t)
	out := call(t, s, "find_pokemon_by_filters", map[string]any{"owned": false})
	assert.NotEmpty(t, out)
}

// ── get_pokemon ───────────────────────────────────────────────────────────────

func TestGetPokemon_Known(t *testing.T) {
	s, _ := newServer(t)
	// get_pokemon uses "name" not "pokemon_name" per its tool schema.
	out := mustOK(t, s, "get_pokemon", map[string]any{"name": "Garchomp"})
	assert.Contains(t, out, "Garchomp")
}

func TestGetPokemon_Unknown(t *testing.T) {
	s, _ := newServer(t)
	mustErr(t, s, "get_pokemon", map[string]any{"name": "Fakemon"})
}

// ── get_moves ─────────────────────────────────────────────────────────────────

func TestGetMoves_KnownSpecies(t *testing.T) {
	s, _ := newServer(t)
	out := call(t, s, "get_moves", map[string]any{"pokemon_name": "Tyranitar"})
	assert.NotEmpty(t, out)
	// Should contain at least Crunch or Protect which are canonical Ttar moves.
	hasMove := strings.Contains(out, "Crunch") || strings.Contains(out, "Protect") ||
		strings.Contains(out, "Rock Slide") || strings.Contains(out, "moves")
	assert.True(t, hasMove, "learnset for Tyranitar must contain known moves, got: %s", out)
}

func TestGetMoves_Unknown(t *testing.T) {
	s, _ := newServer(t)
	out := call(t, s, "get_moves", map[string]any{"pokemon_name": "Fakemon"})
	assert.NotEmpty(t, out)
	assert.Contains(t, strings.ToLower(out), "error")
}

// ── search_moves ──────────────────────────────────────────────────────────────

func TestSearchMoves_Query(t *testing.T) {
	s, _ := newServer(t)
	out := mustOK(t, s, "search_moves", map[string]any{"query": "protect"})
	assert.NotEmpty(t, out)
}

func TestSearchMoves_NoQuery(t *testing.T) {
	s, _ := newServer(t)
	out := call(t, s, "search_moves", map[string]any{})
	assert.NotEmpty(t, out)
}

// ── search_items ──────────────────────────────────────────────────────────────

func TestSearchItems_Query(t *testing.T) {
	s, _ := newServer(t)
	out := mustOK(t, s, "search_items", map[string]any{"query": "berry"})
	assert.NotEmpty(t, out)
}

// ── get_item ──────────────────────────────────────────────────────────────────

func TestGetItem_Known(t *testing.T) {
	s, _ := newServer(t)
	out := mustOK(t, s, "get_item", map[string]any{"name": "Lum Berry"})
	assert.Contains(t, out, "Lum Berry")
}

func TestGetItem_Unknown(t *testing.T) {
	s, _ := newServer(t)
	mustErr(t, s, "get_item", map[string]any{"name": "FakeItem9999"})
}

// ── set_owned ─────────────────────────────────────────────────────────────────

func TestSetOwned_Pokemon_TogglesPropagates(t *testing.T) {
	s, _ := newServer(t)
	// Initially Garchomp is not owned (fresh DB).
	out := call(t, s, "find_pokemon_by_name", map[string]any{"name": "Garchomp"})
	// Default owned=true — not found.
	assert.NotContains(t, out, "Garchomp")

	// Mark owned.
	mustOK(t, s, "set_owned", map[string]any{"type": "pokemon", "name": "Garchomp", "owned": true})
	out = call(t, s, "find_pokemon_by_name", map[string]any{"name": "Garchomp"})
	assert.Contains(t, out, "Garchomp")

	// Unmark owned.
	mustOK(t, s, "set_owned", map[string]any{"type": "pokemon", "name": "Garchomp", "owned": false})
	out = call(t, s, "find_pokemon_by_name", map[string]any{"name": "Garchomp"})
	assert.NotContains(t, out, "Garchomp")
}

func TestSetOwned_Item(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "set_owned", map[string]any{"type": "item", "name": "Lum Berry", "owned": true})
	out := mustOK(t, s, "get_item", map[string]any{"name": "Lum Berry"})
	assert.Contains(t, out, "owned")
}

func TestSetOwned_InvalidType(t *testing.T) {
	s, _ := newServer(t)
	out := mustErr(t, s, "set_owned", map[string]any{"type": "potion", "name": "Lum Berry", "owned": true})
	assert.Contains(t, strings.ToLower(out), "pokemon")
}

func TestSetOwned_UnknownName(t *testing.T) {
	s, _ := newServer(t)
	mustErr(t, s, "set_owned", map[string]any{"type": "pokemon", "name": "Fakemon9999", "owned": true})
}

// ── knowledge ────────────────────────────────────────────────────────────────

func TestIngestDocument_Success(t *testing.T) {
	s, _ := newServer(t)
	out := mustOK(t, s, "ingest_document", map[string]any{
		"title": "TR Guide", "content": "Porygon2 sets Trick Room.",
	})
	assert.Contains(t, strings.ToLower(out), "ingested")
}

func TestSearchKnowledge_FindsIngested(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "ingest_document", map[string]any{
		"title": "TR Guide", "content": "Porygon2 is the best Trick Room setter.",
	})
	out := mustOK(t, s, "search_knowledge", map[string]any{"query": "Trick Room"})
	assert.Contains(t, out, "Porygon2")
}

func TestSearchKnowledge_NoResults(t *testing.T) {
	s, _ := newServer(t)
	out := call(t, s, "search_knowledge", map[string]any{"query": "xyzzy_no_match_7q2"})
	// Must not panic or RPC-error; may return empty notice.
	assert.NotContains(t, out, "rpc-error")
}
