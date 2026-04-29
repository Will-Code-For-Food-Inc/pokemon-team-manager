// Package team provides models, CRUD operations, VGC validation, and analysis
// for Pokemon team management.
package team

import (
	"time"

	"github.com/user/pokemon-team-manager/internal/pokemon"
)

// Team represents a VGC team.
type Team struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Regulation   string    `json:"regulation"`
	// Strategy holds long-term team notes: gameplan, threats, synergy rationale.
	Strategy     string    `json:"strategy"`
	StrategyHTML string    `json:"strategy_html"`
	Members      []Member  `json:"members"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Member is a resolved team slot: slot number + full pokemon config.
type Member struct {
	Slot     int     `json:"slot"`
	ConfigID int     `json:"config_id"`
	Config   *Config `json:"config,omitempty"`
}

// Config is a fully-specified build for a single Pokemon.
// It lives independently of any team slot.
type Config struct {
	ID          int              `json:"id"`
	Species     *pokemon.Species `json:"species,omitempty"`
	Nickname    string           `json:"nickname"`
	Nature      string           `json:"nature"`
	Ability     *pokemon.Ability `json:"ability,omitempty"`
	Item        *pokemon.Item    `json:"item,omitempty"`
	TeraType    string           `json:"tera_type"`
	Role        string           `json:"role"`
	// Notes holds config-level tactical notes for this specific build.
	Notes       string           `json:"notes"`
	EVs         StatSpread       `json:"evs"`
	Moves       []*pokemon.Move  `json:"moves"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// StatSpread holds stat point (SP) values for all six stats.
type StatSpread struct {
	HP  int `json:"hp"`
	Atk int `json:"attack"`
	Def int `json:"defense"`
	SpA int `json:"sp_attack"`
	SpD int `json:"sp_defense"`
	Spe int `json:"speed"`
}

// Total returns the sum of all stat points.
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
	TeamID              int               `json:"team_id"`
	TeamName            string            `json:"team_name"`
	SpeedTiers          []SpeedTier       `json:"speed_tiers"`
	OffensiveCoverage   []string          `json:"offensive_coverage"`
	DefensiveWeaknesses []WeaknessSummary `json:"defensive_weaknesses"`
	Archetypes          []string          `json:"archetypes"`
	EVSummary           []EVSummary       `json:"ev_summary"`
}

// SpeedTier holds the calculated Lv50 speed stat for a team member.
type SpeedTier struct {
	Slot      int    `json:"slot"`
	Name      string `json:"name"`
	BaseSpeed int    `json:"base_speed"`
	StatSpeed int    `json:"stat_speed"`
}

// WeaknessSummary records how many team members share a type weakness.
type WeaknessSummary struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// TeamLog is a single battle/session journal entry.
// Logs are append-only experiential records; use team.Strategy for standing notes.
type TeamLog struct {
	ID        int       `json:"id"`
	TeamID    int       `json:"team_id"`
	Entry     string    `json:"entry"`
	CreatedAt time.Time `json:"created_at"`
}

// EVSummary summarises SP investment for a team member.
type EVSummary struct {
	Slot   int        `json:"slot"`
	Name   string     `json:"name"`
	Nature string     `json:"nature"`
	EVs    StatSpread `json:"evs"`
	Total  int        `json:"total"`
}

// TeamSummary is a lightweight team view used in list operations.
type TeamSummary struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Regulation  string    `json:"regulation"`
	Strategy    string    `json:"strategy"`
	MemberCount int       `json:"member_count"`
	CreatedAt   time.Time `json:"created_at"`
}
