package team_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/pokemon-team-manager/internal/db"
	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

func newTeamDB(t *testing.T) (*pokemon.Repo, *team.Repo) {
	t.Helper()
	sqlDB, err := db.OpenMemory()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.Seed(sqlDB, "../../data"))
	pr := pokemon.NewRepo(sqlDB)
	tr := team.NewRepo(sqlDB, pr)
	return pr, tr
}

func TestCreateTeam(t *testing.T) {
	_, tr := newTeamDB(t)
	id, err := tr.CreateTeam("My Team", "I2")
	require.NoError(t, err)
	assert.Greater(t, int(id), 0)
}

func TestListTeams(t *testing.T) {
	_, tr := newTeamDB(t)
	_, err := tr.CreateTeam("Team A", "I2")
	require.NoError(t, err)
	_, err = tr.CreateTeam("Team B", "I2")
	require.NoError(t, err)
	teams, err := tr.ListTeams()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(teams), 2)
	names := make([]string, len(teams))
	for i, s := range teams {
		names[i] = s.Name
	}
	assert.Contains(t, names, "Team A")
	assert.Contains(t, names, "Team B")
}

func TestGetTeam(t *testing.T) {
	_, tr := newTeamDB(t)
	id, err := tr.CreateTeam("Get Test", "I2")
	require.NoError(t, err)
	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	assert.Equal(t, "Get Test", loaded.Name)
	assert.Equal(t, "I2", loaded.Regulation)
	assert.Empty(t, loaded.Members)
}

func TestAddMember(t *testing.T) {
	pr, tr := newTeamDB(t)
	id, err := tr.CreateTeam("Add Test", "I2")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	configID, err := tr.AddMember(int(id), ttar.Slug, ss.Slug)
	require.NoError(t, err)
	assert.Greater(t, int(configID), 0)
	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	require.Len(t, loaded.Members, 1)
	assert.Equal(t, "Tyranitar", loaded.Members[0].Config.Species.Name)
}

func TestSetNature(t *testing.T) {
	pr, tr := newTeamDB(t)
	id, err := tr.CreateTeam("Nature Test", "I2")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	configID, err := tr.AddMember(int(id), ttar.Slug, ss.Slug)
	require.NoError(t, err)
	require.NoError(t, tr.SetNature(int(configID), "Adamant"))
	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	assert.Equal(t, "Adamant", loaded.Members[0].Config.Nature)
}

func TestSetItem(t *testing.T) {
	pr, tr := newTeamDB(t)
	id, err := tr.CreateTeam("Item Test", "I2")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	configID, err := tr.AddMember(int(id), ttar.Slug, ss.Slug)
	require.NoError(t, err)
	leftovers, err := pr.GetItemByName("Leftovers")
	require.NoError(t, err)
	require.NoError(t, tr.SetItem(int(configID), leftovers.Slug))
	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	require.NotNil(t, loaded.Members[0].Config.Item)
	assert.Equal(t, "Leftovers", loaded.Members[0].Config.Item.Name)
}

func TestSetMoves(t *testing.T) {
	pr, tr := newTeamDB(t)
	id, err := tr.CreateTeam("Move Test", "I2")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	configID, err := tr.AddMember(int(id), ttar.Slug, ss.Slug)
	require.NoError(t, err)
	crunch, err := pr.GetMoveByName("Crunch")
	require.NoError(t, err)
	protect, err := pr.GetMoveByName("Protect")
	require.NoError(t, err)
	require.NoError(t, tr.SetMoves(int(configID), []string{crunch.Slug, protect.Slug}))
	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	assert.Len(t, loaded.Members[0].Config.Moves, 2)
	moveNames := []string{loaded.Members[0].Config.Moves[0].Name, loaded.Members[0].Config.Moves[1].Name}
	assert.Contains(t, moveNames, "Crunch")
	assert.Contains(t, moveNames, "Protect")
}

