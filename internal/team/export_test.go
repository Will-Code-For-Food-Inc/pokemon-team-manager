package team_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

func TestExportMarkdown_ContainsTeamNameAndSpecies(t *testing.T) {
	pr, tr := newTeamDB(t)

	id, err := tr.CreateTeam("Export Test Team", "I2")
	require.NoError(t, err)

	species := []string{"Tyranitar", "Sylveon", "Arcanine"}
	abilities := []string{"Sand Stream", "Pixilate", "Intimidate"}
	for i, name := range species {
		sp, err := pr.GetSpeciesByName(name)
		require.NoError(t, err)
		ab, err := pr.GetAbilityByName(abilities[i])
		require.NoError(t, err)
		_, err = tr.AddMember(int(id), sp.Slug, ab.Slug)
		require.NoError(t, err)
	}

	loaded, err := tr.GetTeam(int(id))
	require.NoError(t, err)

	md := team.ExportMarkdown(loaded)
	assert.NotEmpty(t, md)
	assert.Contains(t, md, "Export Test Team")
	for _, name := range species {
		assert.Contains(t, md, name)
	}
}

func TestExportMarkdown_Minimal(t *testing.T) {
	sp := &pokemon.Species{Slug: "garchomp", ID: 1, Name: "Garchomp", IsFinalEvo: true,
		Type1: "dragon", HP: 108, Attack: 130, Defense: 95, SpAttack: 80, SpDefense: 85, Speed: 102}
	tr := &team.Team{
		ID:         1,
		Name:       "Minimal Team",
		Regulation: "I2",
		Members: []team.Member{
			{Slot: 1, Config: &team.Config{Species: sp, Nature: "Jolly"}},
		},
	}
	md := team.ExportMarkdown(tr)
	assert.True(t, len(md) > 0)
	assert.True(t, strings.Contains(md, "Minimal Team"))
	assert.True(t, strings.Contains(md, "Garchomp"))
}
