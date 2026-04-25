package team

import (
	"fmt"

	"github.com/user/pokemon-team-manager/internal/pokemon"
)

// typeChart maps (attackingType -> defendingType) to the damage multiplier × 10.
// Values: 20 = super effective (×2), 5 = not very effective (×0.5), 0 = immune (×0).
// Absent = normal (×1).
var typeChart = map[pokemon.Type]map[pokemon.Type]int{
	pokemon.TypeNormal:   {pokemon.TypeRock: 5, pokemon.TypeGhost: 0, pokemon.TypeSteel: 5},
	pokemon.TypeFire:     {pokemon.TypeFire: 5, pokemon.TypeWater: 5, pokemon.TypeGrass: 20, pokemon.TypeIce: 20, pokemon.TypeBug: 20, pokemon.TypeRock: 5, pokemon.TypeDragon: 5, pokemon.TypeSteel: 20},
	pokemon.TypeWater:    {pokemon.TypeFire: 20, pokemon.TypeWater: 5, pokemon.TypeGrass: 5, pokemon.TypeGround: 20, pokemon.TypeRock: 20, pokemon.TypeDragon: 5},
	pokemon.TypeElectric: {pokemon.TypeWater: 20, pokemon.TypeElectric: 5, pokemon.TypeGrass: 5, pokemon.TypeGround: 0, pokemon.TypeFlying: 20, pokemon.TypeDragon: 5},
	pokemon.TypeGrass:    {pokemon.TypeFire: 5, pokemon.TypeWater: 20, pokemon.TypeGrass: 5, pokemon.TypePoison: 5, pokemon.TypeGround: 20, pokemon.TypeFlying: 5, pokemon.TypeBug: 5, pokemon.TypeRock: 20, pokemon.TypeDragon: 5, pokemon.TypeSteel: 5},
	pokemon.TypeIce:      {pokemon.TypeFire: 5, pokemon.TypeWater: 5, pokemon.TypeGrass: 20, pokemon.TypeIce: 5, pokemon.TypeGround: 20, pokemon.TypeFlying: 20, pokemon.TypeDragon: 20, pokemon.TypeSteel: 5},
	pokemon.TypeFighting: {pokemon.TypeNormal: 20, pokemon.TypeIce: 20, pokemon.TypePoison: 5, pokemon.TypeFlying: 5, pokemon.TypePsychic: 5, pokemon.TypeBug: 5, pokemon.TypeRock: 20, pokemon.TypeGhost: 0, pokemon.TypeDark: 20, pokemon.TypeSteel: 20, pokemon.TypeFairy: 5},
	pokemon.TypePoison:   {pokemon.TypeGrass: 20, pokemon.TypePoison: 5, pokemon.TypeGround: 5, pokemon.TypeRock: 5, pokemon.TypeGhost: 5, pokemon.TypeSteel: 0, pokemon.TypeFairy: 20},
	pokemon.TypeGround:   {pokemon.TypeFire: 20, pokemon.TypeElectric: 20, pokemon.TypeGrass: 5, pokemon.TypePoison: 20, pokemon.TypeFlying: 0, pokemon.TypeBug: 5, pokemon.TypeRock: 20, pokemon.TypeSteel: 20},
	pokemon.TypeFlying:   {pokemon.TypeElectric: 5, pokemon.TypeGrass: 20, pokemon.TypeFighting: 20, pokemon.TypeBug: 20, pokemon.TypeRock: 5, pokemon.TypeSteel: 5},
	pokemon.TypePsychic:  {pokemon.TypeFighting: 20, pokemon.TypePoison: 20, pokemon.TypePsychic: 5, pokemon.TypeDark: 0, pokemon.TypeSteel: 5},
	pokemon.TypeBug:      {pokemon.TypeFire: 5, pokemon.TypeGrass: 20, pokemon.TypeFighting: 5, pokemon.TypePoison: 5, pokemon.TypeFlying: 5, pokemon.TypePsychic: 20, pokemon.TypeGhost: 5, pokemon.TypeDark: 20, pokemon.TypeSteel: 5, pokemon.TypeFairy: 5},
	pokemon.TypeRock:     {pokemon.TypeFire: 20, pokemon.TypeIce: 20, pokemon.TypeFighting: 5, pokemon.TypeGround: 5, pokemon.TypeFlying: 20, pokemon.TypeBug: 20, pokemon.TypeSteel: 5},
	pokemon.TypeGhost:    {pokemon.TypeNormal: 0, pokemon.TypePsychic: 20, pokemon.TypeGhost: 20, pokemon.TypeDark: 5},
	pokemon.TypeDragon:   {pokemon.TypeDragon: 20, pokemon.TypeSteel: 5, pokemon.TypeFairy: 0},
	pokemon.TypeDark:     {pokemon.TypeFighting: 5, pokemon.TypePsychic: 20, pokemon.TypeGhost: 20, pokemon.TypeDark: 5, pokemon.TypeFairy: 5},
	pokemon.TypeSteel:    {pokemon.TypeFire: 5, pokemon.TypeWater: 5, pokemon.TypeElectric: 5, pokemon.TypeIce: 20, pokemon.TypeRock: 20, pokemon.TypeSteel: 5, pokemon.TypeFairy: 20},
	pokemon.TypeFairy:    {pokemon.TypeFire: 5, pokemon.TypeFighting: 20, pokemon.TypePoison: 5, pokemon.TypeDragon: 20, pokemon.TypeDark: 20, pokemon.TypeSteel: 5},
}

