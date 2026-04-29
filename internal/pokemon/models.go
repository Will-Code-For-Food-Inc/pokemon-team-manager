// Package pokemon provides data models and repository access for Pokemon
// species, moves, abilities, and items stored in the local SQLite database.
package pokemon

import "strings"

// Type represents a Pokemon elemental type (e.g. "fire", "water").
type Type string

// All Pokemon types.
const (
	TypeNormal   Type = "normal"
	TypeFire     Type = "fire"
	TypeWater    Type = "water"
	TypeElectric Type = "electric"
	TypeGrass    Type = "grass"
	TypeIce      Type = "ice"
	TypeFighting Type = "fighting"
	TypePoison   Type = "poison"
	TypeGround   Type = "ground"
	TypeFlying   Type = "flying"
	TypePsychic  Type = "psychic"
	TypeBug      Type = "bug"
	TypeRock     Type = "rock"
	TypeGhost    Type = "ghost"
	TypeDragon   Type = "dragon"
	TypeDark     Type = "dark"
	TypeSteel    Type = "steel"
	TypeFairy    Type = "fairy"
)

// MoveCategory classifies a move's damage source.
type MoveCategory string

const (
	CategoryPhysical MoveCategory = "physical"
	CategorySpecial  MoveCategory = "special"
	CategoryStatus   MoveCategory = "status"
)

// Nature represents a Pokemon's nature which boosts one stat and reduces another.
type Nature struct {
	Name    string
	Boosted Stat
	Reduced Stat
}

// Stat names used in EV/IV/nature references.
type Stat string

const (
	StatHP  Stat = "hp"
	StatAtk Stat = "attack"
	StatDef Stat = "defense"
	StatSpA Stat = "sp_attack"
	StatSpD Stat = "sp_defense"
	StatSpe Stat = "speed"
)

// AllNatures lists the 21 valid Stat Alignments in Pokemon Champions.
// Hardy, Docile, Bashful, and Quirky are not valid in this format.
// Serious is the free neutral nature (0 VP); all others cost 500 VP.
var AllNatures = []Nature{
	{Name: "Lonely", Boosted: StatAtk, Reduced: StatDef},
	{Name: "Brave", Boosted: StatAtk, Reduced: StatSpe},
	{Name: "Adamant", Boosted: StatAtk, Reduced: StatSpA},
	{Name: "Naughty", Boosted: StatAtk, Reduced: StatSpD},
	{Name: "Bold", Boosted: StatDef, Reduced: StatAtk},
	{Name: "Relaxed", Boosted: StatDef, Reduced: StatSpe},
	{Name: "Impish", Boosted: StatDef, Reduced: StatSpA},
	{Name: "Lax", Boosted: StatDef, Reduced: StatSpD},
	{Name: "Timid", Boosted: StatSpe, Reduced: StatAtk},
	{Name: "Hasty", Boosted: StatSpe, Reduced: StatDef},
	{Name: "Serious"},
	{Name: "Jolly", Boosted: StatSpe, Reduced: StatSpA},
	{Name: "Naive", Boosted: StatSpe, Reduced: StatSpD},
	{Name: "Modest", Boosted: StatSpA, Reduced: StatAtk},
	{Name: "Mild", Boosted: StatSpA, Reduced: StatDef},
	{Name: "Quiet", Boosted: StatSpA, Reduced: StatSpe},
	{Name: "Rash", Boosted: StatSpA, Reduced: StatSpD},
	{Name: "Calm", Boosted: StatSpD, Reduced: StatAtk},
	{Name: "Gentle", Boosted: StatSpD, Reduced: StatDef},
	{Name: "Sassy", Boosted: StatSpD, Reduced: StatSpe},
	{Name: "Careful", Boosted: StatSpD, Reduced: StatSpA},
}

// ValidNature returns true if the given nature name is valid in Champions format.
func ValidNature(name string) bool {
	for _, n := range AllNatures {
		if strings.EqualFold(n.Name, name) {
			return true
		}
	}
	return false
}

// NatureByName returns the Nature for the given name (case-insensitive).
func NatureByName(name string) (Nature, bool) {
	for _, n := range AllNatures {
		if strings.EqualFold(n.Name, name) {
			return n, true
		}
	}
	return Nature{}, false
}

// Species represents a Pokemon species row from the database.
type Species struct {
	Slug         string `json:"slug"`
	ID           int    `json:"id"`      // internal autoincrement (for legacy joins)
	DexID        int    `json:"dex_id"`
	Name         string `json:"name"`
	Form         string `json:"form"`
	Type1        Type   `json:"type1"`
	Type2        Type   `json:"type2,omitempty"`
	HP           int    `json:"hp"`
	Attack       int    `json:"attack"`
	Defense      int    `json:"defense"`
	SpAttack     int    `json:"sp_attack"`
	SpDefense    int    `json:"sp_defense"`
	Speed        int    `json:"speed"`
	Generation   int    `json:"generation"`
	IsLegendary  bool   `json:"is_legendary"`
	IsMythical   bool   `json:"is_mythical"`
	IsFinalEvo   bool   `json:"is_final_evo"`
	IsRestricted bool   `json:"is_restricted"`
	Owned        bool   `json:"owned"`
}

// BST returns the base stat total for the species.
func (s Species) BST() int {
	return s.HP + s.Attack + s.Defense + s.SpAttack + s.SpDefense + s.Speed
}

// Types returns a slice of non-empty types for the species.
func (s Species) Types() []Type {
	if s.Type2 == "" {
		return []Type{s.Type1}
	}
	return []Type{s.Type1, s.Type2}
}

// Move represents a move row from the database.
type Move struct {
	ID          int          `json:"id"`
	Slug        string       `json:"slug"`
	Name        string       `json:"name"`
	Type        Type         `json:"type"`
	Category    MoveCategory `json:"category"`
	Power       *int         `json:"power,omitempty"`
	Accuracy    *int         `json:"accuracy,omitempty"`
	PP          int          `json:"pp"`
	Priority    int          `json:"priority"`
	Target      string       `json:"target"`
	Description string       `json:"description"`
}

// Ability represents an ability row from the database.
type Ability struct {
	ID          int    `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Item represents an item row from the database.
type Item struct {
	ID          int    `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsBanned    bool   `json:"is_banned"`
	VPCost      int    `json:"vp_cost"`
	Owned       bool   `json:"owned"`
}

// SpeciesAbility links an ability to a species at a given slot.
type SpeciesAbility struct {
	SpeciesSlug string
	AbilitySlug string
	Slot        int // 1, 2, or 3 (hidden)
}
