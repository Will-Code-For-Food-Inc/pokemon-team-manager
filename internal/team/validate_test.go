package team_test

import (
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
			Nature:  "Hardy",
			EVs:     team.StatSpread{HP: 252, Spe: 252, Def: 4},
			IVs:     team.StatSpread{HP: 31, Atk: 31, Def: 31, SpA: 31, SpD: 31, Spe: 31},
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
			Slot: 1, Species: sp, Nature: "Hardy",
			EVs: team.StatSpread{HP: 252, Atk: 252, Def: 252}, // 756 total
			IVs: team.StatSpread{HP: 31, Atk: 31, Def: 31, SpA: 31, SpD: 31, Spe: 31},
		}},
	}
	vs := team.Validate(tr, nil, nil)
	rules := violationRules(vs)
	assert.Contains(t, rules, "ev_total")
	assert.Contains(t, rules, "team_size") // only 1 member
}

func TestStatSpread_Total(t *testing.T) {
	s := team.StatSpread{HP: 252, Atk: 252, Spe: 4}
	assert.Equal(t, 508, s.Total())
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
			Nature:  "Hardy",
			EVs:     team.StatSpread{HP: 252, Spe: 252, Def: 4},
			IVs:     team.StatSpread{HP: 31, Atk: 31, Def: 31, SpA: 31, SpD: 31, Spe: 31},
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
