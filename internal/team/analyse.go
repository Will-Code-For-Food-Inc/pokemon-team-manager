package team

import (
	"fmt"
	"math"
	"strings"

	"github.com/user/pokemon-team-manager/internal/pokemon"
)

// calcStat computes the final Lv50 stat using the Pokemon Champions formula.
// IVs are always 31. 1 stat point = +2 to the inner term = +1 to the final stat.
//
//	HP:    floor((2*base + 31 + sp*2) * 50 / 100) + 60
//	Other: floor((floor((2*base + 31 + sp*2) * 50 / 100) + 5) * natureMult)
func calcStat(base, sp int, isHP bool, natureMult float64) int {
	inner := (2*base + 31 + sp*2) * 50 / 100
	if isHP {
		return inner + 60
	}
	return int(math.Floor(float64(inner+5) * natureMult))
}

// CalcStat is the exported form of calcStat for testing and external use.
func CalcStat(base, sp int, isHP bool, natureMult float64) int {
	return calcStat(base, sp, isHP, natureMult)
}

// natureMult returns the nature multiplier for a stat.
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

	for _, m := range t.Members {
		if m.Config == nil || m.Config.Species == nil {
			continue
		}
		c := m.Config
		mult := natureMult(c.Nature, string(pokemon.StatSpe))
		speed := calcStat(c.Species.Speed, c.EVs.Spe, false, mult)
		a.SpeedTiers = append(a.SpeedTiers, SpeedTier{
			Slot:      m.Slot,
			Name:      displayName(m),
			BaseSpeed: c.Species.Speed,
			StatSpeed: speed,
		})
	}

	weakCounts := map[string]int{}
	for _, m := range t.Members {
		if m.Config == nil || m.Config.Species == nil {
			continue
		}
		defTypes := m.Config.Species.Types()
		for _, at := range AllTypes {
			if DefenseMultiplier(at, defTypes) > 10 {
				weakCounts[string(at)]++
			}
		}
	}
	for typ, count := range weakCounts {
		if count >= 2 {
			a.DefensiveWeaknesses = append(a.DefensiveWeaknesses, WeaknessSummary{Type: typ, Count: count})
		}
	}

	coveredTypes := map[string]bool{}
	for _, m := range t.Members {
		if m.Config == nil {
			continue
		}
		for _, mv := range m.Config.Moves {
			if mv == nil || mv.Category == pokemon.CategoryStatus {
				continue
			}
			for _, dt := range AllTypes {
				if DefenseMultiplier(mv.Type, []pokemon.Type{dt}) > 10 {
					coveredTypes[string(dt)] = true
				}
			}
		}
	}
	for typ := range coveredTypes {
		a.OffensiveCoverage = append(a.OffensiveCoverage, typ)
	}

	a.Archetypes = detectArchetypes(t.Members)

	for _, m := range t.Members {
		if m.Config == nil || m.Config.Species == nil {
			continue
		}
		c := m.Config
		a.EVSummary = append(a.EVSummary, EVSummary{
			Slot:   m.Slot,
			Name:   displayName(m),
			Nature: c.Nature,
			EVs:    c.EVs,
			Total:  c.EVs.Total(),
		})
	}

	return a
}

