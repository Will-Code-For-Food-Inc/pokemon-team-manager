package team_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

func cfg(sp *pokemon.Species, nature string, evs team.StatSpread) *team.Config {
	return &team.Config{Species: sp, Nature: nature, EVs: evs}
}

func cfgWithItem(sp *pokemon.Species, nature string, evs team.StatSpread, item *pokemon.Item) *team.Config {
	return &team.Config{Species: sp, Nature: nature, EVs: evs, Item: item}
}

func cfgWithMoves(sp *pokemon.Species, nature string, evs team.StatSpread, moves ...*pokemon.Move) *team.Config {
	c := &team.Config{Species: sp, Nature: nature, EVs: evs}
	c.Moves = moves
	return c
}

func TestValidate_EmptyTeam(t *testing.T) {
	tr := &team.Team{ID: 1, Name: "Test", Regulation: "I2"}
	vs := team.Validate(tr, nil, nil)
	assert.Len(t, vs, 1)
	assert.Equal(t, "team_size", vs[0].Rule)
}

func TestValidate_DuplicateSpecies(t *testing.T) {
	sp := &pokemon.Species{Slug: "garchomp", ID: 1, Name: "Garchomp", IsFinalEvo: true}
	tr := &team.Team{
		ID:         1,
		Regulation: "I2",
		Members:    makeMembers(6, sp),
	}
	vs := team.Validate(tr, nil, nil)
	rules := violationRules(vs)
	assert.Contains(t, rules, "species_clause")
}

func TestValidate_LegalTeam(t *testing.T) {
	members := make([]team.Member, 6)
	for i := range members {
		sp := &pokemon.Species{Slug: fmt.Sprintf("mon%d", i+1), ID: i + 1, Name: fmt.Sprintf("Pokemon%d", i+1), IsFinalEvo: true}
		// Each member needs a unique item to avoid item_clause; non-Serious
		// nature and a fully-allocated 66 SP spread to clear the
		// build-completeness heuristics.
		item := &pokemon.Item{Slug: fmt.Sprintf("item%d", i+1), ID: i + 1, Name: fmt.Sprintf("Item%d", i+1)}
		members[i] = team.Member{
			Slot:   i + 1,
			Config: cfgWithItem(sp, "Timid", team.StatSpread{HP: 32, Spe: 32, Def: 2}, item),
		}
	}
	tr := &team.Team{ID: 1, Regulation: "I2", Members: members}
	vs := team.Validate(tr, nil, nil)
	assert.Empty(t, vs)
}

func TestValidate_EVOverLimit(t *testing.T) {
	sp := &pokemon.Species{Slug: "garchomp", ID: 1, Name: "Garchomp", IsFinalEvo: true}
	tr := &team.Team{
		ID:         1,
		Regulation: "I2",
		Members: []team.Member{{
			Slot:   1,
			Config: cfg(sp, "Serious", team.StatSpread{HP: 32, Atk: 32, Def: 32}),
		}},
	}
	vs := team.Validate(tr, nil, nil)
	rules := violationRules(vs)
	assert.Contains(t, rules, "stat_total")
	assert.Contains(t, rules, "team_size")
}

func TestStatSpread_Total(t *testing.T) {
	s := team.StatSpread{HP: 32, Atk: 32, Spe: 2}
	assert.Equal(t, 66, s.Total())
}

func TestDefenseMultiplier_SuperEffective(t *testing.T) {
	mult := team.DefenseMultiplier(pokemon.TypeFire, []pokemon.Type{pokemon.TypeGrass})
	assert.Equal(t, 20, mult)
}

func TestDefenseMultiplier_Immune(t *testing.T) {
	mult := team.DefenseMultiplier(pokemon.TypeElectric, []pokemon.Type{pokemon.TypeGround})
	assert.Equal(t, 0, mult)
}

func makeMembers(n int, sp *pokemon.Species) []team.Member {
	m := make([]team.Member, n)
	for i := range m {
		m[i] = team.Member{
			Slot:   i + 1,
			Config: cfg(sp, "Serious", team.StatSpread{HP: 32, Spe: 32, Def: 2}),
		}
	}
	return m
}

func violationRules(vs []team.Violation) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.Rule
	}
	return out
}

