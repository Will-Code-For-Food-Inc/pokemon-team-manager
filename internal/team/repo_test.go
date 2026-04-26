package team_test

import (
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

	id, err := tr.CreateTeam("My Team", "H")
	require.NoError(t, err)
	assert.Greater(t, int(id), 0)
}

func TestListTeams(t *testing.T) {
	_, tr := newTeamDB(t)

	_, err := tr.CreateTeam("Team A", "H")
	require.NoError(t, err)
	_, err = tr.CreateTeam("Team B", "H")
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

	id, err := tr.CreateTeam("Get Test", "H")
	require.NoError(t, err)

	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	assert.Equal(t, "Get Test", loaded.Name)
	assert.Equal(t, "H", loaded.Regulation)
	assert.Empty(t, loaded.Members)
}

func TestAddMember(t *testing.T) {
	pr, tr := newTeamDB(t)

	id, err := tr.CreateTeam("Add Test", "H")
	require.NoError(t, err)

	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)

	memberID, err := tr.AddMember(int(id), ttar.ID, ss.ID)
	require.NoError(t, err)
	assert.Greater(t, int(memberID), 0)

	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	require.Len(t, loaded.Members, 1)
	assert.Equal(t, "Tyranitar", loaded.Members[0].Species.Name)
}

func TestSetNature(t *testing.T) {
	pr, tr := newTeamDB(t)

	id, err := tr.CreateTeam("Nature Test", "H")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	memberID, err := tr.AddMember(int(id), ttar.ID, ss.ID)
	require.NoError(t, err)

	require.NoError(t, tr.SetNature(int(memberID), "Adamant"))

	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	assert.Equal(t, "Adamant", loaded.Members[0].Nature)
}

func TestSetItem(t *testing.T) {
	pr, tr := newTeamDB(t)

	id, err := tr.CreateTeam("Item Test", "H")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	memberID, err := tr.AddMember(int(id), ttar.ID, ss.ID)
	require.NoError(t, err)

	leftovers, err := pr.GetItemByName("Leftovers")
	require.NoError(t, err)
	require.NoError(t, tr.SetItem(int(memberID), leftovers.ID))

	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	require.NotNil(t, loaded.Members[0].Item)
	assert.Equal(t, "Leftovers", loaded.Members[0].Item.Name)
}

func TestSetMoves(t *testing.T) {
	pr, tr := newTeamDB(t)

	id, err := tr.CreateTeam("Move Test", "H")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	memberID, err := tr.AddMember(int(id), ttar.ID, ss.ID)
	require.NoError(t, err)

	crunch, err := pr.GetMoveByName("Crunch")
	require.NoError(t, err)
	protect, err := pr.GetMoveByName("Protect")
	require.NoError(t, err)

	require.NoError(t, tr.SetMoves(int(memberID), []int{crunch.ID, protect.ID}))

	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	assert.Len(t, loaded.Members[0].Moves, 2)
	moveNames := []string{loaded.Members[0].Moves[0].Name, loaded.Members[0].Moves[1].Name}
	assert.Contains(t, moveNames, "Crunch")
	assert.Contains(t, moveNames, "Protect")
}

func TestSetEVs(t *testing.T) {
	pr, tr := newTeamDB(t)

	id, err := tr.CreateTeam("EV Test", "H")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	memberID, err := tr.AddMember(int(id), ttar.ID, ss.ID)
	require.NoError(t, err)

	evs := team.StatSpread{HP: 32, Atk: 32, Def: 2}
	require.NoError(t, tr.SetEVs(int(memberID), evs))

	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	assert.Equal(t, 32, loaded.Members[0].EVs.HP)
	assert.Equal(t, 32, loaded.Members[0].EVs.Atk)
	assert.Equal(t, 2, loaded.Members[0].EVs.Def)
}

func TestSetRole(t *testing.T) {
	pr, tr := newTeamDB(t)

	id, err := tr.CreateTeam("Role Test", "H")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	memberID, err := tr.AddMember(int(id), ttar.ID, ss.ID)
	require.NoError(t, err)

	require.NoError(t, tr.SetRole(int(memberID), "lead"))

	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	assert.Equal(t, "lead", loaded.Members[0].Role)
}

func TestSetMemberNotes(t *testing.T) {
	pr, tr := newTeamDB(t)

	id, err := tr.CreateTeam("Notes Test", "H")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	memberID, err := tr.AddMember(int(id), ttar.ID, ss.ID)
	require.NoError(t, err)

	require.NoError(t, tr.SetMemberNotes(int(memberID), "core setter"))

	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	assert.Equal(t, "core setter", loaded.Members[0].Notes)
}

func TestSetAbility(t *testing.T) {
	pr, tr := newTeamDB(t)

	id, err := tr.CreateTeam("Ability Test", "H")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	memberID, err := tr.AddMember(int(id), ttar.ID, ss.ID)
	require.NoError(t, err)

	pixilate, err := pr.GetAbilityByName("Pixilate")
	require.NoError(t, err)

	require.NoError(t, tr.SetAbility(int(memberID), pixilate.ID))

	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	require.NotNil(t, loaded.Members[0].Ability)
	assert.Equal(t, "Pixilate", loaded.Members[0].Ability.Name)
}

func TestGetTeamNotesHTML(t *testing.T) {
	_, tr := newTeamDB(t)

	id, err := tr.CreateTeam("Notes Test", "H")
	require.NoError(t, err)

	require.NoError(t, tr.UpdateTeamNotes(int(id), "# Hello\n\nSome **bold** text."))

	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)
	assert.Equal(t, "# Hello\n\nSome **bold** text.", loaded.Notes)
	assert.Contains(t, loaded.NotesHTML, "<h1>")
	assert.Contains(t, loaded.NotesHTML, "<strong>")
}
