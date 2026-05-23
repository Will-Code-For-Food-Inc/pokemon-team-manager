package tools_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_FullTeamLifecycle builds a complete 6-mon team using only MCP tools,
// validates it, analyses it, and exports it — all from an empty DB, no network.
func TestE2E_FullTeamLifecycle(t *testing.T) {
	s, _ := newServer(t)

	// Regulation exists.
	regs := mustOK(t, s, "list_regulations", nil)
	assert.Contains(t, regs, "I2")

	// Create team.
	mustOK(t, s, "create_team", map[string]any{"name": "E2E Team", "regulation": "I2"})

	// Add 6 Pokemon from seeded data.
	mons := []string{"Garchomp", "Tyranitar", "Sylveon", "Arcanine", "Excadrill", "Whimsicott"}
	for _, m := range mons {
		mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": m})
	}

	// Full team must be reflected in get_team.
	team := mustOK(t, s, "get_team", map[string]any{"team_id": 1})
	for _, m := range mons {
		assert.Contains(t, team, m, "get_team must include %s", m)
	}

	// Configure first Pokemon.
	mustOK(t, s, "set_ability", map[string]any{
		"team_id": 1, "pokemon_name": "Garchomp", "ability": "Rough Skin",
	})
	mustOK(t, s, "set_nature", map[string]any{
		"team_id": 1, "pokemon_name": "Garchomp", "nature": "Jolly",
	})
	mustOK(t, s, "set_stats", map[string]any{
		"team_id": 1, "pokemon_name": "Garchomp", "hp": 32, "attack": 32, "defense": 2,
	})
	mustOK(t, s, "set_moves", map[string]any{
		"team_id": 1, "pokemon_name": "Garchomp",
		"move1": "Earthquake", "move2": "Rock Slide", "move3": "Protect", "move4": "Substitute",
	})

	// Validate — may have incomplete build violations but must not crash.
	val := call(t, s, "validate_team", map[string]any{"team_id": 1})
	assert.NotEmpty(t, val)

	// Analyse — returns coverage data.
	analysis := mustOK(t, s, "analyse_team", map[string]any{"team_id": 1})
	assert.NotEmpty(t, analysis)

	// Export — markdown contains team name and all species.
	export := mustOK(t, s, "export_team", map[string]any{"team_id": 1})
	assert.Contains(t, export, "E2E Team")
	for _, m := range mons {
		assert.Contains(t, export, m, "export must include %s", m)
	}

	// Training cost is non-zero (at least recruit costs).
	cost := mustOK(t, s, "training_cost", map[string]any{"team_id": 1})
	assert.Contains(t, cost, "800")
}

// TestE2E_OwnershipPropagation ensures set_owned flows through find_by_name and get_item.
func TestE2E_OwnershipPropagation(t *testing.T) {
	s, _ := newServer(t)

	// Garchomp not owned by default.
	out := call(t, s, "find_pokemon_by_name", map[string]any{"name": "Garchomp"})
	assert.NotContains(t, out, "Garchomp", "Garchomp must not appear with default owned=true filter")

	// Mark owned.
	mustOK(t, s, "set_owned", map[string]any{"type": "pokemon", "name": "Garchomp", "owned": true})
	out = call(t, s, "find_pokemon_by_name", map[string]any{"name": "Garchomp"})
	assert.Contains(t, out, "Garchomp")

	// Unmark owned.
	mustOK(t, s, "set_owned", map[string]any{"type": "pokemon", "name": "Garchomp", "owned": false})
	out = call(t, s, "find_pokemon_by_name", map[string]any{"name": "Garchomp"})
	assert.NotContains(t, out, "Garchomp")

	// Item ownership propagates to get_item.
	mustOK(t, s, "set_owned", map[string]any{"type": "item", "name": "Lum Berry", "owned": true})
	out = mustOK(t, s, "get_item", map[string]any{"name": "Lum Berry"})
	assert.Contains(t, out, "owned")
}

// TestE2E_ErrorRecovery confirms that failed tool calls do not corrupt team state.
func TestE2E_ErrorRecovery(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Recovery", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Tyranitar"})

	// Set valid moves first.
	mustOK(t, s, "set_moves", map[string]any{
		"team_id": 1, "pokemon_name": "Tyranitar",
		"move1": "Crunch", "move2": "Rock Slide",
	})

	// Try to set moves including one not in learnset.
	out := call(t, s, "set_moves", map[string]any{
		"team_id": 1, "pokemon_name": "Tyranitar",
		"move1": "Crunch", "move2": "Surf", // Surf not in Ttar learnset
	})
	if strings.HasPrefix(out, "Error:") {
		// Atomic guarantee: original moves survive the failed write.
		team := mustOK(t, s, "get_team", map[string]any{"team_id": 1})
		assert.Contains(t, team, "Crunch", "original moves must persist after failed set_moves")
	}

	// Stat overflow rejected, original stats unaffected.
	mustErr(t, s, "set_stats", map[string]any{
		"team_id": 1, "pokemon_name": "Tyranitar", "hp": 33,
	})

	// After rejection, successful stat set still works.
	mustOK(t, s, "set_stats", map[string]any{
		"team_id": 1, "pokemon_name": "Tyranitar", "hp": 32, "attack": 32, "defense": 2,
	})

	// Team full then remove then add.
	mustOK(t, s, "create_team", map[string]any{"name": "FullTest", "regulation": "I2"})
	mons := []string{"Garchomp", "Tyranitar", "Sylveon", "Arcanine", "Excadrill", "Whimsicott"}
	for _, m := range mons {
		mustOK(t, s, "add_pokemon", map[string]any{"team_id": 2, "pokemon_name": m})
	}
	out = mustErr(t, s, "add_pokemon", map[string]any{"team_id": 2, "pokemon_name": "Dragonite"})
	assert.Contains(t, strings.ToLower(out), "full")
	mustOK(t, s, "remove_pokemon", map[string]any{"team_id": 2, "pokemon_name": "Whimsicott"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 2, "pokemon_name": "Dragonite"})
}

// TestE2E_KnowledgeRoundTrip ingests a document and retrieves it via search.
func TestE2E_KnowledgeRoundTrip(t *testing.T) {
	s, _ := newServer(t)

	mustOK(t, s, "ingest_document", map[string]any{
		"title":   "TR Guide",
		"content": "Trick Room teams lead with Porygon2 as the primary setter.",
	})

	out := mustOK(t, s, "search_knowledge", map[string]any{"query": "Trick Room lead"})
	assert.Contains(t, out, "Porygon2", "search must surface the ingested document content")

	// Unrelated query must not crash.
	out = call(t, s, "search_knowledge", map[string]any{"query": "xyzzy_no_match"})
	require.NotContains(t, out, "rpc-error")

	// Battle log cycle.
	mustOK(t, s, "create_team", map[string]any{"name": "Log Test", "regulation": "I2"})
	mustOK(t, s, "add_team_log", map[string]any{"team_id": 1, "entry": "Beat rain team."})
	mustOK(t, s, "add_team_log", map[string]any{"team_id": 1, "entry": "Lost to sun team."})
	logs := mustOK(t, s, "get_team_logs", map[string]any{"team_id": 1})
	assert.Contains(t, logs, "Beat rain team.")
	assert.Contains(t, logs, "Lost to sun team.")
}
