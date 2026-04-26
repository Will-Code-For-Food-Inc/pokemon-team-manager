// Package team provides models, CRUD operations, VGC validation, and analysis
// for Pokemon team management.
package team

import (
	"time"

	"github.com/user/pokemon-team-manager/internal/pokemon"
)

// Team represents a VGC team stored in the database.
type Team struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	Regulation string    `json:"regulation"`
	Notes      string    `json:"notes"`
	// NotesHTML is the Notes field rendered to HTML via goldmark (unsafe HTML stripped).
	NotesHTML  string    `json:"notes_html"`
	Members    []Member  `json:"members"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Member represents a single Pokemon slot on a team.
type Member struct {
	ID        int              `json:"id"`
	TeamID    int              `json:"team_id"`
	Slot      int              `json:"slot"`
	Species   *pokemon.Species `json:"species,omitempty"`
	Nickname  string           `json:"nickname"`
	Ability   *pokemon.Ability `json:"ability,omitempty"`
	Item      *pokemon.Item    `json:"item,omitempty"`
	TeraType  string           `json:"tera_type"`
	Nature    string           `json:"nature"`
	Role      string           `json:"role"`
	Notes     string           `json:"notes"`
	EVs       StatSpread       `json:"evs"`
	Moves     []*pokemon.Move  `json:"moves"`
}

// StatSpread holds stat point values for all six stats.
type StatSpread struct {
	HP  int `json:"hp"`
	Atk int `json:"attack"`
	Def int `json:"defense"`
	SpA int `json:"sp_attack"`
	SpD int `json:"sp_defense"`
	Spe int `json:"speed"`
}

// Total returns the sum of all stats in the spread.
func (s StatSpread) Total() int {
	return s.HP + s.Atk + s.Def + s.SpA + s.SpD + s.Spe
}

// Violation is a single VGC rule violation message.
type Violation struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Analysis is the result of analysing a team's composition.
type Analysis struct {
	TeamID           int               `json:"team_id"`
	TeamName         string            `json:"team_name"`
	SpeedTiers       []SpeedTier       `json:"speed_tiers"`
	OffensiveCoverage []string         `json:"offensive_coverage"`
	DefensiveWeaknesses []WeaknessSummary `json:"defensive_weaknesses"`
	Archetypes       []string          `json:"archetypes"`
	EVSummary        []EVSummary       `json:"ev_summary"`
}

// SpeedTier holds the calculated Lv. 50 speed stat for a member.
type SpeedTier struct {
	Slot      int    `json:"slot"`
	Name      string `json:"name"`
	BaseSpeed int    `json:"base_speed"`
	StatSpeed int    `json:"stat_speed"` // calculated at Lv.50 with EVs/IVs/nature
}

// WeaknessSummary records how many team members share a weakness.
type WeaknessSummary struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// TeamLog is a single combat/session log entry for a team.
type TeamLog struct {
	ID        int       `json:"id"`
	TeamID    int       `json:"team_id"`
	Entry     string    `json:"entry"`
	CreatedAt time.Time `json:"created_at"`
}

// EVSummary summarises EV investment for a team member.
type EVSummary struct {
	Slot   int        `json:"slot"`
	Name   string     `json:"name"`
	Nature string     `json:"nature"`
	EVs    StatSpread `json:"evs"`
	Total  int        `json:"total"`
}
