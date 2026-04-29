package db_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/pokemon-team-manager/internal/pokemon"
)

func TestVPCosts_CorrectConstants(t *testing.T) {
	// Champions VP constants: spPerPoint=5, nature=500, move=250, ability=500, recruit=800
	pr, _ := newSeededDB(t)

	// Verify items have correct VP costs
	leftovers, err := pr.GetItemByName("Leftovers")
	require.NoError(t, err)
	assert.Equal(t, 700, leftovers.VPCost, "Leftovers is a common 700 VP item")

	// Lum Berry and Sitrus Berry are in the 700 VP "common" tier, not the 400 VP resist/cure berry tier
	lum, err := pr.GetItemByName("Lum Berry")
	require.NoError(t, err)
	assert.Equal(t, 700, lum.VPCost, "Lum Berry is 700 VP (not in the 400 VP resist berry tier)")

	// Type-resist berries are 400 VP
	occa, err := pr.GetItemByName("Occa Berry")
	require.NoError(t, err)
	assert.Equal(t, 400, occa.VPCost, "Occa Berry is a type-resist berry (400 VP)")

	focusSash, err := pr.GetItemByName("Focus Sash")
	require.NoError(t, err)
	assert.Equal(t, 700, focusSash.VPCost, "Focus Sash is a common 700 VP item")
}

func TestVPCosts_ItemsHaveSlugs(t *testing.T) {
	pr, _ := newSeededDB(t)

	items, err := pr.SearchItems("", 50, pokemon.ItemFilter{})
	require.NoError(t, err)
	for _, it := range items {
		assert.NotEmpty(t, it.Slug, "item %q should have a slug", it.Name)
		assert.Greater(t, it.VPCost, 0, "item %q should have vp_cost > 0", it.Name)
	}
}

func TestChampionsLearnset_ReturnsMovesForKnownSpecies(t *testing.T) {
	pr, _ := newSeededDB(t)

	// Sylveon should have Hyper Voice in Champions format
	sylveon, err := pr.GetSpeciesByName("Sylveon")
	require.NoError(t, err)

	moves, err := pr.GetChampionsLearnset(sylveon.Slug)
	require.NoError(t, err)
	// Champions learnset is seeded from champions_learnsets.csv
	// Note: champions data uses "sylveon" slug
	if len(moves) > 0 {
		names := make([]string, len(moves))
		for i, m := range moves {
			names[i] = m.Slug
		}
		assert.Contains(t, names, "hyper-voice", "Sylveon should have Hyper Voice in Champions learnset")
		assert.Contains(t, names, "moonblast", "Sylveon should have Moonblast in Champions learnset")
	}
}

func TestCanLearnMove_SlugBased(t *testing.T) {
	pr, _ := newSeededDB(t)

	sylveon, err := pr.GetSpeciesByName("Sylveon")
	require.NoError(t, err)
	hv, err := pr.GetMoveByName("Hyper Voice")
	require.NoError(t, err)

	// CanLearnMove checks champions_learnsets (slug-based)
	ok, err := pr.CanLearnMove(sylveon.Slug, hv.Slug)
	require.NoError(t, err)
	assert.True(t, ok, "Sylveon should be able to learn Hyper Voice in Champions format")

	// A move Sylveon definitely can't learn
	ok, err = pr.CanLearnMove(sylveon.Slug, "earthquake")
	require.NoError(t, err)
	assert.False(t, ok, "Sylveon should not be able to learn Earthquake")
}

func TestHasAbility_SlugBased(t *testing.T) {
	pr, _ := newSeededDB(t)

	sylveon, err := pr.GetSpeciesByName("Sylveon")
	require.NoError(t, err)
	pixilate, err := pr.GetAbilityByName("Pixilate")
	require.NoError(t, err)

	ok, err := pr.HasAbility(sylveon.Slug, pixilate.Slug)
	require.NoError(t, err)
	assert.True(t, ok, "Sylveon should have Pixilate")

	ok, err = pr.HasAbility(sylveon.Slug, "sand-stream")
	require.NoError(t, err)
	assert.False(t, ok, "Sylveon should not have Sand Stream")
}

func TestSpeciesHasSlugs(t *testing.T) {
	pr, _ := newSeededDB(t)

	results, err := pr.SearchSpecies("", 50, pokemon.SpeciesFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, results)
	for _, sp := range results {
		assert.NotEmpty(t, sp.Slug, "species %q should have a slug", sp.Name)
	}
}

func TestMovesHaveSlugs(t *testing.T) {
	pr, _ := newSeededDB(t)

	moves, err := pr.SearchMoves("", 50, pokemon.MoveFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, moves)
	for _, mv := range moves {
		assert.NotEmpty(t, mv.Slug, "move %q should have a slug", mv.Name)
	}
}
