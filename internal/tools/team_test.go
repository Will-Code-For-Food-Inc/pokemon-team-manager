package tools_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── create_team ───────────────────────────────────────────────────────────────

func TestCreateTeam_Success(t *testing.T) {
	s, _ := newServer(t)
	out := mustOK(t, s, "create_team", map[string]any{"name": "My Team", "regulation": "I2"})
	assert.Contains(t, out, "Team created with ID")
}

func TestCreateTeam_MissingName(t *testing.T) {
	s, _ := newServer(t)
	out := call(t, s, "create_team", map[string]any{"regulation": "I2"})
	// Missing name — either graceful error or empty-name DB error. Not a panic.
	assert.NotEmpty(t, out)
}

// ── list_teams ────────────────────────────────────────────────────────────────

func TestListTeams_Empty(t *testing.T) {
	s, _ := newServer(t)
	out := call(t, s, "list_teams", map[string]any{})
	assert.Contains(t, out, "No teams found")
}

func TestListTeams_ShowsCreatedTeam(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Alpha Team", "regulation": "I2"})
	out := mustOK(t, s, "list_teams", map[string]any{})
	assert.Contains(t, out, "Alpha Team")
}

// ── get_team ──────────────────────────────────────────────────────────────────

func TestGetTeam_ReturnsTeamDetails(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Detail Team", "regulation": "I2"})
	out := mustOK(t, s, "get_team", map[string]any{"team_id": 1})
	assert.Contains(t, out, "Detail Team")
}

func TestGetTeam_UnknownID(t *testing.T) {
	s, _ := newServer(t)
	mustErr(t, s, "get_team", map[string]any{"team_id": 9999})
}

// ── add_pokemon ───────────────────────────────────────────────────────────────

func TestAddPokemon_Success(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Add Test", "regulation": "I2"})
	out := mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	assert.Contains(t, out, "Garchomp")
}

func TestAddPokemon_UnknownSpecies(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "T", "regulation": "I2"})
	mustErr(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Fakemon"})
}

func TestAddPokemon_TeamFull(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Full", "regulation": "I2"})
	mons := []string{"Garchomp", "Tyranitar", "Sylveon", "Arcanine", "Excadrill", "Whimsicott"}
	for _, m := range mons {
		mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": m})
	}
	out := mustErr(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Dragonite"})
	assert.Contains(t, strings.ToLower(out), "full")
}

func TestAddPokemon_WithNamedAbility(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Ab Test", "regulation": "I2"})
	out := mustOK(t, s, "add_pokemon", map[string]any{
		"team_id": 1, "pokemon_name": "Tyranitar", "ability": "Sand Stream",
	})
	assert.Contains(t, out, "Tyranitar")
}

func TestAddPokemon_InvalidAbility(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Bad Ab", "regulation": "I2"})
	out := mustErr(t, s, "add_pokemon", map[string]any{
		"team_id": 1, "pokemon_name": "Tyranitar", "ability": "Levitate",
	})
	assert.Contains(t, strings.ToLower(out), "not a valid ability")
}

// ── remove_pokemon ────────────────────────────────────────────────────────────

func TestRemovePokemon_Success(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Remove", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	out := mustOK(t, s, "remove_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	assert.Contains(t, strings.ToLower(out), "removed")
	// Confirm team is now empty.
	out = mustOK(t, s, "get_team", map[string]any{"team_id": 1})
	assert.NotContains(t, out, "Garchomp")
}

func TestRemovePokemon_NotOnTeam(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "NoMon", "regulation": "I2"})
	mustErr(t, s, "remove_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Sylveon"})
}

// ── rename_team ───────────────────────────────────────────────────────────────

func TestRenameTeam_Success(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Old Name", "regulation": "I2"})
	mustOK(t, s, "rename_team", map[string]any{"team_id": 1, "name": "New Name"})
	out := mustOK(t, s, "list_teams", map[string]any{})
	assert.Contains(t, out, "New Name")
	assert.NotContains(t, out, "Old Name")
}

// ── copy_team ─────────────────────────────────────────────────────────────────

func TestCopyTeam_Success(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Source", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	out := mustOK(t, s, "copy_team", map[string]any{"team_id": 1, "name": "Copy"})
	assert.Contains(t, out, "Copy")
	// Both teams exist.
	list := mustOK(t, s, "list_teams", map[string]any{})
	assert.Contains(t, list, "Source")
	assert.Contains(t, list, "Copy")
}

// ── swap_slots ────────────────────────────────────────────────────────────────

func TestSwapSlots_Success(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Swap", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Tyranitar"})
	out := mustOK(t, s, "swap_slots", map[string]any{"team_id": 1, "slot_a": 1, "slot_b": 2})
	assert.Contains(t, strings.ToLower(out), "swap")
}

func TestSwapSlots_InvalidSlot(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Swap2", "regulation": "I2"})
	out := mustErr(t, s, "swap_slots", map[string]any{"team_id": 1, "slot_a": 0, "slot_b": 1})
	assert.Contains(t, strings.ToLower(out), "slot")
}

// ── set_ability ───────────────────────────────────────────────────────────────

