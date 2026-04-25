package db_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/pokemon-team-manager/internal/db"
	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

// TestSeedAndTeam seeds the in-memory DB from data/ files and builds the Apr 24
// test team, verifying the full round-trip: seed → add members → validate.
func TestSeedAndTeam(t *testing.T) {
	sqlDB, err := db.OpenMemory()
	require.NoError(t, err)
	defer sqlDB.Close()

	require.NoError(t, db.Seed(sqlDB, "../../data"), "seed from data/")

	pr := pokemon.NewRepo(sqlDB)
	tr := team.NewRepo(sqlDB, pr)

	// --- Verify seed data ---
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	assert.Equal(t, 248, ttar.ID)
	assert.Equal(t, pokemon.Type("rock"), ttar.Type1)
	assert.True(t, ttar.IsFinalEvo)

	sylveon, err := pr.GetSpeciesByName("Sylveon")
	require.NoError(t, err)
	assert.Equal(t, 700, sylveon.ID)
	assert.Equal(t, pokemon.Type("fairy"), sylveon.Type1)

	// Pixilate must be a legal ability for Sylveon.
	pixilate, err := pr.GetAbilityByName("Pixilate")
	require.NoError(t, err)
	ok, err := pr.HasAbility(sylveon.ID, pixilate.ID)
	require.NoError(t, err)
	assert.True(t, ok, "Sylveon should have Pixilate")

	// Hyper Voice must be in Sylveon's learnset.
	hv, err := pr.GetMoveByName("Hyper Voice")
	require.NoError(t, err)
	ok, err = pr.CanLearnMove(sylveon.ID, hv.ID)
	require.NoError(t, err)
	assert.True(t, ok, "Sylveon should be able to learn Hyper Voice")

	// --- Build the Apr 24 team ---
	teamID, err := tr.CreateTeam("Apr 24 Sand Team", "H")
	require.NoError(t, err)

	addMember := func(speciesName, abilityName, nature, item string, moveNames []string, evStats []string) {
		t.Helper()
		sp, err := pr.GetSpeciesByName(speciesName)
		require.NoError(t, err, "species %s", speciesName)
		ab, err := pr.GetAbilityByName(abilityName)
		require.NoError(t, err, "ability %s for %s", abilityName, speciesName)

		memberID, err := tr.AddMember(int(teamID), sp.ID, ab.ID)
		require.NoError(t, err, "add %s", speciesName)
		mid := int(memberID)

		require.NoError(t, tr.SetNature(mid, nature))

		if item != "" {
			it, err := pr.GetItemByName(item)
			require.NoError(t, err, "item %s", item)
			require.NoError(t, tr.SetItem(mid, it.ID))
		}

		var moveIDs []int
		for _, mn := range moveNames {
			mv, err := pr.GetMoveByName(mn)
			require.NoError(t, err, "move %s on %s", mn, speciesName)
			moveIDs = append(moveIDs, mv.ID)
		}
		if len(moveIDs) > 0 {
			require.NoError(t, tr.SetMoves(mid, moveIDs))
		}

		evs := buildEVs(evStats)
		require.NoError(t, tr.SetEVs(mid, evs))
	}

	addMember("Tyranitar",  "Sand Stream", "Adamant", "Leftovers", []string{"Crunch", "Rock Slide", "Earthquake", "Protect"}, []string{"attack", "hp"})
	addMember("Sandaconda", "Sand Spit",   "Impish",  "",          []string{"Coil", "Body Press", "Glare", "Protect"},         []string{"defense", "hp"})
	addMember("Aggron",     "Sturdy",      "Relaxed", "Shuca Berry",[]string{"Iron Defense", "Body Press", "Heavy Slam", "Protect"}, []string{"defense", "hp"})
	addMember("Dragonite",  "Multiscale",  "Adamant", "",          []string{"Dragon Dance", "Extreme Speed", "Outrage", "Ice Punch"}, []string{"attack", "speed"})
	addMember("Arcanine",   "Intimidate",  "Timid",   "",          []string{"Flare Blitz", "Flamethrower", "Protect", "Extreme Speed"}, []string{"speed", "sp_attack"})
	addMember("Sylveon",    "Pixilate",    "Modest",  "Lum Berry", []string{"Hyper Voice", "Moonblast", "Calm Mind", "Psychic"},  []string{"hp", "sp_attack"})

	// --- Load and validate ---
	loaded, err := tr.GetTeam(int(teamID))
	require.NoError(t, err)
	assert.Len(t, loaded.Members, 6)

	reg, _ := tr.GetRegulation("H")
	violations := team.Validate(loaded, reg, pr)
	assert.Empty(t, violations, "Apr 24 team should be fully legal in Regulation H")

	// --- Spot-check members ---
	assert.Equal(t, "Tyranitar", loaded.Members[0].Species.Name)
	assert.Equal(t, "Sylveon", loaded.Members[5].Species.Name)
	assert.NotNil(t, loaded.Members[5].Item)
	assert.Equal(t, "Lum Berry", loaded.Members[5].Item.Name)
	assert.Equal(t, "Modest", loaded.Members[5].Nature)
	assert.Len(t, loaded.Members[5].Moves, 4)

	// --- Analyse ---
	analysis := team.Analyse(loaded)
	assert.Equal(t, int(teamID), analysis.TeamID)
	assert.NotEmpty(t, analysis.SpeedTiers)
}

func buildEVs(stats []string) team.StatSpread {
	var evs team.StatSpread
	vals := make([]int, len(stats))
	switch len(stats) {
	case 2:
		vals[0], vals[1] = 252, 252
	case 3:
		vals[0], vals[1], vals[2] = 172, 172, 164
	}
	for i, s := range stats {
		switch s {
		case "hp":
			evs.HP = vals[i]
		case "attack":
			evs.Atk = vals[i]
		case "defense":
			evs.Def = vals[i]
		case "sp_attack":
			evs.SpA = vals[i]
		case "sp_defense":
			evs.SpD = vals[i]
		case "speed":
			evs.Spe = vals[i]
		}
	}
	return evs
}