func TestSetEVs(t *testing.T) {
	pr, tr := newTeamDB(t)
	id, err := tr.CreateTeam("EV Test", "I2")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	configID, err := tr.AddMember(int(id), ttar.Slug, ss.Slug)
	require.NoError(t, err)
	evs := team.StatSpread{HP: 32, Atk: 32, Def: 2}
	require.NoError(t, tr.SetEVs(int(configID), evs))
	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	assert.Equal(t, 32, loaded.Members[0].Config.EVs.HP)
	assert.Equal(t, 32, loaded.Members[0].Config.EVs.Atk)
	assert.Equal(t, 2, loaded.Members[0].Config.EVs.Def)
}

func TestSetRole(t *testing.T) {
	pr, tr := newTeamDB(t)
	id, err := tr.CreateTeam("Role Test", "I2")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	configID, err := tr.AddMember(int(id), ttar.Slug, ss.Slug)
	require.NoError(t, err)
	require.NoError(t, tr.SetRole(int(configID), "lead"))
	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	assert.Equal(t, "lead", loaded.Members[0].Config.Role)
}

func TestSetConfigNotes(t *testing.T) {
	pr, tr := newTeamDB(t)
	id, err := tr.CreateTeam("Notes Test", "I2")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	configID, err := tr.AddMember(int(id), ttar.Slug, ss.Slug)
	require.NoError(t, err)
	require.NoError(t, tr.SetConfigNotes(int(configID), "core setter"))
	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	assert.Equal(t, "core setter", loaded.Members[0].Config.Notes)
}

func TestSetAbility(t *testing.T) {
	pr, tr := newTeamDB(t)
	id, err := tr.CreateTeam("Ability Test", "I2")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	configID, err := tr.AddMember(int(id), ttar.Slug, ss.Slug)
	require.NoError(t, err)
	pixilate, err := pr.GetAbilityByName("Pixilate")
	require.NoError(t, err)
	require.NoError(t, tr.SetAbility(int(configID), pixilate.Slug))
	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	require.NotNil(t, loaded.Members[0].Config.Ability)
	assert.Equal(t, "Pixilate", loaded.Members[0].Config.Ability.Name)
}

func TestReplaceMember_PreservesSlot(t *testing.T) {
	// Build a 3-mon team, replace the middle slot, verify slot ordering and
	// that the old config is gone (cascades clean).
	pr, tr := newTeamDB(t)
	teamID, err := tr.CreateTeam("Replace Test", "I2")
	require.NoError(t, err)

	ttar, _ := pr.GetSpeciesByName("Tyranitar")
	ss, _ := pr.GetAbilityByName("Sand Stream")
	_, err = tr.AddMember(int(teamID), ttar.Slug, ss.Slug) // slot 1
	require.NoError(t, err)

	excad, _ := pr.GetSpeciesByName("Excadrill")
	sandRush, _ := pr.GetAbilityByName("Sand Rush")
	oldMidID, err := tr.AddMember(int(teamID), excad.Slug, sandRush.Slug) // slot 2
	require.NoError(t, err)

	whim, _ := pr.GetSpeciesByName("Whimsicott")
	prankster, _ := pr.GetAbilityByName("Prankster")
	_, err = tr.AddMember(int(teamID), whim.Slug, prankster.Slug) // slot 3
	require.NoError(t, err)

	// Give the soon-to-be-replaced mon some moves so we can confirm cascade.
	rockSlide, _ := pr.GetMoveByName("Rock Slide")
	require.NoError(t, tr.SetMoves(int(oldMidID), []string{rockSlide.Slug}))

	// Now replace slot 2 with Garchomp.
	garchomp, _ := pr.GetSpeciesByName("Garchomp")
	roughSkin, _ := pr.GetAbilityByName("Rough Skin")
	newID, err := tr.ReplaceMember(int(teamID), 2, garchomp.Slug, roughSkin.Slug)
	require.NoError(t, err)
	assert.NotEqual(t, int64(oldMidID), newID, "ReplaceMember must mint a fresh config_id")

	loaded, err := tr.GetTeam(int(teamID))
	require.NoError(t, err)
	require.Len(t, loaded.Members, 3)
	// Slot ordering must be preserved (1 Tyranitar, 2 Garchomp, 3 Whimsicott).
	assert.Equal(t, "Tyranitar", loaded.Members[0].Config.Species.Name)
	assert.Equal(t, "Garchomp", loaded.Members[1].Config.Species.Name)
	assert.Equal(t, "Whimsicott", loaded.Members[2].Config.Species.Name)
	// Replaced slot must have a fresh empty config — no inherited moves.
	assert.Empty(t, loaded.Members[1].Config.Moves, "new config must start with no moves")
	// Old config must be gone — GetConfig should fail.
	_, err = tr.GetConfig(int(oldMidID))
	assert.Error(t, err, "old config should have been deleted")
}

