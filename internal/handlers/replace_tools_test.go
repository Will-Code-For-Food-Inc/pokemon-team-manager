package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/pokemon-team-manager/internal/db"
	"github.com/user/pokemon-team-manager/internal/knowledge"
	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

// newReplaceTestServices spins up an in-memory DB with seeded data and returns
// the Services + a one-mon team for replace_move tests. The mon has all 4
// move slots populated so we can assert the others stay untouched.
func newReplaceTestServices(t *testing.T) (*Services, int, string) {
	t.Helper()
	sqlDB, err := db.OpenMemory()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.Seed(sqlDB, "../../data"))

	pr := pokemon.NewRepo(sqlDB)
	tr := team.NewRepo(sqlDB, pr)
	kr := knowledge.NewRepo(sqlDB)
	svc := &Services{DB: sqlDB, Pokemon: pr, Team: tr, Knowledge: kr}

	teamID, err := tr.CreateTeam("Replace Move Test", "I2")
	require.NoError(t, err)

	ttar, _ := pr.GetSpeciesByName("Tyranitar")
	ss, _ := pr.GetAbilityByName("Sand Stream")
	configID, err := tr.AddMember(int(teamID), ttar.Slug, ss.Slug)
	require.NoError(t, err)

	// Populate 4 moves that are all in Tyranitar's Champions learnset.
	moves := []string{"Crunch", "Rock Slide", "Earthquake", "Protect"}
	slugs := make([]string, 0, 4)
	for _, n := range moves {
		mv, err := pr.GetMoveByName(n)
		require.NoError(t, err)
		ok, _ := pr.CanLearnMove(ttar.Slug, mv.Slug)
		require.True(t, ok, "seed data must include %s in Tyranitar's learnset for this test", n)
		slugs = append(slugs, mv.Slug)
	}
	require.NoError(t, tr.SetMoves(int(configID), slugs))
	return svc, int(teamID), "Tyranitar"
}

// findTool finds a tool by name in BuildPtmTools' output.
func findTool(t *testing.T, svc *Services, name string) AgentTool {
	t.Helper()
	for _, tool := range BuildPtmTools(svc) {
		if tool.Schema.Function.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not registered in BuildPtmTools", name)
	return AgentTool{}
}

func TestReplaceMove_PreservesOtherSlots(t *testing.T) {
	svc, teamID, pokeName := newReplaceTestServices(t)
	tool := findTool(t, svc, "replace_move")

	// Replace slot 2 (Rock Slide) with Stone Edge.
	out := tool.Execute(map[string]any{
		"team_id":       teamID,
		"pokemon_name":  pokeName,
		"slot":          2,
		"new_move_name": "Stone Edge",
	})
	assert.NotContains(t, out, "error", "replace_move should succeed: %s", out)

	loaded, err := svc.Team.GetTeam(teamID)
	require.NoError(t, err)
	require.Len(t, loaded.Members[0].Config.Moves, 4, "all 4 slots must remain populated")

	names := make([]string, 4)
	for i, mv := range loaded.Members[0].Config.Moves {
		names[i] = mv.Name
	}
	assert.Equal(t, "Crunch", names[0], "slot 1 must be untouched")
	assert.Equal(t, "Stone Edge", names[1], "slot 2 must be the new move")
	assert.Equal(t, "Earthquake", names[2], "slot 3 must be untouched")
	assert.Equal(t, "Protect", names[3], "slot 4 must be untouched")
}

func TestReplaceMove_RejectsOutOfRangeSlot(t *testing.T) {
	svc, teamID, pokeName := newReplaceTestServices(t)
	tool := findTool(t, svc, "replace_move")

	for _, slot := range []int{0, 5, -1, 99} {
		out := tool.Execute(map[string]any{
			"team_id":       teamID,
			"pokemon_name":  pokeName,
			"slot":          slot,
			"new_move_name": "Stone Edge",
		})
		assert.Contains(t, out, "1-4", "slot %d must be rejected with a 1-4 hint, got: %s", slot, out)
	}
}

func TestReplaceMove_RejectsMoveNotInLearnset(t *testing.T) {
	// Pick a move that is real (in the moves table) but NOT in Tyranitar's
	// curated Champions learnset. We discover such a move at runtime so this
	// test stays green if the seed data drifts.
	svc, teamID, pokeName := newReplaceTestServices(t)
	ttar, err := svc.Pokemon.GetSpeciesByName(pokeName)
	require.NoError(t, err)
	rows, err := svc.DB.Query(`
		SELECT name FROM moves
		WHERE slug NOT IN (SELECT move_slug FROM champions_learnsets WHERE species_slug=?)
		LIMIT 1`, ttar.Slug)
	require.NoError(t, err)
	defer rows.Close()
	require.True(t, rows.Next(), "seed data must contain at least one move outside Tyranitar's learnset")
	var illegalMoveName string
	require.NoError(t, rows.Scan(&illegalMoveName))
	rows.Close()

	tool := findTool(t, svc, "replace_move")
	out := tool.Execute(map[string]any{
		"team_id":       teamID,
		"pokemon_name":  pokeName,
		"slot":          1,
		"new_move_name": illegalMoveName,
	})
	assert.Contains(t, out, "learnset", "rejection message must mention learnset for %q, got: %s", illegalMoveName, out)

	// Confirm the original slot 1 (Crunch) is still there — no partial writes.
	loaded, err := svc.Team.GetTeam(teamID)
	require.NoError(t, err)
	require.Len(t, loaded.Members[0].Config.Moves, 4)
	assert.Equal(t, "Crunch", loaded.Members[0].Config.Moves[0].Name)
}

func TestReplacePokemon_AtomicSwap(t *testing.T) {
	// replace_pokemon should take a fully-configured slot, swap the species,
	// and present a fresh empty config under the same slot index.
	svc, teamID, _ := newReplaceTestServices(t)

	// Verify we start with a fully-built Tyranitar in slot 1.
	loaded, _ := svc.Team.GetTeam(teamID)
	require.Equal(t, "Tyranitar", loaded.Members[0].Config.Species.Name)
	require.Len(t, loaded.Members[0].Config.Moves, 4)

	tool := findTool(t, svc, "replace_pokemon")
	out := tool.Execute(map[string]any{
		"team_id":          teamID,
		"slot":             1,
		"new_pokemon_name": "Garchomp",
	})
	assert.NotContains(t, out, "error", "replace_pokemon should succeed: %s", out)

	loaded, _ = svc.Team.GetTeam(teamID)
	require.Len(t, loaded.Members, 1, "team should still have exactly one member")
	assert.Equal(t, "Garchomp", loaded.Members[0].Config.Species.Name)
	assert.Empty(t, loaded.Members[0].Config.Moves, "swap must leave a fresh empty config")
}

func TestReplacePokemon_RejectsOutOfRangeSlot(t *testing.T) {
	svc, teamID, _ := newReplaceTestServices(t)
	tool := findTool(t, svc, "replace_pokemon")

	for _, slot := range []int{0, 7, -1} {
		out := tool.Execute(map[string]any{
			"team_id":          teamID,
			"slot":             slot,
			"new_pokemon_name": "Garchomp",
		})
		assert.Contains(t, out, "1-6", "slot %d must be rejected with a 1-6 hint, got: %s", slot, out)
	}
}
