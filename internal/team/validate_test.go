package team_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

func TestValidate_EmptyTeam(t *testing.T) {
	tr := &team.Team{ID: 1, Name: "Test", Regulation: "H"}
	vs := team.Validate(tr, nil, nil)
	assert.Len(t, vs, 1)
	assert.Equal(t, "team_size", vs[0].Rule)
}

func TestValidate_DuplicateSpecies(t *testing.T) {
	sp := &pokemon.Species{ID: 1, Name: "Garchomp", IsFinalEvo: true}
	tr := &team.Team{
		ID:         1,
		Regulation: "H",
		Members: makeMembers(6, sp),
	}
	vs := team.Validate(tr, nil, nil)
	// Should flag duplicate species (5 duplicates of Garchomp)
	rules := violationRules(vs)
	assert.Contains(t, rules, "species_clause")
}

func TestValidate_LegalTeam(t *testing.T) {
	members := make([]team.Member, 6)
	for i := range members {
		members[i] = team.Member{
			Slot:    i + 1,
			Species: &pokemon.Species{ID: i + 1, Name: "Pokemon", IsFinalEvo: true},
			Nature:  "Serious",
			EVs: team.StatSpread{HP: 32, Spe: 32, Def: 2},
		}
	}
	tr := &team.Team{ID: 1, Regulation: "H", Members: members}
	vs := team.Validate(tr, nil, nil)
	assert.Empty(t, vs)
}

func TestValidate_EVOverLimit(t *testing.T) {
	sp := &pokemon.Species{ID: 1, Name: "Garchomp", IsFinalEvo: true}
	tr := &team.Team{
		ID:         1,
		Regulation: "H",
		Members: []team.Member{{
			Slot: 1, Species: sp, Nature: "Serious",
			EVs: team.StatSpread{HP: 32, Atk: 32, Def: 32}, // 756 total
		}},
	}
	vs := team.Validate(tr, nil, nil)
	rules := violationRules(vs)
	assert.Contains(t, rules, "stat_total")
	assert.Contains(t, rules, "team_size") // only 1 member
}

func TestStatSpread_Total(t *testing.T) {
	s := team.StatSpread{HP: 32, Atk: 32, Spe: 2}
	assert.Equal(t, 66, s.Total())
}

func TestDefenseMultiplier_SuperEffective(t *testing.T) {
	// Fire vs Grass should be ×2 (20 in ×10 scale)
	mult := team.DefenseMultiplier(pokemon.TypeFire, []pokemon.Type{pokemon.TypeGrass})
	assert.Equal(t, 20, mult)
}

func TestDefenseMultiplier_Immune(t *testing.T) {
	// Electric vs Ground is immune
	mult := team.DefenseMultiplier(pokemon.TypeElectric, []pokemon.Type{pokemon.TypeGround})
	assert.Equal(t, 0, mult)
}

// --- helpers ---

func makeMembers(n int, sp *pokemon.Species) []team.Member {
	m := make([]team.Member, n)
	for i := range m {
		m[i] = team.Member{
			Slot:    i + 1,
			Species: sp,
			Nature:  "Serious",
			EVs: team.StatSpread{HP: 32, Spe: 32, Def: 2},
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
	item := &pokemon.Item{ID: 1, Name: "Life Orb"}
	members := make([]team.Member, 6)
	for i := range members {
		members[i] = team.Member{
			Slot:    i + 1,
			Species: &pokemon.Species{ID: i + 1, Name: fmt.Sprintf("Mon%d", i+1), IsFinalEvo: true},
			Nature:  "Serious",
			EVs:     team.StatSpread{HP: 32, Spe: 32, Def: 2},
		}
	}
	// Two mons holding the same item
	members[0].Item = item
	members[1].Item = item
	tr := &team.Team{ID: 1, Regulation: "H", Members: members}
	vs := team.Validate(tr, nil, nil)
	assert.Contains(t, violationRules(vs), "item_clause")
}

func TestValidate_PerStatCap(t *testing.T) {
	members := makeMembers(6, &pokemon.Species{ID: 1, Name: "Garchomp", IsFinalEvo: true})
	members[0].EVs = team.StatSpread{HP: 33} // over 32 cap
	tr := &team.Team{ID: 1, Regulation: "H", Members: members}
	vs := team.Validate(tr, nil, nil)
	assert.Contains(t, violationRules(vs), "stat_range")
}

func TestValidate_RemovedNaturesRejected(t *testing.T) {
	// Hardy, Docile, Bashful, Quirky removed in Pokemon Champions — only Serious is neutral
	for _, nature := range []string{"Hardy", "Docile", "Bashful", "Quirky"} {
		assert.False(t, pokemon.ValidNature(nature), "%s should not be a valid Stat Alignment", nature)
	}
	assert.True(t, pokemon.ValidNature("Serious"), "Serious should be the only neutral Stat Alignment")
}

func TestValidate_BannedItem(t *testing.T) {
	bannedItem := &pokemon.Item{ID: 99, Name: "Soul Dew", IsBanned: true}
	members := makeMembers(6, &pokemon.Species{ID: 1, Name: "Latios", IsFinalEvo: true})
	members[0].Item = bannedItem
	tr := &team.Team{ID: 1, Regulation: "H", Members: members}
	vs := team.Validate(tr, nil, nil)
	assert.Contains(t, violationRules(vs), "banned_item")
}

func TestValidate_BannedMove(t *testing.T) {
	bannedMoveID := 999
	reg := team.NewRegulation("H", "Regulation H", 0, nil, nil, []int{bannedMoveID})

	mv := &pokemon.Move{ID: bannedMoveID, Name: "Dark Void"}
	members := makeMembers(6, &pokemon.Species{ID: 1, Name: "Darkrai", IsFinalEvo: true})
	members[0].Moves = []*pokemon.Move{mv}
	tr := &team.Team{ID: 1, Regulation: "H", Members: members}
	vs := team.Validate(tr, reg, nil)
	assert.Contains(t, violationRules(vs), "banned_move")
}

// TestCalcStat_Champions verifies the formula against known in-game values from screenshots.
func TestCalcStat_Champions(t *testing.T) {
	tests := []struct {
		name       string
		base       int
		sp         int
		isHP       bool
		natureMult float64
		want       int
	}{
		// Sylveon HP 95 base, 32 SP, neutral → 202 (screenshot)
		{"Sylveon HP", 95, 32, true, 1.0, 202},
		// Sylveon SpA 110 base, 32 SP, Modest +10% → 178 (screenshot)
		{"Sylveon SpA Modest", 110, 32, false, 1.1, 178},
		// Aggron Def 180 base, 32 SP, Impish +10% → 255 (screenshot)
		{"Aggron Def Impish", 180, 32, false, 1.1, 255},
		// Arcanine HP 90 base, 32 SP, neutral → 197 (screenshot)
		{"Arcanine HP", 90, 32, true, 1.0, 197},
		// Tyranitar Atk 134 base, 32 SP, Jolly (neutral on Atk) → 186 (screenshot)
		{"Tyranitar Atk", 134, 32, false, 1.0, 186},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := team.CalcStat(tc.base, tc.sp, tc.isHP, tc.natureMult)
			assert.Equal(t, tc.want, got, "base=%d sp=%d isHP=%v mult=%.1f", tc.base, tc.sp, tc.isHP, tc.natureMult)
		})
	}
}
