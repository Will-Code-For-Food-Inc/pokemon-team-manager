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
		SELECT id, name, form, type1, COALESCE(type2,''), hp, attack, defense,
		       sp_attack, sp_defense, speed, generation,
		       is_legendary, is_mythical, is_final_evo, is_restricted
		FROM species WHERE lower(name) = lower(?)`, name))
}

// GetSpeciesByID returns a species by its National Dex number.
func (r *Repo) GetSpeciesByID(id int) (*Species, error) {
	return r.scanSpecies(r.db.QueryRow(`
		SELECT id, name, form, type1, COALESCE(type2,''), hp, attack, defense,
		       sp_attack, sp_defense, speed, generation,
		       is_legendary, is_mythical, is_final_evo, is_restricted
		FROM species WHERE id = ?`, id))
}

// SearchSpecies performs a case-insensitive substring search over species names.
// Results are ordered by name. limit <= 0 returns up to 20.
func (r *Repo) SearchSpecies(query string, limit int) ([]Species, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.Query(`
		SELECT id, name, form, type1, COALESCE(type2,''), hp, attack, defense,
		       sp_attack, sp_defense, speed, generation,
		       is_legendary, is_mythical, is_final_evo, is_restricted
		FROM species WHERE lower(name) LIKE lower(?) ORDER BY name LIMIT ?`,
		"%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanSpeciesRows(rows)
}

// GetAbilitiesForSpecies returns all abilities (slots 1, 2, 3) for a species.
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

// SearchMoves searches moves by name/description substring, optionally filtered
// by type and category.
func (r *Repo) SearchMoves(query, moveType, category string, limit int) ([]Move, error) {
	if limit <= 0 {
		limit = 20
	}
	args := []any{"%" + strings.ToLower(query) + "%"}
	q := `SELECT id, name, type, category, power, accuracy, pp, priority, target, description
		FROM moves WHERE (lower(name) LIKE ? OR lower(description) LIKE ?)`
	args = append(args, "%"+strings.ToLower(query)+"%")
	if moveType != "" {
		q += " AND lower(type) = lower(?)"
		args = append(args, moveType)
	}
	if category != "" {
		q += " AND lower(category) = lower(?)"
		args = append(args, category)
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
	err := r.db.QueryRow(`SELECT id, name, description, is_banned FROM items WHERE lower(name) = lower(?)`, name).
		Scan(&it.ID, &it.Name, &it.Description, &it.IsBanned)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("item %q not found", name)
	}
	return &it, err
}

// SearchItems searches items by name/description substring.
func (r *Repo) SearchItems(query string, limit int) ([]Item, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.Query(`SELECT id, name, description, is_banned FROM items
		WHERE lower(name) LIKE lower(?) OR lower(description) LIKE lower(?)
		ORDER BY name LIMIT ?`,
		"%"+query+"%", "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Name, &it.Description, &it.IsBanned); err != nil {
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
	err := row.Scan(&s.ID, &s.Name, &s.Form, &s.Type1, &type2,
		&s.HP, &s.Attack, &s.Defense, &s.SpAttack, &s.SpDefense, &s.Speed,
		&s.Generation, &s.IsLegendary, &s.IsMythical, &s.IsFinalEvo, &s.IsRestricted)
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
		if err := rows.Scan(&s.ID, &s.Name, &s.Form, &s.Type1, &type2,
			&s.HP, &s.Attack, &s.Defense, &s.SpAttack, &s.SpDefense, &s.Speed,
			&s.Generation, &s.IsLegendary, &s.IsMythical, &s.IsFinalEvo, &s.IsRestricted); err != nil {
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