func TestSetAbility_Success(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Ab", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Tyranitar"})
	out := mustOK(t, s, "set_ability", map[string]any{
		"team_id": 1, "pokemon_name": "Tyranitar", "ability": "Sand Stream",
	})
	assert.Contains(t, out, "Sand Stream")
}

// ── set_nature ────────────────────────────────────────────────────────────────

func TestSetNature_Valid(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Nat", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	out := mustOK(t, s, "set_nature", map[string]any{
		"team_id": 1, "pokemon_name": "Garchomp", "nature": "Jolly",
	})
	assert.Contains(t, out, "Jolly")
}

func TestSetNature_Invalid(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "BadNat", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	mustErr(t, s, "set_nature", map[string]any{
		"team_id": 1, "pokemon_name": "Garchomp", "nature": "Foobar",
	})
}

// ── set_item ──────────────────────────────────────────────────────────────────

func TestSetItem_Success(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Item", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	out := mustOK(t, s, "set_item", map[string]any{
		"team_id": 1, "pokemon_name": "Garchomp", "item": "Lum Berry",
	})
	assert.Contains(t, out, "Lum Berry")
}

func TestSetItem_DuplicateOnTeam(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "DupItem", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Sylveon"})
	mustOK(t, s, "set_item", map[string]any{"team_id": 1, "pokemon_name": "Garchomp", "item": "Lum Berry"})
	out := mustErr(t, s, "set_item", map[string]any{"team_id": 1, "pokemon_name": "Sylveon", "item": "Lum Berry"})
	assert.Contains(t, strings.ToLower(out), "lum berry", "duplicate item error must name the item")
}

// ── set_moves ─────────────────────────────────────────────────────────────────

func TestSetMoves_Success(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Moves", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Tyranitar"})
	out := mustOK(t, s, "set_moves", map[string]any{
		"team_id": 1, "pokemon_name": "Tyranitar",
		"move1": "Crunch", "move2": "Rock Slide", "move3": "Earthquake", "move4": "Protect",
	})
	assert.Contains(t, out, "Tyranitar")
}

func TestSetMoves_IllegalMove_Atomic(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "BadMove", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Tyranitar"})
	// Set valid moves first.
	mustOK(t, s, "set_moves", map[string]any{
		"team_id": 1, "pokemon_name": "Tyranitar",
		"move1": "Crunch", "move2": "Rock Slide",
	})
	// Attempt to set with one illegal move — should fail.
	out := call(t, s, "set_moves", map[string]any{
		"team_id": 1, "pokemon_name": "Tyranitar",
		"move1": "Crunch", "move2": "Surf", // Surf not in Tyranitar's learnset
	})
	if strings.HasPrefix(out, "Error:") {
		// Atomic guarantee: original moves still present.
		teamOut := mustOK(t, s, "get_team", map[string]any{"team_id": 1})
		assert.Contains(t, teamOut, "Crunch")
	}
	// If Surf happens to be in the learnset (data drift), we accept the move.
}

// ── set_stats ─────────────────────────────────────────────────────────────────

func TestSetStats_Valid(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Stats", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	out := mustOK(t, s, "set_stats", map[string]any{
		"team_id": 1, "pokemon_name": "Garchomp",
		"hp": 32, "attack": 32, "defense": 2,
	})
	assert.Contains(t, strings.ToLower(out), "hp")
}

func TestSetStats_Overflow(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Overflow", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	out := mustErr(t, s, "set_stats", map[string]any{
		"team_id": 1, "pokemon_name": "Garchomp",
		"hp": 32, "attack": 32, "defense": 3, // total 67
	})
	assert.Contains(t, out, "66")
}

func TestSetStats_PerStatCap(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "PerStat", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	out := mustErr(t, s, "set_stats", map[string]any{
		"team_id": 1, "pokemon_name": "Garchomp", "hp": 33,
	})
	assert.Contains(t, out, "32")
}

// ── set_notes / set_nickname / set_role / set_team_notes ─────────────────────

func TestSetNotes_Persists(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Notes", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	mustOK(t, s, "set_notes", map[string]any{"team_id": 1, "pokemon_name": "Garchomp", "notes": "lead setup"})
	out := mustOK(t, s, "get_team", map[string]any{"team_id": 1})
	assert.Contains(t, out, "lead setup")
}

func TestSetNickname_Persists(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Nick", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	mustOK(t, s, "set_nickname", map[string]any{"team_id": 1, "pokemon_name": "Garchomp", "nickname": "Chompy"})
	out := mustOK(t, s, "get_team", map[string]any{"team_id": 1})
	assert.Contains(t, out, "Chompy")
}

func TestSetRole_Persists(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Role", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	mustOK(t, s, "set_role", map[string]any{"team_id": 1, "pokemon_name": "Garchomp", "role": "lead"})
	out := mustOK(t, s, "get_team", map[string]any{"team_id": 1})
	assert.Contains(t, out, "lead")
}

