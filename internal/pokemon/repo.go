package pokemon

import (
	"database/sql"
	"fmt"
	"strings"
)

// Repo provides database access for Pokemon data.
type Repo struct {
	db *sql.DB
}

// NewRepo creates a new Repo backed by the given database connection.
func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// GetSpeciesByName returns a species by name (case-insensitive).
func (r *Repo) GetSpeciesByName(name string) (*Species, error) {
	return r.scanSpecies(r.db.QueryRow(`
		SELECT id, dex_id, name, form, type1, COALESCE(type2,''), hp, attack, defense,
		       sp_attack, sp_defense, speed, generation,
		       is_legendary, is_mythical, is_final_evo, is_restricted, owned
		FROM species WHERE lower(name) = lower(?)`, name))
}

// GetSpeciesByID returns a species by its National Dex number.
func (r *Repo) GetSpeciesByID(id int) (*Species, error) {
	return r.scanSpecies(r.db.QueryRow(`
		SELECT id, dex_id, name, form, type1, COALESCE(type2,''), hp, attack, defense,
		       sp_attack, sp_defense, speed, generation,
		       is_legendary, is_mythical, is_final_evo, is_restricted, owned
		FROM species WHERE id = ?`, id))
}

// SpeciesFilter holds optional filters for SearchSpecies.
type SpeciesFilter struct {
	Type         string // filter by type1 or type2
	Generation   int    // filter by generation (0 = any)
	Owned        *bool  // nil = any, true/false = owned status
	Legendary    *bool  // nil = any
	Restricted   *bool  // nil = any
	FinalEvoOnly bool   // if true, only final evolutions
	// Heuristic filters — derived from base stats at query time.
	// Role: "physical attacker", "special attacker", "mixed attacker", "support", "tank"
	Role      string
	// SpeedTier: "fast" (>100), "mid" (70-100), "slow" (<70)
	SpeedTier string
}

