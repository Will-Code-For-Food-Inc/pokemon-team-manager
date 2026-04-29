package db_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/pokemon-team-manager/internal/db"
	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

func newSeededDB(t *testing.T) (*pokemon.Repo, *team.Repo) {
	t.Helper()
	sqlDB, err := db.OpenMemory()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.Seed(sqlDB, "../../data"))
	pr := pokemon.NewRepo(sqlDB)
	tr := team.NewRepo(sqlDB, pr)
	return pr, tr
}

func TestSeed_SpeciesPresent(t *testing.T) {
	pr, _ := newSeededDB(t)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	assert.Equal(t, pokemon.Type("rock"), ttar.Type1)
	assert.True(t, ttar.IsFinalEvo)
	sylveon, err := pr.GetSpeciesByName("Sylveon")
	require.NoError(t, err)
	assert.Equal(t, pokemon.Type("fairy"), sylveon.Type1)
}

func TestSeed_AbilityLearnset(t *testing.T) {
	pr, _ := newSeededDB(t)
	sylveon, err := pr.GetSpeciesByName("Sylveon")
	require.NoError(t, err)
	pixilate, err := pr.GetAbilityByName("Pixilate")
	require.NoError(t, err)
	ok, err := pr.HasAbility(sylveon.Slug, pixilate.Slug)
	require.NoError(t, err)
	assert.True(t, ok, "Sylveon should have Pixilate")
	hv, err := pr.GetMoveByName("Hyper Voice")
	require.NoError(t, err)
	ok, err = pr.CanLearnMove(sylveon.Slug, hv.Slug)
	require.NoError(t, err)
	assert.True(t, ok, "Sylveon should be able to learn Hyper Voice in Champions format")
}

func TestSeed_BuildAndValidateTeam(t *testing.T) {
	pr, tr := newSeededDB(t)
	teamID, err := tr.CreateTeam("Apr 24 Sand Team", "I2")
	require.NoError(t, err)

	addMember := func(speciesName, abilityName, nature, item string, moveNames []string, evStats []string) {
		t.Helper()
		sp, err := pr.GetSpeciesByName(speciesName)
		require.NoError(t, err, "species %s", speciesName)
		ab, err := pr.GetAbilityByName(abilityName)
		require.NoError(t, err, "ability %s for %s", abilityName, speciesName)
		configID, err := tr.AddMember(int(teamID), sp.Slug, ab.Slug)
		require.NoError(t, err, "add %s", speciesName)
		cid := int(configID)
		require.NoError(t, tr.SetNature(cid, nature))
		if item != "" {
			it, err := pr.GetItemByName(item)
			require.NoError(t, err, "item %s", item)
			require.NoError(t, tr.SetItem(cid, it.Slug))
		}
		var moveSlugs []string
		for _, mn := range moveNames {
			mv, err := pr.GetMoveByName(mn)
			require.NoError(t, err, "move %s on %s", mn, speciesName)
			moveSlugs = append(moveSlugs, mv.Slug)
		}
		if len(moveSlugs) > 0 {
			require.NoError(t, tr.SetMoves(cid, moveSlugs))
		}
		require.NoError(t, tr.SetEVs(cid, buildEVs(evStats)))
	}

	addMember("Tyranitar", "Sand Stream", "Adamant", "Leftovers", []string{"Crunch", "Rock Slide", "Earthquake", "Protect"}, []string{"attack", "hp"})
	addMember("Sandaconda", "Sand Spit", "Impish", "", []string{"Coil", "Body Press", "Glare", "Protect"}, []string{"defense", "hp"})
	addMember("Aggron", "Sturdy", "Relaxed", "Shuca Berry", []string{"Iron Defense", "Body Press", "Heavy Slam", "Protect"}, []string{"defense", "hp"})
	addMember("Dragonite", "Multiscale", "Adamant", "", []string{"Dragon Dance", "Extreme Speed", "Outrage", "Ice Punch"}, []string{"attack", "speed"})
	addMember("Arcanine", "Intimidate", "Timid", "", []string{"Flare Blitz", "Flamethrower", "Protect", "Extreme Speed"}, []string{"speed", "sp_attack"})
	addMember("Sylveon", "Pixilate", "Modest", "Lum Berry", []string{"Hyper Voice", "Moonblast", "Calm Mind", "Psychic"}, []string{"hp", "sp_attack"})

	loaded, err := tr.GetTeam(int(teamID))
	require.NoError(t, err)
	assert.Len(t, loaded.Members, 6)

	reg, _ := tr.GetRegulation("I2")
	violations := team.Validate(loaded, reg, pr)
	// Filter out build-completeness warnings — this test checks legality, not
	// whether every fixture-mon has a maxed SP spread + held item.
	completenessRules := map[string]bool{
		"incomplete_stats": true, "placeholder_nature": true, "missing_item": true,
	}
	var legalityIssues []team.Violation
	for _, v := range violations {
		if !completenessRules[v.Rule] {
			legalityIssues = append(legalityIssues, v)
		}
	}
	assert.Empty(t, legalityIssues, "team should be fully legal in Regulation I2")
}

func TestSeed_TeamMembersLoaded(t *testing.T) {
	pr, tr := newSeededDB(t)
	teamID, err := tr.CreateTeam("Load Test Team", "I2")
	require.NoError(t, err)
	sylveon, err := pr.GetSpeciesByName("Sylveon")
	require.NoError(t, err)
	pixilate, err := pr.GetAbilityByName("Pixilate")
	require.NoError(t, err)
	lumBerry, err := pr.GetItemByName("Lum Berry")
	require.NoError(t, err)
	configID, err := tr.AddMember(int(teamID), sylveon.Slug, pixilate.Slug)
	require.NoError(t, err)
	require.NoError(t, tr.SetNature(int(configID), "Modest"))
	require.NoError(t, tr.SetItem(int(configID), lumBerry.Slug))
	loaded, err := tr.GetTeam(int(teamID))
	require.NoError(t, err)
	require.Len(t, loaded.Members, 1)
	m := loaded.Members[0]
	assert.Equal(t, "Sylveon", m.Config.Species.Name)
	assert.Equal(t, "Modest", m.Config.Nature)
	require.NotNil(t, m.Config.Item)
	assert.Equal(t, "Lum Berry", m.Config.Item.Name)
}

func TestSeed_AnalyseTeam(t *testing.T) {
	pr, tr := newSeededDB(t)
	teamID, err := tr.CreateTeam("Analyse Test", "I2")
	require.NoError(t, err)
	ttar, err := pr.GetSpeciesByName("Tyranitar")
	require.NoError(t, err)
	ss, err := pr.GetAbilityByName("Sand Stream")
	require.NoError(t, err)
	_, err = tr.AddMember(int(teamID), ttar.Slug, ss.Slug)
	require.NoError(t, err)
	loaded, err := tr.GetTeam(int(teamID))
	require.NoError(t, err)
	analysis := team.Analyse(loaded)
	assert.Equal(t, int(teamID), analysis.TeamID)
	assert.NotEmpty(t, analysis.SpeedTiers)
}

func buildEVs(stats []string) team.StatSpread {
	var evs team.StatSpread
	vals := make([]int, len(stats))
	switch len(stats) {
	case 2:
		vals[0], vals[1] = 32, 32
	case 3:
		vals[0], vals[1], vals[2] = 32, 32, 2
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