func TestReplaceMember_RejectsUnknownSlot(t *testing.T) {
	pr, tr := newTeamDB(t)
	teamID, _ := tr.CreateTeam("Empty Slot Test", "I2")
	garchomp, _ := pr.GetSpeciesByName("Garchomp")
	roughSkin, _ := pr.GetAbilityByName("Rough Skin")
	_, err := tr.ReplaceMember(int(teamID), 4, garchomp.Slug, roughSkin.Slug)
	assert.Error(t, err, "ReplaceMember on a slot with no member must error, not silently insert")
}

// ── Snapshot history ──────────────────────────────────────────────────────────

// initSnapshotTeam builds a one-mon team and returns (teamID, configID).
// Each test that needs history isolates with this helper to keep snapshot
// counts predictable.
func initSnapshotTeam(t *testing.T) (*pokemon.Repo, *team.Repo, int, int) {
	t.Helper()
	pr, tr := newTeamDB(t)
	teamID, err := tr.CreateTeam("Snap Test", "I2")
	require.NoError(t, err)
	ttar, _ := pr.GetSpeciesByName("Tyranitar")
	ss, _ := pr.GetAbilityByName("Sand Stream")
	configID, err := tr.AddMember(int(teamID), ttar.Slug, ss.Slug)
	require.NoError(t, err)
	return pr, tr, int(teamID), int(configID)
}

func TestSnapshot_AddMemberWritesOne(t *testing.T) {
	_, tr, teamID, _ := initSnapshotTeam(t)
	// AddMember should have produced exactly one mutation snapshot.
	n, err := tr.CountTeamSnapshots(teamID)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "AddMember must write exactly one snapshot")
	snaps, err := tr.ListTeamSnapshots(teamID, 10, 0)
	require.NoError(t, err)
	require.Len(t, snaps, 1)
	assert.Equal(t, "mutation", snaps[0].Kind)
	assert.Equal(t, "add_pokemon", snaps[0].Label)
}

func TestSnapshot_SetMovesWritesAdditional(t *testing.T) {
	pr, tr, teamID, configID := initSnapshotTeam(t)
	crunch, _ := pr.GetMoveByName("Crunch")
	require.NoError(t, tr.SetMoves(configID, []string{crunch.Slug}))
	n, _ := tr.CountTeamSnapshots(teamID)
	// AddMember (1) + SetMoves (1) = 2.
	assert.Equal(t, 2, n)
	snaps, _ := tr.ListTeamSnapshots(teamID, 10, 0)
	// Newest first.
	assert.Equal(t, "set_moves", snaps[0].Label)
}