// detectArchetypes identifies team archetypes from member data.
func detectArchetypes(members []Member) []string {
	var archetypes []string

	hasTR := false
	trCount := 0
	slowCount := 0
	for _, m := range members {
		if m.Config == nil {
			continue
		}
		for _, mv := range m.Config.Moves {
			if mv != nil && strings.EqualFold(mv.Name, "trick room") {
				hasTR = true
				trCount++
			}
		}
		if m.Config.Species != nil && m.Config.Species.Speed <= 50 {
			slowCount++
		}
	}
	if hasTR || (slowCount >= 3 && trCount > 0) {
		archetypes = append(archetypes, "Trick Room")
	}

	weatherAbilities := map[string]string{
		"drought": "Sun", "drizzle": "Rain",
		"sandstream": "Sand", "snow warning": "Snow",
	}
	weatherCounts := map[string]int{}
	for _, m := range members {
		if m.Config == nil || m.Config.Ability == nil {
			continue
		}
		ab := strings.ToLower(m.Config.Ability.Name)
		for name, weather := range weatherAbilities {
			if ab == name {
				weatherCounts[weather]++
			}
		}
		if ab == "protosynthesis" {
			weatherCounts["Sun"]++
		}
		if ab == "swift swim" || ab == "rain dish" {
			weatherCounts["Rain"]++
		}
	}
	for weather, count := range weatherCounts {
		if count >= 2 {
			archetypes = append(archetypes, weather+" Team")
		}
	}

	twCount := 0
	for _, m := range members {
		if m.Config == nil {
			continue
		}
		for _, mv := range m.Config.Moves {
			if mv != nil && strings.EqualFold(mv.Name, "tailwind") {
				twCount++
			}
		}
	}
	if twCount >= 1 {
		archetypes = append(archetypes, "Tailwind")
	}

	offCount := 0
	for _, m := range members {
		if m.Config == nil || m.Config.Species == nil {
			continue
		}
		if m.Config.Species.BST() >= 580 && m.Config.Species.Speed >= 100 {
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

// TypeMatchup holds the full defensive type chart result for a species.
type TypeMatchup struct {
	Immune    []string `json:"immune"`    // 0×
	Quarter   []string `json:"quarter"`   // 0.25×
	Half      []string `json:"half"`      // 0.5×
	Neutral   []string `json:"neutral"`   // 1×
	Double    []string `json:"double"`    // 2×
	Quadruple []string `json:"quadruple"` // 4×
}

// GetTypeMatchup returns the full defensive type chart for the given types.
func GetTypeMatchup(types []pokemon.Type) TypeMatchup {
	var m TypeMatchup
	for _, at := range AllTypes {
		mult := DefenseMultiplier(at, types)
		name := string(at)
		switch mult {
		case 0:
			m.Immune = append(m.Immune, name)
		case 25:
			m.Quarter = append(m.Quarter, name)
		case 5:
			m.Half = append(m.Half, name)
		case 10:
			m.Neutral = append(m.Neutral, name)
		case 20:
			m.Double = append(m.Double, name)
		case 40:
			m.Quadruple = append(m.Quadruple, name)
		}
	}
	return m
}

// SpeciesEval is the species-level evaluation result.
type SpeciesEval struct {
	Name              string      `json:"name"`
	Types             []string    `json:"types"`
	TypeMatchup       TypeMatchup `json:"type_matchup"`
	OffensiveCoverage []string    `json:"offensive_coverage"`
	StatRole          string      `json:"stat_role"`
	SpeedTier         string      `json:"speed_tier"`
	PhysicalBulk      float64     `json:"physical_bulk"`
	SpecialBulk       float64     `json:"special_bulk"`
}

// MemberEval extends SpeciesEval with assigned-moveset/EV context.
type MemberEval struct {
	SpeciesEval
	MoveStatMismatch bool   `json:"move_stat_mismatch"`
	HasPriorityMove  bool   `json:"has_priority_move"`
	HasSetupMove     bool   `json:"has_setup_move"`
	HasRecoveryMove  bool   `json:"has_recovery_move"`
	HasRedirection   bool   `json:"has_redirection"`
	EVEfficiency     string `json:"ev_efficiency"`
}

// EvaluateSpecies computes a species-level evaluation using base stats and learnset.
func EvaluateSpecies(sp *pokemon.Species, learnset []pokemon.Move) SpeciesEval {
	typeNames := make([]string, 0, 2)
	for _, t := range sp.Types() {
		typeNames = append(typeNames, string(t))
	}

	coveredTypes := map[string]bool{}
	for _, mv := range learnset {
		if mv.Category == pokemon.CategoryStatus {
			continue
		}
		for _, dt := range AllTypes {
			if DefenseMultiplier(mv.Type, []pokemon.Type{dt}) > 10 {
				coveredTypes[string(dt)] = true
			}
		}
	}
	coverage := make([]string, 0, len(coveredTypes))
	for t := range coveredTypes {
		coverage = append(coverage, t)
	}

	var speedTier string
	switch {
	case sp.Speed > 100:
		speedTier = fmt.Sprintf("fast (%d base)", sp.Speed)
	case sp.Speed >= 70:
		speedTier = fmt.Sprintf("mid (%d base)", sp.Speed)
	default:
		speedTier = fmt.Sprintf("slow (%d base)", sp.Speed)
	}

	physBulk := float64(sp.HP) * float64(sp.Defense) / 100
	specBulk := float64(sp.HP) * float64(sp.SpDefense) / 100

	return SpeciesEval{
		Name:              sp.Name,
		Types:             typeNames,
		TypeMatchup:       GetTypeMatchup(sp.Types()),
		OffensiveCoverage: coverage,
		StatRole:          classifyStatRole(sp),
		SpeedTier:         speedTier,
		PhysicalBulk:      math.Round(physBulk*10) / 10,
		SpecialBulk:       math.Round(specBulk*10) / 10,
	}
}

func classifyStatRole(sp *pokemon.Species) string {
	if sp.HP*int(sp.Defense+sp.SpDefense)/200 >= 80 {
		return "tank"
	}
	if sp.Attack < 80 && sp.SpAttack < 80 {
		return "support"
	}
	if sp.Attack >= sp.SpAttack+20 {
		return "physical attacker"
	}
	if sp.SpAttack >= sp.Attack+20 {
		return "special attacker"
	}
	return "mixed attacker"
}

// EvaluateMember computes a member-level evaluation including move/EV context.
func EvaluateMember(m *Member) MemberEval {
	if m.Config == nil || m.Config.Species == nil {
		return MemberEval{}
	}
	c := m.Config

	learnset := make([]pokemon.Move, 0, len(c.Moves))
	for _, mv := range c.Moves {
		if mv != nil {
			learnset = append(learnset, *mv)
		}
	}
	eval := MemberEval{SpeciesEval: EvaluateSpecies(c.Species, learnset)}

	setupMoves := map[string]bool{
		"dragon dance": true, "calm mind": true, "swords dance": true,
		"nasty plot": true, "iron defense": true, "quiver dance": true,
		"belly drum": true, "clangorous soul": true,
	}
	recoveryMoves := map[string]bool{
		"recover": true, "roost": true, "moonlight": true, "synthesis": true,
		"shore up": true, "slack off": true, "soft-boiled": true, "wish": true,
		"healing wish": true, "lunar blessing": true,
	}
	redirectMoves := map[string]bool{"follow me": true, "rage powder": true}

	physMoveCount, specMoveCount := 0, 0
	for _, mv := range c.Moves {
		if mv == nil {
			continue
		}
		name := strings.ToLower(mv.Name)
		if mv.Priority > 0 {
			eval.HasPriorityMove = true
		}
		if setupMoves[name] {
			eval.HasSetupMove = true
		}
		if recoveryMoves[name] {
			eval.HasRecoveryMove = true
		}
		if redirectMoves[name] {
			eval.HasRedirection = true
		}
		if mv.Category == pokemon.CategoryPhysical {
			physMoveCount++
		} else if mv.Category == pokemon.CategorySpecial {
			specMoveCount++
		}
	}

	if physMoveCount > 0 && specMoveCount == 0 && c.EVs.SpA > 0 {
		eval.MoveStatMismatch = true
		eval.EVEfficiency = fmt.Sprintf("SpA EVs (%d pts) wasted — moveset is purely physical", c.EVs.SpA)
	} else if specMoveCount > 0 && physMoveCount == 0 && c.EVs.Atk > 0 {
		eval.MoveStatMismatch = true
		eval.EVEfficiency = fmt.Sprintf("Atk EVs (%d pts) wasted — moveset is purely special", c.EVs.Atk)
	} else {
		eval.EVEfficiency = "ok"
	}

	return eval
}

// displayName returns nickname if set, otherwise species name.
func displayName(m Member) string {
	if m.Config != nil {
		if m.Config.Nickname != "" {
			return m.Config.Nickname
		}
		if m.Config.Species != nil {
			return m.Config.Species.Name
		}
	}
	return fmt.Sprintf("Slot %d", m.Slot)
}
