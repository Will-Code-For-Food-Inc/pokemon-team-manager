package pokemon_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/pokemon-team-manager/internal/db"
	"github.com/user/pokemon-team-manager/internal/pokemon"
)

func newSeededRepo(t *testing.T) *pokemon.Repo {
	t.Helper()
	sqlDB, err := db.OpenMemory()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.Seed(sqlDB, "../../data"))
	return pokemon.NewRepo(sqlDB)
}

func TestGetSpeciesByName_ExactName_BaseForm(t *testing.T) {
	pr := newSeededRepo(t)
	// "Rotom" should resolve to base form (form='') even though every Rotom
	// form row shares name='Rotom'.
	sp, err := pr.GetSpeciesByName("Rotom")
	require.NoError(t, err)
	assert.Equal(t, "Rotom", sp.Name)
	assert.Empty(t, sp.Form, "ambiguous bare name must return base form")
	assert.Equal(t, "rotom", sp.Slug)
}

func TestGetSpeciesByName_HyphenatedForm(t *testing.T) {
	pr := newSeededRepo(t)
	// Real-world LLM input: "Rotom-Wash". Was failing with "species not found"
	// because lower(name)='rotom' doesn't match 'rotom-wash'.
	sp, err := pr.GetSpeciesByName("Rotom-Wash")
	require.NoError(t, err, "form Pokémon must resolve via slug normalisation")
	assert.Equal(t, "Rotom", sp.Name)
	assert.Equal(t, "Wash", sp.Form)
	assert.Equal(t, "rotom-wash", sp.Slug)
}

func TestGetSpeciesByName_SpaceSeparatedForm(t *testing.T) {
	pr := newSeededRepo(t)
	// "Rotom Wash" should normalise to slug 'rotom-wash'.
	sp, err := pr.GetSpeciesByName("Rotom Wash")
	require.NoError(t, err)
	assert.Equal(t, "rotom-wash", sp.Slug)
}

func TestGetSpeciesByName_CaseInsensitive(t *testing.T) {
	pr := newSeededRepo(t)
	for _, input := range []string{"GARCHOMP", "garchomp", "GarChomp"} {
		sp, err := pr.GetSpeciesByName(input)
		require.NoErrorf(t, err, "input %q must resolve regardless of case", input)
		assert.Equal(t, "Garchomp", sp.Name)
	}
}

func TestGetSpeciesByName_TrimsWhitespace(t *testing.T) {
	pr := newSeededRepo(t)
	sp, err := pr.GetSpeciesByName("  Garchomp  ")
	require.NoError(t, err)
	assert.Equal(t, "Garchomp", sp.Name)
}

func TestGetSpeciesByName_Empty(t *testing.T) {
	pr := newSeededRepo(t)
	_, err := pr.GetSpeciesByName("")
	assert.Error(t, err)
	_, err = pr.GetSpeciesByName("   ")
	assert.Error(t, err, "whitespace-only input must error, not silently match the first row")
}

func TestGetSpeciesByName_Unknown(t *testing.T) {
	pr := newSeededRepo(t)
	_, err := pr.GetSpeciesByName("NotARealPokemon")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
