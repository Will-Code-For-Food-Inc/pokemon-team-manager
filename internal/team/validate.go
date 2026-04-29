package team

import (
	"fmt"

	"github.com/user/pokemon-team-manager/internal/pokemon"
)

// typeChart maps (attackingType -> defendingType) to the damage multiplier × 10.
// Values: 20 = super effective (×2), 5 = not very effective (×0.5), 0 = immune (×0).
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
// attackingType hits a Pokemon of the given defending types.
func DefenseMultiplier(attackingType pokemon.Type, defTypes []pokemon.Type) int {
	mult := 10
	for _, dt := range defTypes {
		if row, ok := typeChart[attackingType]; ok {
			if v, ok := row[dt]; ok {
				mult = mult * v / 10
			}
		}
	}
	return mult
}

// AllTypes is the complete list of all 18 Pokemon types.
var AllTypes = []pokemon.Type{
	pokemon.TypeNormal, pokemon.TypeFire, pokemon.TypeWater, pokemon.TypeElectric,
	pokemon.TypeGrass, pokemon.TypeIce, pokemon.TypeFighting, pokemon.TypePoison,
	pokemon.TypeGround, pokemon.TypeFlying, pokemon.TypePsychic, pokemon.TypeBug,
	pokemon.TypeRock, pokemon.TypeGhost, pokemon.TypeDragon, pokemon.TypeDark,
	pokemon.TypeSteel, pokemon.TypeFairy,
}

// RepoChecker is the interface used by Validate for learnset and ability checks.
// Implementations use slug-based lookups against the champions_learnsets table.
type RepoChecker interface {
	CanLearnMove(speciesSlug, moveSlug string) (bool, error)
	HasAbility(speciesSlug, abilitySlug string) (bool, error)
}

// Validate checks a fully-loaded Team against VGC rules and returns any
// violations. An empty slice means the team is legal.
func Validate(t *Team, regulation *Regulation, checker RepoChecker) []Violation {
	var vs []Violation
	add := func(rule, msg string, args ...any) {
		vs = append(vs, Violation{Rule: rule, Message: fmt.Sprintf(msg, args...)})
	}

	if len(t.Members) != 6 {
		add("team_size", "Team has %d Pokemon; exactly 6 are required", len(t.Members))
	}

	seenSpecies := map[string]bool{}
	seenItems := map[string]bool{}
	restrictedCount := 0

	for _, m := range t.Members {
		c := m.Config
		if c == nil || c.Species == nil {
			add("missing_species", "Slot %d has no species assigned", m.Slot)
			continue
		}
		sp := c.Species

		// Species clause.
		if seenSpecies[sp.Slug] {
			add("species_clause", "Duplicate species: %s appears more than once", sp.Name)
		}
		seenSpecies[sp.Slug] = true

		// Regulation checks.
		if regulation != nil {
			if regulation.IsBanned(sp.Slug) {
				add("banned_species", "%s is banned in Regulation %s", sp.Name, regulation.ID)
			}
			if sp.IsRestricted || regulation.IsRestricted(sp.Slug) {
				restrictedCount++
			}
		}

		// Item clause.
		if c.Item != nil {
			if seenItems[c.Item.Slug] {
				add("item_clause", "Duplicate item: %s is held by more than one Pokemon", c.Item.Name)
			}
			seenItems[c.Item.Slug] = true
			if c.Item.IsBanned {
				add("banned_item", "Item %s is not allowed in VGC", c.Item.Name)
			}
		}

		// Ability legality.
		if c.Ability != nil && checker != nil {
			ok, err := checker.HasAbility(sp.Slug, c.Ability.Slug)
			if err == nil && !ok {
				add("illegal_ability", "%s cannot have ability %s", sp.Name, c.Ability.Name)
			}
		}

		// Move legality (Champions learnset) and regulation move bans.
		for _, mv := range c.Moves {
			if mv == nil {
				continue
			}
			if regulation != nil && regulation.IsMoveBanned(mv.Slug) {
				add("banned_move", "Move %s is banned in Regulation %s", mv.Name, regulation.ID)
			}
			if checker != nil {
				ok, err := checker.CanLearnMove(sp.Slug, mv.Slug)
				if err == nil && !ok {
					add("illegal_move", "%s cannot learn %s in Champions format", sp.Name, mv.Name)
				}
			}
		}

		// Stat point validation (66 total, max 32/stat).
		evTotal := c.EVs.Total()
		if evTotal > 66 {
			add("stat_total", "%s has %d stat points; maximum is 66", sp.Name, evTotal)
		}
		for statName, val := range map[string]int{
			"HP": c.EVs.HP, "Atk": c.EVs.Atk, "Def": c.EVs.Def,
			"SpA": c.EVs.SpA, "SpD": c.EVs.SpD, "Spe": c.EVs.Spe,
		} {
			if val < 0 || val > 32 {
				add("stat_range", "%s %s is %d stat points; must be 0–32", sp.Name, statName, val)
			}
		}

		// Nature validity.
		if !pokemon.ValidNature(c.Nature) {
			add("invalid_nature", "%s has invalid nature %q", sp.Name, c.Nature)
		}

		// Build-completeness heuristics. These aren't legality issues — the
		// team would still be tournament-legal — but they flag builds that
		// have placeholder/default values where real choices belong.
		if evTotal == 0 {
			add("incomplete_stats", "%s has 0 stat points allocated (build incomplete; budget is 66)", sp.Name)
		} else if evTotal < 66 {
			add("incomplete_stats", "%s has %d/66 stat points allocated (under-spent — %d points unused)", sp.Name, evTotal, 66-evTotal)
		}
		if c.Nature == "Serious" {
			add("placeholder_nature", "%s has Serious nature (placeholder — pick a real one with a +/- spread)", sp.Name)
		}
		if c.Item == nil {
			add("missing_item", "%s has no held item assigned", sp.Name)
		}
	}

	// Restricted Legendary count.
	if regulation != nil && restrictedCount > regulation.MaxRestricted {
		add("restricted_count",
			"Team has %d restricted Pokemon; Regulation %s allows at most %d",
			restrictedCount, regulation.ID, regulation.MaxRestricted)
	}

	return vs
}