func TestSnapshot_PayloadCapturesPriorState(t *testing.T) {
	// After SetMoves, the latest snapshot's payload must reflect the new
	// moves — not the empty pre-mutation state.
	pr, tr, _, configID := initSnapshotTeam(t)
	crunch, _ := pr.GetMoveByName("Crunch")
	protect, _ := pr.GetMoveByName("Protect")
	require.NoError(t, tr.SetMoves(configID, []string{crunch.Slug, protect.Slug}))

	snaps, _ := tr.ListTeamSnapshots(snapshotTeamID(t, tr), 10, 0)
	require.NotEmpty(t, snaps)
	// Decode the latest payload and confirm both moves are in slot 1's config.
	var snap team.Team
	require.NoError(t, json.Unmarshal([]byte(snaps[0].Payload), &snap))
	require.Len(t, snap.Members, 1)
	require.Len(t, snap.Members[0].Config.Moves, 2)
	names := []string{snap.Members[0].Config.Moves[0].Name, snap.Members[0].Config.Moves[1].Name}
	assert.Contains(t, names, "Crunch")
	assert.Contains(t, names, "Protect")
}

func snapshotTeamID(t *testing.T, tr *team.Repo) int {
	t.Helper()
	teams, err := tr.ListTeams()
	require.NoError(t, err)
	require.NotEmpty(t, teams)
	return teams[0].ID
}

func TestSnapshot_AutoPruneKeepsLastN(t *testing.T) {
	// Burst many small mutations and confirm the table never grows beyond
	// the auto-prune cap (50). We run 60 SetEVs cycles.
	_, tr, teamID, configID := initSnapshotTeam(t)
	for i := 0; i < 60; i++ {
		require.NoError(t, tr.SetEVs(configID, team.StatSpread{HP: i % 33}))
	}
	n, _ := tr.CountTeamSnapshots(teamID)
	// AddMember (1) + 60 SetEVs, but auto-prune caps mutation rows at 50.
	// The cap is on mutation kind only; AddMember is also mutation, so the
	// final count is exactly 50.
	assert.LessOrEqual(t, n, 50, "mutation snapshots must not exceed the cap")
	assert.Greater(t, n, 0)
}

func TestSnapshot_PruneIsUserDriven(t *testing.T) {
	// PruneTeamSnapshots is the user-only path. It must drop mutation rows
	// beyond the kept window.
	_, tr, teamID, configID := initSnapshotTeam(t)
	for i := 0; i < 10; i++ {
		require.NoError(t, tr.SetEVs(configID, team.StatSpread{HP: i}))
	}
	before, _ := tr.CountTeamSnapshots(teamID)
	dropped, err := tr.PruneTeamSnapshots(teamID, 3)
	require.NoError(t, err)
	after, _ := tr.CountTeamSnapshots(teamID)
	assert.Equal(t, before-int(dropped), after)
	assert.Equal(t, 3, after, "after prune-keep=3, exactly 3 mutation snapshots remain")
}

func TestSnapshot_CascadeOnTeamDelete(t *testing.T) {
	pr, tr, teamID, configID := initSnapshotTeam(t)
	crunch, _ := pr.GetMoveByName("Crunch")
	require.NoError(t, tr.SetMoves(configID, []string{crunch.Slug}))
	n, _ := tr.CountTeamSnapshots(teamID)
	require.Greater(t, n, 0)
	// Deleting the team must cascade-delete its snapshots.
	require.NoError(t, tr.DeleteTeam(teamID))
	n2, err := tr.CountTeamSnapshots(teamID)
	require.NoError(t, err)
	assert.Equal(t, 0, n2, "team delete must cascade and drop all snapshots for that team")
}

func TestGetTeamStrategyHTML(t *testing.T) {
	_, tr := newTeamDB(t)
	id, err := tr.CreateTeam("Strategy Test", "I2")
	require.NoError(t, err)
	require.NoError(t, tr.UpdateTeamStrategy(int(id), "# Hello\n\nSome **bold** text."))
	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	assert.Equal(t, "# Hello\n\nSome **bold** text.", loaded.Strategy)
	assert.Contains(t, loaded.StrategyHTML, "<h1>")
	assert.Contains(t, loaded.StrategyHTML, "<strong>")
}