// SearchSpecies performs a case-insensitive fuzzy search over species names
// with optional filters. Each whitespace-separated word in name must appear
// somewhere in the species name (AND semantics). Results are ordered by name.
// limit <= 0 returns up to 20.
func (r *Repo) SearchSpecies(name string, limit int, f SpeciesFilter) ([]Species, error) {
	if limit <= 0 {
		limit = 20
	}
	q := `SELECT id, dex_id, name, form, type1, COALESCE(type2,''), hp, attack, defense,
		       sp_attack, sp_defense, speed, generation,
		       is_legendary, is_mythical, is_final_evo, is_restricted, owned
		FROM species WHERE 1=1`
	var args []any
	for _, word := range strings.Fields(name) {
		q += " AND lower(name) LIKE lower(?)"
		args = append(args, "%"+word+"%")
	}

	if f.Type != "" {
		q += " AND (lower(type1) = lower(?) OR lower(type2) = lower(?))"
		args = append(args, f.Type, f.Type)
	}
	if f.Generation > 0 {
		q += " AND generation = ?"
		args = append(args, f.Generation)
	}
	if f.Owned != nil {
		q += " AND owned = ?"
		if *f.Owned {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	if f.Legendary != nil {
		q += " AND is_legendary = ?"
		if *f.Legendary {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	if f.Restricted != nil {
		q += " AND is_restricted = ?"
		if *f.Restricted {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	if f.FinalEvoOnly {
		q += " AND is_final_evo = 1"
	}
	switch f.Role {
	case "tank":
		q += " AND (hp * (defense + sp_defense) / 200) >= 80"
	case "support":
		q += " AND attack < 80 AND sp_attack < 80"
	case "physical attacker":
		q += " AND attack >= sp_attack + 20"
	case "special attacker":
		q += " AND sp_attack >= attack + 20"
	case "mixed attacker":
		q += " AND attack < sp_attack + 20 AND sp_attack < attack + 20 AND NOT (attack < 80 AND sp_attack < 80)"
	}
	switch f.SpeedTier {
	case "fast":
		q += " AND speed > 100"
	case "mid":
		q += " AND speed >= 70 AND speed <= 100"
	case "slow":
		q += " AND speed < 70"
	}
	q += " ORDER BY name LIMIT ?"
	args = append(args, limit)

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanSpeciesRows(rows)
}

// GetAbilitiesForSpecies returns all abilities (slots 1, 2, 3) for a species.
// SearchSpeciesForAgent applies agent-facing defaults before calling SearchSpecies:
// owned=true if unspecified, limit=20 if <= 0.
func (r *Repo) SearchSpeciesForAgent(name string, limit int, f SpeciesFilter) ([]Species, error) {
	if f.Owned == nil {
		t := true
		f.Owned = &t
	}
	if limit <= 0 {
		limit = 20
	}
	return r.SearchSpecies(name, limit, f)
}

func (r *Repo) GetAbilitiesForSpecies(speciesID int) ([]Ability, error) {
	rows, err := r.db.Query(`
		SELECT a.id, a.name, a.description
		FROM abilities a
		JOIN species_abilities sa ON sa.ability_id = a.id
		WHERE sa.species_id = ?
		ORDER BY sa.slot`, speciesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ability
	for rows.Next() {
		var a Ability
		if err := rows.Scan(&a.ID, &a.Name, &a.Description); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetLearnset returns all moves a species can learn.
func (r *Repo) GetLearnset(speciesID int) ([]Move, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT m.id, m.name, m.type, m.category,
		       m.power, m.accuracy, m.pp, m.priority, m.target, m.description
		FROM moves m
		JOIN learnsets l ON l.move_id = m.id
		WHERE l.species_id = ?
		ORDER BY m.name`, speciesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanMoveRows(rows)
}

// CanLearnMove returns true if the species can learn the move by any method.
func (r *Repo) CanLearnMove(speciesID, moveID int) (bool, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM learnsets WHERE species_id=? AND move_id=?`,
		speciesID, moveID).Scan(&n)
	return n > 0, err
}

// HasAbility returns true if the ability is legal for the species.
func (r *Repo) HasAbility(speciesID, abilityID int) (bool, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM species_abilities WHERE species_id=? AND ability_id=?`,
		speciesID, abilityID).Scan(&n)
	return n > 0, err
}

// GetMoveByName returns a move by name (case-insensitive).
func (r *Repo) GetMoveByName(name string) (*Move, error) {
	row := r.db.QueryRow(`
		SELECT id, name, type, category, power, accuracy, pp, priority, target, description
		FROM moves WHERE lower(name) = lower(?)`, name)
	return r.scanMove(row)
}

// GetMoveByID returns a move by ID.
func (r *Repo) GetMoveByID(id int) (*Move, error) {
	row := r.db.QueryRow(`
		SELECT id, name, type, category, power, accuracy, pp, priority, target, description
		FROM moves WHERE id = ?`, id)
	return r.scanMove(row)
}

// MoveFilter holds optional filters for SearchMoves.
type MoveFilter struct {
	Type        string // filter by type
	Category    string // physical, special, status
	MinPower    int    // 0 = no minimum
	MaxPower    int    // 0 = no maximum
	MinAccuracy int    // 0 = no minimum
	Priority    *int   // nil = any
}

// SearchMoves searches moves by name/description substring with optional filters.
func (r *Repo) SearchMoves(query string, limit int, f MoveFilter) ([]Move, error) {
	if limit <= 0 {
		limit = 20
	}
	like := "%" + strings.ToLower(query) + "%"
	q := `SELECT id, name, type, category, power, accuracy, pp, priority, target, description
		FROM moves WHERE (lower(name) LIKE ? OR lower(description) LIKE ?)`
	args := []any{like, like}

	if f.Type != "" {
		q += " AND lower(type) = lower(?)"
		args = append(args, f.Type)
	}
	if f.Category != "" {
		q += " AND lower(category) = lower(?)"
		args = append(args, f.Category)
	}
	if f.MinPower > 0 {
		q += " AND power >= ?"
		args = append(args, f.MinPower)
	}
	if f.MaxPower > 0 {
		q += " AND power <= ?"
		args = append(args, f.MaxPower)
	}
	if f.MinAccuracy > 0 {
		q += " AND accuracy >= ?"
		args = append(args, f.MinAccuracy)
	}
	if f.Priority != nil {
		q += " AND priority = ?"
		args = append(args, *f.Priority)
	}
	q += " ORDER BY name LIMIT ?"
	args = append(args, limit)

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanMoveRows(rows)
}

// GetAbilityByName returns an ability by name (case-insensitive).
func (r *Repo) GetAbilityByName(name string) (*Ability, error) {
	var a Ability
	err := r.db.QueryRow(`SELECT id, name, description FROM abilities WHERE lower(name) = lower(?)`, name).
		Scan(&a.ID, &a.Name, &a.Description)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("ability %q not found", name)
	}
	return &a, err
}

// GetItemByName returns an item by name (case-insensitive).
func (r *Repo) GetItemByName(name string) (*Item, error) {
	var it Item
	err := r.db.QueryRow(`SELECT id, name, description, is_banned, owned FROM items WHERE lower(name) = lower(?)`, name).
		Scan(&it.ID, &it.Name, &it.Description, &it.IsBanned, &it.Owned)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("item %q not found", name)
	}
	return &it, err
}

// ItemFilter holds optional filters for SearchItems.
type ItemFilter struct {
	Owned   *bool // nil = any
	Banned  *bool // nil = any
}

// SearchItems searches items by name/description substring with optional filters.
func (r *Repo) SearchItems(query string, limit int, f ItemFilter) ([]Item, error) {
	if limit <= 0 {
		limit = 20
	}
	like := "%" + query + "%"
	q := `SELECT id, name, description, is_banned, owned FROM items
		WHERE (lower(name) LIKE lower(?) OR lower(description) LIKE lower(?))`
	args := []any{like, like}

	if f.Owned != nil {
		q += " AND owned = ?"
		if *f.Owned {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	if f.Banned != nil {
		q += " AND is_banned = ?"
		if *f.Banned {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	q += " ORDER BY name LIMIT ?"
	args = append(args, limit)

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Name, &it.Description, &it.IsBanned, &it.Owned); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// --- scan helpers ---

func (r *Repo) scanSpecies(row *sql.Row) (*Species, error) {
	var s Species
	var type2 string
	err := row.Scan(&s.ID, &s.DexID, &s.Name, &s.Form, &s.Type1, &type2,
		&s.HP, &s.Attack, &s.Defense, &s.SpAttack, &s.SpDefense, &s.Speed,
		&s.Generation, &s.IsLegendary, &s.IsMythical, &s.IsFinalEvo, &s.IsRestricted, &s.Owned)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("species not found")
	}
	if err != nil {
		return nil, err
	}
	s.Type2 = Type(type2)
	return &s, nil
}

func (r *Repo) scanSpeciesRows(rows *sql.Rows) ([]Species, error) {
	var out []Species
	for rows.Next() {
		var s Species
		var type2 string
		if err := rows.Scan(&s.ID, &s.DexID, &s.Name, &s.Form, &s.Type1, &type2,
			&s.HP, &s.Attack, &s.Defense, &s.SpAttack, &s.SpDefense, &s.Speed,
			&s.Generation, &s.IsLegendary, &s.IsMythical, &s.IsFinalEvo, &s.IsRestricted, &s.Owned); err != nil {
			return nil, err
		}
		s.Type2 = Type(type2)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repo) scanMove(row *sql.Row) (*Move, error) {
	var m Move
	var power, accuracy sql.NullInt64
	err := row.Scan(&m.ID, &m.Name, &m.Type, &m.Category,
		&power, &accuracy, &m.PP, &m.Priority, &m.Target, &m.Description)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("move not found")
	}
	if err != nil {
		return nil, err
	}
	if power.Valid {
		v := int(power.Int64)
		m.Power = &v
	}
	if accuracy.Valid {
		v := int(accuracy.Int64)
		m.Accuracy = &v
	}
	return &m, nil
}

func (r *Repo) scanMoveRows(rows *sql.Rows) ([]Move, error) {
	var out []Move
	for rows.Next() {
		var m Move
		var power, accuracy sql.NullInt64
		if err := rows.Scan(&m.ID, &m.Name, &m.Type, &m.Category,
			&power, &accuracy, &m.PP, &m.Priority, &m.Target, &m.Description); err != nil {
			return nil, err
		}
		if power.Valid {
			v := int(power.Int64)
			m.Power = &v
		}
		if accuracy.Valid {
			v := int(accuracy.Int64)
			m.Accuracy = &v
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddLearnsetMove adds a move to a species' learnset if not already present.
func (r *Repo) AddLearnsetMove(speciesID, moveID int) error {
	_, err := r.db.Exec(
		`INSERT OR IGNORE INTO learnsets (species_id, move_id, learn_method) VALUES (?, ?, 'level-up')`,
		speciesID, moveID)
	return err
}

// SetSpeciesOwned sets the owned flag for a species by ID.
func (r *Repo) SetSpeciesOwned(id int, owned bool) error {
	v := 0
	if owned {
		v = 1
	}
	_, err := r.db.Exec(`UPDATE species SET owned=? WHERE id=?`, v, id)
	return err
}

// SetItemOwned sets the owned flag for an item by ID.
func (r *Repo) SetItemOwned(id int, owned bool) error {
	v := 0
	if owned {
		v = 1
	}
	_, err := r.db.Exec(`UPDATE items SET owned=? WHERE id=?`, v, id)
	return err
}
