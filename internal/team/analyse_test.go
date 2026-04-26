package team_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

func TestAnalyse_SpeedTiersAndTeamID(t *testing.T) {
	pr, tr := newTeamDB(t)

	id, err := tr.CreateTeam("Speed Test", "H")
	require.NoError(t, err)

	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	_, err = tr.AddMember(int(id), ttar.ID, ss.ID)
	require.NoError(t, err)

	sylveon, err := pr.GetSpeciesByName("Sylveon")
	require.NoError(t, err)
	pixilate, err := pr.GetAbilityByName("Pixilate")
	require.NoError(t, err)
	_, err = tr.AddMember(int(id), sylveon.ID, pixilate.ID)
	require.NoError(t, err)

	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)

	analysis := team.Analyse(loaded)
	assert.Equal(t, int(id), analysis.TeamID)
	assert.NotEmpty(t, analysis.SpeedTiers)
	assert.Len(t, analysis.SpeedTiers, 2)
}

func TestAnalyse_SandTeamArchetype(t *testing.T) {
	ttar := &pokemon.Species{
		ID: 1, Name: "Tyranitar", IsFinalEvo: true,
		HP: 100, Attack: 134, Defense: 110, SpAttack: 95, SpDefense: 100, Speed: 61,
	}
	sandStreamAbility := &pokemon.Ability{ID: 1, Name: "Sand Stream"}

	t.Run("balance fallback when single setter", func(t *testing.T) {
		tr := &team.Team{
			ID:   10,
			Name: "Sand",
			Members: []team.Member{
				{Slot: 1, Species: ttar, Ability: sandStreamAbility, Nature: "Adamant"},
			},
		}
		analysis := team.Analyse(tr)
		assert.NotEmpty(t, analysis.Archetypes)
	})
}

func TestAnalyse_ArchetypeDetected_TrickRoom(t *testing.T) {
	trMove := &pokemon.Move{ID: 1, Name: "Trick Room", Type: pokemon.TypePsychic, Category: pokemon.CategoryStatus}
	sp := &pokemon.Species{ID: 1, Name: "Slowbro", IsFinalEvo: true, Speed: 30, HP: 95, Attack: 75, Defense: 110, SpAttack: 100, SpDefense: 80}

	t.Run("trick room detected", func(t *testing.T) {
		tr := &team.Team{
			ID:   5,
			Name: "TR",
			Members: []team.Member{
				{Slot: 1, Species: sp, Nature: "Serious", Moves: []*pokemon.Move{trMove}},
			},
		}
		analysis := team.Analyse(tr)
		assert.Contains(t, analysis.Archetypes, "Trick Room")
	})
}