// DefenseMultiplier returns the damage multiplier (×10) when a move of
// attackingType hits a Pokemon of the given types.
func DefenseMultiplier(attackingType pokemon.Type, defTypes []pokemon.Type) int {
	mult := 10
	for _, dt := range defTypes {
		row, ok := typeChart[attackingType]
		if !ok {
			continue
		}
		if v, ok := row[dt]; ok {
			mult = mult * v / 10
		}
	}
	return mult
}

// AllTypes is the list of all 18 Pokemon types.
var AllTypes = []pokemon.Type{
	pokemon.TypeNormal, pokemon.TypeFire, pokemon.TypeWater, pokemon.TypeElectric,
	pokemon.TypeGrass, pokemon.TypeIce, pokemon.TypeFighting, pokemon.TypePoison,
	pokemon.TypeGround, pokemon.TypeFlying, pokemon.TypePsychic, pokemon.TypeBug,
	pokemon.TypeRock, pokemon.TypeGhost, pokemon.TypeDragon, pokemon.TypeDark,
	pokemon.TypeSteel, pokemon.TypeFairy,
}

// Validate checks a fully-loaded Team against VGC rules and returns any
// violations. An empty slice means the team is legal.
//
// The repo parameter is used for learnset and ability legality checks.
// If repo is nil, those checks are skipped (useful for partial validation).
type RepoChecker interface {
	CanLearnMove(speciesID, moveID int) (bool, error)
	HasAbility(speciesID, abilityID int) (bool, error)
}

