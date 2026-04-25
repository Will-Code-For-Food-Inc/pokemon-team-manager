package team

import (
	"fmt"
	"math"
	"strings"

	"github.com/user/pokemon-team-manager/internal/pokemon"
)

// calcStat computes the final Lv. 50 stat value using the standard formula.
//
//	HP:    floor((2*base + iv + floor(ev/4)) * 50 / 100 + 60)
//	Other: floor((floor((2*base + iv + floor(ev/4)) * 50 / 100) + 5) * natureMult)
func calcStat(base, iv, ev int, isHP bool, natureMult float64) int {
	inner := (2*base + iv + ev/4) * 50 / 100
	if isHP {
		return inner + 60
	}
	return int(math.Floor(float64(inner+5) * natureMult))
}

// natureMult returns the nature multiplier for a stat.
// boosted stat → 1.1, reduced stat → 0.9, neutral → 1.0.
func natureMult(nature, stat string) float64 {
	n, ok := pokemon.NatureByName(nature)
	if !ok {
		return 1.0
	}
	if string(n.Boosted) == stat {
		return 1.1
	}
	if string(n.Reduced) == stat {
		return 0.9
	}
	return 1.0
}

// Analyse computes team analysis metrics for the given team.
func Analyse(t *Team) *Analysis {
	a := &Analysis{
		TeamID:   t.ID,
		TeamName: t.Name,
	}

	// Speed tiers.
	for _, m := range t.Members {
		if m.Species == nil {
			continue
		}
		mult := natureMult(m.Nature, string(pokemon.StatSpe))
		speed := calcStat(m.Species.Speed, m.IVs.Spe, m.EVs.Spe, false, mult)
		a.SpeedTiers = append(a.SpeedTiers, SpeedTier{
			Slot:      m.Slot,
			Name:      displayName(m),
			BaseSpeed: m.Species.Speed,
			StatSpeed: speed,
		})
	}

	// Defensive weaknesses.
	weakCounts := map[string]int{}
	for _, m := range t.Members {
		if m.Species == nil {
			continue
		}
		defTypes := m.Species.Types()
		for _, at := range AllTypes {
			mult := DefenseMultiplier(at, defTypes)
			if mult > 10 {
				weakCounts[string(at)]++
			}
		}
	}
	for t, count := range weakCounts {
		if count >= 2 {
			a.DefensiveWeaknesses = append(a.DefensiveWeaknesses, WeaknessSummary{Type: t, Count: count})
		}
	}

	// Offensive type coverage (types the team can hit super-effectively).
	coveredTypes := map[string]bool{}
	for _, m := range t.Members {
		for _, mv := range m.Moves {
			if mv == nil || mv.Category == pokemon.CategoryStatus {
				continue
			}
			for _, dt := range AllTypes {
				mult := DefenseMultiplier(mv.Type, []pokemon.Type{dt})
				if mult > 10 {
					coveredTypes[string(dt)] = true
				}
			}
		}
	}
	for t := range coveredTypes {
		a.OffensiveCoverage = append(a.OffensiveCoverage, t)
	}

	// Archetype detection.
	a.Archetypes = detectArchetypes(t.Members)

	// EV summaries.
	for _, m := range t.Members {
		if m.Species == nil {
			continue
		}
		a.EVSummary = append(a.EVSummary, EVSummary{
			Slot:   m.Slot,
			Name:   displayName(m),
			Nature: m.Nature,
			EVs:    m.EVs,
			Total:  m.EVs.Total(),
		})
	}

	return a
}

// detectArchetypes identifies team archetypes from member data.
func detectArchetypes(members []Member) []string {
	var archetypes []string

	// Trick Room: look for moves/abilities associated with TR support.
	trSetters := []string{"trick room"}
	trSupportAbilities := []string{"telepathy", "indirectly"}
	hasTR := false
	trCount := 0
	slowCount := 0

	for _, m := range members {
		for _, mv := range m.Moves {
			if mv == nil {
				continue
			}
			for _, tr := range trSetters {
				if strings.EqualFold(mv.Name, tr) {
					hasTR = true
					trCount++
				}
			}
		}
		if m.Ability != nil {
			for _, ab := range trSupportAbilities {
				if strings.Contains(strings.ToLower(m.Ability.Name), ab) {
					_ = ab
				}
			}
		}
		if m.Species != nil && m.Species.Speed <= 50 {
			slowCount++
		}
	}
	_ = trSupportAbilities
	if hasTR || (slowCount >= 3 && trCount > 0) {
		archetypes = append(archetypes, "Trick Room")
	}

	// Weather detection via abilities.
	weatherAbilities := map[string]string{
		"drought":      "Sun",
		"drizzle":      "Rain",
		"sandstream":   "Sand",
		"snow warning": "Snow",
		"cloud nine":   "Weather Nullify",
	}
	weatherCounts := map[string]int{}
	for _, m := range members {
		if m.Ability == nil {
			continue
		}
		for ab, weather := range weatherAbilities {
			if strings.EqualFold(m.Ability.Name, ab) {
				weatherCounts[weather]++
			}
		}
		// Also check for Protosynthesis/Quark Drive (Paradox mons that thrive in weather).
		if strings.EqualFold(m.Ability.Name, "protosynthesis") {
			weatherCounts["Sun"]++
		}
		if strings.EqualFold(m.Ability.Name, "swift swim") || strings.EqualFold(m.Ability.Name, "rain dish") {
			weatherCounts["Rain"]++
		}
	}
	for weather, count := range weatherCounts {
		if count >= 2 {
			archetypes = append(archetypes, weather+" Team")
		}
	}

	// Tailwind detection.
	twCount := 0
	for _, m := range members {
		for _, mv := range m.Moves {
			if mv != nil && strings.EqualFold(mv.Name, "tailwind") {
				twCount++
			}
		}
	}
	if twCount >= 1 {
		archetypes = append(archetypes, "Tailwind")
	}

	// Hyper Offense: high BST, low bulk spread (all offensive EVs).
	offCount := 0
	for _, m := range members {
		if m.Species == nil {
			continue
		}
		if m.Species.BST() >= 580 && m.Species.Speed >= 100 {
			offCount++
		}
	}
	if offCount >= 4 {
		archetypes = append(archetypes, "Hyper Offense")
	}

	if len(archetypes) == 0 {
		archetypes = append(archetypes, "Balance")
	}
	return archetypes
}

// displayName returns the nickname if set, otherwise the species name.
func displayName(m Member) string {
	if m.Nickname != "" {
		return m.Nickname
	}
	if m.Species != nil {
		return m.Species.Name
	}
	return fmt.Sprintf("Slot %d", m.Slot)
}