// Regulation holds the rules for a VGC regulation set.
type Regulation struct {
	ID              string
	Name            string
	Description     string
	MaxRestricted   int
	BannedSlugs     []string
	RestrictedSlugs []string
	BannedMoveSlugs []string
	bannedSpecies   map[string]bool
	restricted      map[string]bool
	bannedMoves     map[string]bool
}

// NewRegulation constructs a Regulation from slug slices loaded from the database.
func NewRegulation(id, name, description string, maxRestricted int,
	bannedSlugs, restrictedSlugs, bannedMoveSlugs []string) *Regulation {
	r := &Regulation{
		ID:              id,
		Name:            name,
		Description:     description,
		MaxRestricted:   maxRestricted,
		BannedSlugs:     bannedSlugs,
		RestrictedSlugs: restrictedSlugs,
		BannedMoveSlugs: bannedMoveSlugs,
		bannedSpecies:   make(map[string]bool),
		restricted:      make(map[string]bool),
		bannedMoves:     make(map[string]bool),
	}
	for _, s := range bannedSlugs {
		r.bannedSpecies[s] = true
	}
	for _, s := range restrictedSlugs {
		r.restricted[s] = true
	}
	for _, s := range bannedMoveSlugs {
		r.bannedMoves[s] = true
	}
	return r
}

// IsBanned returns true if the species slug is banned in this regulation.
func (r *Regulation) IsBanned(slug string) bool { return r.bannedSpecies[slug] }

// IsRestricted returns true if the species slug is restricted in this regulation.
func (r *Regulation) IsRestricted(slug string) bool { return r.restricted[slug] }

// IsMoveBanned returns true if the move slug is banned in this regulation.
func (r *Regulation) IsMoveBanned(slug string) bool { return r.bannedMoves[slug] }