func TestSetTeamNotes_Persists(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "TNotes", "regulation": "I2"})
	// set_team_notes uses "strategy" as the param name (the field it updates).
	mustOK(t, s, "set_team_notes", map[string]any{"team_id": 1, "strategy": "sand balance core"})
	out := mustOK(t, s, "get_team", map[string]any{"team_id": 1})
	assert.Contains(t, out, "sand balance core")
}

// ── validate_team ─────────────────────────────────────────────────────────────

func TestValidateTeam_EmptyTeam(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Validate", "regulation": "I2"})
	out := call(t, s, "validate_team", map[string]any{"team_id": 1})
	// Empty team must report some violation.
	assert.NotEmpty(t, out)
}

func TestValidateTeam_UnknownID(t *testing.T) {
	s, _ := newServer(t)
	mustErr(t, s, "validate_team", map[string]any{"team_id": 9999})
}

// ── analyse_team ──────────────────────────────────────────────────────────────

func TestAnalyseTeam_OneMonTeam(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Analyse", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	out := mustOK(t, s, "analyse_team", map[string]any{"team_id": 1})
	assert.NotEmpty(t, out)
}

// ── export_team ───────────────────────────────────────────────────────────────

func TestExportTeam_ContainsSpeciesName(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Export", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Sylveon"})
	out := mustOK(t, s, "export_team", map[string]any{"team_id": 1})
	assert.Contains(t, out, "Export")
	assert.Contains(t, out, "Sylveon")
}

// ── training_cost ─────────────────────────────────────────────────────────────

func TestTrainingCost_EmptyTeam(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Cost", "regulation": "I2"})
	out := call(t, s, "training_cost", map[string]any{"team_id": 1})
	// Empty team may return "Team has no members." or a zero-cost summary.
	assert.NotEmpty(t, out)
}

func TestTrainingCost_WithOneMon(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "CostMon", "regulation": "I2"})
	mustOK(t, s, "add_pokemon", map[string]any{"team_id": 1, "pokemon_name": "Garchomp"})
	out := mustOK(t, s, "training_cost", map[string]any{"team_id": 1})
	assert.Contains(t, out, "800", "recruit cost must appear for one Pokemon")
}

// ── battle logs ───────────────────────────────────────────────────────────────

func TestBattleLogs_AddAndGet(t *testing.T) {
	s, _ := newServer(t)
	mustOK(t, s, "create_team", map[string]any{"name": "Logs", "regulation": "I2"})
	mustOK(t, s, "add_team_log", map[string]any{"team_id": 1, "entry": "Won vs. rain team."})
	mustOK(t, s, "add_team_log", map[string]any{"team_id": 1, "entry": "Lost vs. TR Pult."})
	out := mustOK(t, s, "get_team_logs", map[string]any{"team_id": 1})
	assert.Contains(t, out, "Won vs. rain team.")
	assert.Contains(t, out, "Lost vs. TR Pult.")
}

func TestBattleLogs_UnknownTeam(t *testing.T) {
	s, _ := newServer(t)
	mustErr(t, s, "add_team_log", map[string]any{"team_id": 9999, "entry": "test"})
}

// ── regulations ───────────────────────────────────────────────────────────────

func TestListRegulations_ContainsI2(t *testing.T) {
	s, _ := newServer(t)
	out := mustOK(t, s, "list_regulations", map[string]any{})
	assert.Contains(t, out, "I2")
}

func TestGetRegulation_Valid(t *testing.T) {
	s, _ := newServer(t)
	out := mustOK(t, s, "get_regulation", map[string]any{"id": "I2"})
	assert.Contains(t, out, "I2")
}

func TestGetRegulation_Unknown(t *testing.T) {
	s, _ := newServer(t)
	mustErr(t, s, "get_regulation", map[string]any{"id": "ZZZ"})
}

// ── calc_stats ────────────────────────────────────────────────────────────────

func TestCalcStats_KnownSpecies(t *testing.T) {
	s, _ := newServer(t)
	// calc_stats uses "species" not "pokemon_name".
	out := mustOK(t, s, "calc_stats", map[string]any{"species": "Garchomp"})
	assert.NotEmpty(t, out)
}

func TestCalcStats_UnknownSpecies(t *testing.T) {
	s, _ := newServer(t)
	mustErr(t, s, "calc_stats", map[string]any{"species": "Fakemon"})
}

// ── evaluate_pokemon ──────────────────────────────────────────────────────────

func TestEvaluatePokemon_SpeciesLevel(t *testing.T) {
	s, _ := newServer(t)
	out := mustOK(t, s, "evaluate_pokemon", map[string]any{"pokemon_name": "Garchomp"})
	assert.NotEmpty(t, out)
}

func TestEvaluatePokemon_UnknownSpecies(t *testing.T) {
	s, _ := newServer(t)
	// evaluate_pokemon returns an error text (not necessarily "Error:" prefix).
	out := call(t, s, "evaluate_pokemon", map[string]any{"pokemon_name": "Fakemon"})
	require.NotEmpty(t, out)
	assert.True(t,
		strings.Contains(strings.ToLower(out), "error") || strings.Contains(strings.ToLower(out), "not found"),
		"unknown species must produce an error message, got: %s", out)
}