func TestValidate_ItemClause(t *testing.T) {
	item := &pokemon.Item{Slug: "life-orb", ID: 1, Name: "Life Orb"}
	members := make([]team.Member, 6)
	for i := range members {
		sp := &pokemon.Species{Slug: fmt.Sprintf("mon%d", i+1), ID: i + 1, Name: fmt.Sprintf("Mon%d", i+1), IsFinalEvo: true}
		members[i] = team.Member{
			Slot:   i + 1,
			Config: cfg(sp, "Serious", team.StatSpread{HP: 32, Spe: 32, Def: 2}),
		}
	}
	members[0].Config = cfgWithItem(members[0].Config.Species, "Serious", team.StatSpread{HP: 32, Spe: 32, Def: 2}, item)
	members[1].Config = cfgWithItem(members[1].Config.Species, "Serious", team.StatSpread{HP: 32, Spe: 32, Def: 2}, item)
	tr := &team.Team{ID: 1, Regulation: "I2", Members: members}
	vs := team.Validate(tr, nil, nil)
	assert.Contains(t, violationRules(vs), "item_clause")
}

func TestValidate_PerStatCap(t *testing.T) {
	members := makeMembers(6, &pokemon.Species{Slug: "garchomp", ID: 1, Name: "Garchomp", IsFinalEvo: true})
	members[0].Config.EVs = team.StatSpread{HP: 33}
	tr := &team.Team{ID: 1, Regulation: "I2", Members: members}
	vs := team.Validate(tr, nil, nil)
	assert.Contains(t, violationRules(vs), "stat_range")
}

func TestValidate_RemovedNaturesRejected(t *testing.T) {
	for _, nature := range []string{"Hardy", "Docile", "Bashful", "Quirky"} {
		assert.False(t, pokemon.ValidNature(nature), "%s should not be a valid Stat Alignment", nature)
	}
	assert.True(t, pokemon.ValidNature("Serious"), "Serious should be the free neutral Stat Alignment")
}

func TestValidate_BannedItem(t *testing.T) {
	bannedItem := &pokemon.Item{Slug: "soul-dew", ID: 99, Name: "Soul Dew", IsBanned: true}
	members := makeMembers(6, &pokemon.Species{Slug: "latios", ID: 1, Name: "Latios", IsFinalEvo: true})
	members[0].Config = cfgWithItem(members[0].Config.Species, "Serious", team.StatSpread{HP: 32, Spe: 32, Def: 2}, bannedItem)
	tr := &team.Team{ID: 1, Regulation: "I2", Members: members}
	vs := team.Validate(tr, nil, nil)
	assert.Contains(t, violationRules(vs), "banned_item")
}

func TestValidate_BannedMove(t *testing.T) {
	reg := team.NewRegulation("H", "Regulation H", "", 0, nil, nil, []string{"dark-void"})
	mv := &pokemon.Move{Slug: "dark-void", ID: 999, Name: "Dark Void"}
	sp := &pokemon.Species{Slug: "darkrai", ID: 1, Name: "Darkrai", IsFinalEvo: true}
	members := makeMembers(6, sp)
	members[0].Config = cfgWithMoves(sp, "Serious", team.StatSpread{HP: 32, Spe: 32, Def: 2}, mv)
	tr := &team.Team{ID: 1, Regulation: "I2", Members: members}
	vs := team.Validate(tr, reg, nil)
	assert.Contains(t, violationRules(vs), "banned_move")
}

func TestCalcStat_Champions(t *testing.T) {
	tests := []struct {
		name       string
		base, sp   int
		isHP       bool
		natureMult float64
		want       int
	}{
		{"Sylveon HP", 95, 32, true, 1.0, 202},
		{"Sylveon SpA Modest", 110, 32, false, 1.1, 178},
		{"Aggron Def Impish", 180, 32, false, 1.1, 255},
		{"Arcanine HP", 90, 32, true, 1.0, 197},
		{"Tyranitar Atk", 134, 32, false, 1.0, 186},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := team.CalcStat(tc.base, tc.sp, tc.isHP, tc.natureMult)
			assert.Equal(t, tc.want, got)
		})
	}
}