// Validate runs all VGC legality checks on t.
func Validate(t *Team, regulation *Regulation, checker RepoChecker) []Violation {
	var vs []Violation
	add := func(rule, msg string, args ...any) {
		vs = append(vs, Violation{Rule: rule, Message: fmt.Sprintf(msg, args...)})
	}

	// 1. Must have exactly 6 members.
	if len(t.Members) != 6 {
		add("team_size", "Team has %d Pokemon; exactly 6 are required", len(t.Members))
	}

	seenSpecies := map[int]bool{}
	seenItems := map[int]bool{}
	restrictedCount := 0

	for _, m := range t.Members {
		if m.Species == nil {
			add("missing_species", "Slot %d has no species assigned", m.Slot)
			continue
		}

		// 2. Species clause.
		if seenSpecies[m.Species.ID] {
			add("species_clause", "Duplicate species: %s appears more than once", m.Species.Name)
		}
		seenSpecies[m.Species.ID] = true

		// 3. Final evolution only.
		if !m.Species.IsFinalEvo {
			add("final_evo", "%s is not a fully-evolved Pokemon", m.Species.Name)
		}

		// 4. Banned species.
		if regulation != nil {
			if regulation.IsBanned(m.Species.ID) {
				add("banned_species", "%s is banned in Regulation %s", m.Species.Name, regulation.ID)
			}
			if m.Species.IsRestricted || regulation.IsRestricted(m.Species.ID) {
				restrictedCount++
			}
		}

		// 5. Item clause.
		if m.Item != nil {
			if seenItems[m.Item.ID] {
				add("item_clause", "Duplicate item: %s is held by more than one Pokemon", m.Item.Name)
			}
			seenItems[m.Item.ID] = true
			if m.Item.IsBanned {
				add("banned_item", "Item %s is not allowed in VGC", m.Item.Name)
			}
		}

		// 6. Ability legality.
		if m.Ability != nil && checker != nil {
			ok, err := checker.HasAbility(m.Species.ID, m.Ability.ID)
			if err == nil && !ok {
				add("illegal_ability", "%s cannot have ability %s", m.Species.Name, m.Ability.Name)
			}
		}

		// 7. Move learnset legality and banned moves.
		for _, mv := range m.Moves {
			if mv == nil {
				continue
			}
			if regulation != nil && regulation.IsMoveBanned(mv.ID) {
				add("banned_move", "Move %s is banned in Regulation %s", mv.Name, regulation.ID)
			}
			if checker != nil {
				ok, err := checker.CanLearnMove(m.Species.ID, mv.ID)
				if err == nil && !ok {
					add("illegal_move", "%s cannot learn %s", m.Species.Name, mv.Name)
				}
			}
		}

		// 8. Stat point validation (Pokemon Champions: 66 total, max 32/stat).
		evTotal := m.EVs.Total()
		if evTotal > 66 {
			add("stat_total", "%s has %d stat points; maximum is 66", m.Species.Name, evTotal)
		}
		for statName, val := range map[string]int{
			"HP": m.EVs.HP, "Atk": m.EVs.Atk, "Def": m.EVs.Def,
			"SpA": m.EVs.SpA, "SpD": m.EVs.SpD, "Spe": m.EVs.Spe,
		} {
			if val < 0 || val > 32 {
				add("stat_range", "%s %s is %d stat points; must be 0–32", m.Species.Name, statName, val)
			}
		}

		// 9. IV validation.
		for statName, val := range map[string]int{
			"HP": m.IVs.HP, "Atk": m.IVs.Atk, "Def": m.IVs.Def,
			"SpA": m.IVs.SpA, "SpD": m.IVs.SpD, "Spe": m.IVs.Spe,
		} {
			if val < 0 || val > 31 {
				add("iv_range", "%s %s IV is %d; must be 0–31", m.Species.Name, statName, val)
			}
		}

		// 10. Nature validity.
		if !pokemon.ValidNature(m.Nature) {
			add("invalid_nature", "%s has invalid nature %q", m.Species.Name, m.Nature)
		}
	}

	// 11. Restricted count.
	if regulation != nil && restrictedCount > regulation.MaxRestricted {
		add("restricted_count",
			"Team has %d restricted Pokemon; Regulation %s allows at most %d",
			restrictedCount, regulation.ID, regulation.MaxRestricted)
	}

	return vs
}

// Regulation holds the rules for a VGC regulation set.
type Regulation struct {
	ID            string
	Name          string
	MaxRestricted int
	bannedSpecies map[int]bool
	restricted    map[int]bool
	bannedMoves   map[int]bool
}

// NewRegulation constructs a Regulation from database-loaded data.
func NewRegulation(id, name string, maxRestricted int,
	bannedSpeciesIDs, restrictedSpeciesIDs, bannedMoveIDs []int) *Regulation {
	r := &Regulation{
		ID:            id,
		Name:          name,
		MaxRestricted: maxRestricted,
		bannedSpecies: make(map[int]bool),
		restricted:    make(map[int]bool),
		bannedMoves:   make(map[int]bool),
	}
	for _, id := range bannedSpeciesIDs {
		r.bannedSpecies[id] = true
	}
	for _, id := range restrictedSpeciesIDs {
		r.restricted[id] = true
	}
	for _, id := range bannedMoveIDs {
		r.bannedMoves[id] = true
	}
	return r
}

// IsBanned returns true if the species is banned in this regulation.
func (r *Regulation) IsBanned(speciesID int) bool { return r.bannedSpecies[speciesID] }

// IsRestricted returns true if the species is restricted in this regulation.
func (r *Regulation) IsRestricted(speciesID int) bool { return r.restricted[speciesID] }

// IsMoveBanned returns true if the move is banned in this regulation.
func (r *Regulation) IsMoveBanned(moveID int) bool { return r.bannedMoves[moveID] }
