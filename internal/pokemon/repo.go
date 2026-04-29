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

// GetSpeciesByName looks up a species by name with form-aware matching.
// Form Pokémon (Rotom-Wash, Indeedee-F, Tauros-Paldean-Combat) are stored as
// (name="Rotom", form="Wash"), so a literal name match on "Rotom-Wash" misses.
// We try, in order:
//  1. exact name match — base form preferred when ambiguous (Rotom → rotom).
//  2. slug normalisation — "Rotom-Wash" / "Rotom Wash" → "rotom-wash".
//  3. concatenated name+form — matches "Rotom Wash" against name||' '||form.
//
// Returns a "species not found" error if all three fail.
func (r *Repo) GetSpeciesByName(name string) (*Species, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("species name is empty")
	}
	const cols = `slug, dex_id, name, form, type1, COALESCE(type2,''), hp, attack, defense,
		       sp_attack, sp_defense, speed, generation,
		       is_legendary, is_mythical, is_final_evo, is_restricted, owned`

	// 1. Exact name. ORDER BY puts base form (form='') first when multiple rows
	//    share a name (every Rotom row has name='Rotom').
	if sp, err := r.scanSpecies(r.db.QueryRow(
		`SELECT `+cols+` FROM species WHERE lower(name) = lower(?)
		 ORDER BY (form = '') DESC, slug LIMIT 1`, name)); err == nil {
		return sp, nil
	}

	// 2. Slug normalisation: "Rotom-Wash" → "rotom-wash", "Rotom Wash" → "rotom-wash".
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	if sp, err := r.GetSpeciesBySlug(slug); err == nil {
		return sp, nil
	}

	// 3. name+form concat — handles inputs that don't match the slug exactly,
	//    e.g. species whose form has spaces ("Tauros Paldean Combat").
	if sp, err := r.scanSpecies(r.db.QueryRow(
		`SELECT `+cols+` FROM species
		 WHERE form != ''
		   AND (lower(name || '-' || form) = lower(?)
		     OR lower(name || ' ' || form) = lower(?))
		 LIMIT 1`, name, name)); err == nil {
		return sp, nil
	}

	return nil, fmt.Errorf("species %q not found", name)
}

// GetSpeciesBySlug returns a species by its slug (primary key).
func (r *Repo) GetSpeciesBySlug(slug string) (*Species, error) {
	return r.scanSpecies(r.db.QueryRow(`
		SELECT slug, dex_id, name, form, type1, COALESCE(type2,''), hp, attack, defense,
		       sp_attack, sp_defense, speed, generation,
		       is_legendary, is_mythical, is_final_evo, is_restricted, owned
		FROM species WHERE slug = ?`, slug))
}

// GetSpeciesByDexID returns a species by its national dex number.
func (r *Repo) GetSpeciesByDexID(dexID int) (*Species, error) {
	return r.scanSpecies(r.db.QueryRow(`
		SELECT slug, dex_id, name, form, type1, COALESCE(type2,''), hp, attack, defense,
		       sp_attack, sp_defense, speed, generation,
		       is_legendary, is_mythical, is_final_evo, is_restricted, owned
		FROM species WHERE dex_id = ? AND form = ''`, dexID))
}

// SpeciesFilter holds optional filters for SearchSpecies.
type SpeciesFilter struct {
	Type         string
	Generation   int
	Owned        *bool
	Legendary    *bool
	Restricted   *bool
	FinalEvoOnly bool
	Role         string // "physical attacker", "special attacker", "mixed attacker", "support", "tank"
	SpeedTier    string // "fast" (>100), "mid" (70-100), "slow" (<70)
}

// SearchSpecies performs a case-insensitive fuzzy search over species names
// with optional filters. limit <= 0 returns up to 20.
func (r *Repo) SearchSpecies(name string, limit int, f SpeciesFilter) ([]Species, error) {
	if limit <= 0 {
		limit = 20
	}
	q := `SELECT slug, dex_id, name, form, type1, COALESCE(type2,''), hp, attack, defense,
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

// SearchSpeciesForAgent applies agent defaults (owned=true) before calling SearchSpecies.
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

// GetAbilitiesForSpecies returns all abilities (slots 1, 2, 3) for a species by slug.
func (r *Repo) GetAbilitiesForSpecies(speciesSlug string) ([]Ability, error) {
	rows, err := r.db.Query(`
		SELECT a.id, a.slug, a.name, a.description
		FROM abilities a
		JOIN species_abilities sa ON sa.ability_slug = a.slug
		WHERE sa.species_slug = ?
		ORDER BY sa.slot`, speciesSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ability
	for rows.Next() {
		var a Ability
		if err := rows.Scan(&a.ID, &a.Slug, &a.Name, &a.Description); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetChampionsLearnset returns the Champions-specific curated move list for a species.
// Uses LEFT JOIN so moves not yet in the moves table still appear (slug-only).
func (r *Repo) GetChampionsLearnset(speciesSlug string) ([]Move, error) {
	rows, err := r.db.Query(`
		SELECT cl.move_slug,
		       COALESCE(m.id, 0),
		       COALESCE(m.name, cl.move_slug),
		       COALESCE(m.type, ''),
		       COALESCE(m.category, 'status'),
		       m.power, m.accuracy,
		       COALESCE(m.pp, 0),
		       COALESCE(m.priority, 0),
		       COALESCE(m.target, ''),
		       COALESCE(m.description, '')
		FROM champions_learnsets cl
		LEFT JOIN moves m ON m.slug = cl.move_slug
		WHERE cl.species_slug = ?
		ORDER BY cl.move_slug`, speciesSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Move
	for rows.Next() {
		var m Move
		var power, accuracy sql.NullInt64
		if err := rows.Scan(&m.Slug, &m.ID, &m.Name, &m.Type, &m.Category,
			&power, &accuracy, &m.PP, &m.Priority, &m.Target, &m.Description); err != nil {
			return nil, err
		}
		if power.Valid {
			v := int(power.Int64); m.Power = &v
		}
		if accuracy.Valid {
			v := int(accuracy.Int64); m.Accuracy = &v
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetLearnset returns all moves a species can learn (PokeAPI learnset, fallback).
func (r *Repo) GetLearnset(speciesSlug string) ([]Move, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT m.id, m.slug, m.name, m.type, m.category,
		       m.power, m.accuracy, m.pp, m.priority, m.target, m.description
		FROM moves m
		JOIN learnsets l ON l.move_slug = m.slug
		WHERE l.species_slug = ?
		ORDER BY m.name`, speciesSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanMoveRows(rows)
}

// CanLearnMove returns true if the species can learn the move in Champions format.
// Checks the champions_learnsets table (authoritative).
func (r *Repo) CanLearnMove(speciesSlug, moveSlug string) (bool, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM champions_learnsets WHERE species_slug=? AND move_slug=?`,
		speciesSlug, moveSlug).Scan(&n)
	return n > 0, err
}

// HasAbility returns true if the ability is available for the species.
func (r *Repo) HasAbility(speciesSlug, abilitySlug string) (bool, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM species_abilities WHERE species_slug=? AND ability_slug=?`,
		speciesSlug, abilitySlug).Scan(&n)
	return n > 0, err
}

// GetMoveByName returns a move by name (case-insensitive).
func (r *Repo) GetMoveByName(name string) (*Move, error) {
	mv, err := r.scanMove(r.db.QueryRow(`
		SELECT id, slug, name, type, category, power, accuracy, pp, priority, target, description
		FROM moves WHERE lower(name) = lower(?)`, name))
	if err != nil && err.Error() == "move not found" {
		return nil, fmt.Errorf("move %q not found in the Champions move set", name)
	}
	return mv, err
}

// GetMoveBySlug returns a move by slug.
func (r *Repo) GetMoveBySlug(slug string) (*Move, error) {
	return r.scanMove(r.db.QueryRow(`
		SELECT id, slug, name, type, category, power, accuracy, pp, priority, target, description
		FROM moves WHERE slug = ?`, slug))
}

// MoveFilter holds optional filters for SearchMoves.
type MoveFilter struct {
	Type        string
	Category    string
	MinPower    int
	MaxPower    int
	MinAccuracy int
	Priority    *int
}

// SearchMoves searches moves by name/description substring with optional filters.
func (r *Repo) SearchMoves(query string, limit int, f MoveFilter) ([]Move, error) {
	if limit <= 0 {
		limit = 20
	}
	like := "%" + strings.ToLower(query) + "%"
	q := `SELECT id, slug, name, type, category, power, accuracy, pp, priority, target, description
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
	err := r.db.QueryRow(
		`SELECT id, slug, name, description FROM abilities WHERE lower(name) = lower(?)`, name).
		Scan(&a.ID, &a.Slug, &a.Name, &a.Description)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("ability %q not found", name)
	}
	return &a, err
}

// GetItemByName returns an item by name (case-insensitive).
func (r *Repo) GetItemByName(name string) (*Item, error) {
	var it Item
	err := r.db.QueryRow(
		`SELECT id, slug, name, description, is_banned, vp_cost, owned FROM items WHERE lower(name) = lower(?)`, name).
		Scan(&it.ID, &it.Slug, &it.Name, &it.Description, &it.IsBanned, &it.VPCost, &it.Owned)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("item %q not found", name)
	}
	return &it, err
}

// ItemFilter holds optional filters for SearchItems.
type ItemFilter struct {
	Owned  *bool
	Banned *bool
}

// SearchItems searches items by name/description substring with optional filters.
func (r *Repo) SearchItems(query string, limit int, f ItemFilter) ([]Item, error) {
	if limit <= 0 {
		limit = 20
	}
	like := "%" + query + "%"
	q := `SELECT id, slug, name, description, is_banned, vp_cost, owned FROM items
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
		if err := rows.Scan(&it.ID, &it.Slug, &it.Name, &it.Description, &it.IsBanned, &it.VPCost, &it.Owned); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// AddLearnsetMove appends a move to a species' Champions learnset and returns
// the updated CSV row for the caller to append to champions_learnsets.csv.
func (r *Repo) AddLearnsetMove(speciesSlug, moveSlug string) error {
	_, err := r.db.Exec(
		`INSERT OR IGNORE INTO champions_learnsets(species_slug, move_slug) VALUES(?, ?)`,
		speciesSlug, moveSlug)
	return err
}

// AddSpeciesAbility links an ability to a species (creates the ability if absent).
// Returns the ability slug so the caller can append it to species_abilities.csv.
func (r *Repo) AddSpeciesAbility(speciesSlug, abilityName, description string) (string, error) {
	abilitySlug := strings.ToLower(strings.ReplaceAll(abilityName, " ", "-"))

	// Upsert ability.
	_, err := r.db.Exec(
		`INSERT OR IGNORE INTO abilities(slug, name, description) VALUES(?, ?, ?)`,
		abilitySlug, abilityName, description)
	if err != nil {
		return "", fmt.Errorf("insert ability: %w", err)
	}

	// Find next available slot.
	slot := 1
	r.db.QueryRow(
		`SELECT COALESCE(MAX(slot),0)+1 FROM species_abilities WHERE species_slug=?`,
		speciesSlug).Scan(&slot)

	_, err = r.db.Exec(
		`INSERT OR IGNORE INTO species_abilities(species_slug, ability_slug, slot) VALUES(?,?,?)`,
		speciesSlug, abilitySlug, slot)
	return abilitySlug, err
}

// SetSpeciesOwned sets the owned flag for a species by slug.
func (r *Repo) SetSpeciesOwned(slug string, owned bool) error {
	v := 0
	if owned {
		v = 1
	}
	res, err := r.db.Exec(`UPDATE species SET owned=? WHERE slug=?`, v, slug)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no species with slug %q", slug)
	}
	return nil
}

// SetItemOwned sets the owned flag for an item by slug.
func (r *Repo) SetItemOwned(slug string, owned bool) error {
	v := 0
	if owned {
		v = 1
	}
	res, err := r.db.Exec(`UPDATE items SET owned=? WHERE slug=?`, v, slug)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no item with slug %q", slug)
	}
	return nil
}

// GetAbilityByID returns an ability by its integer ID (used by web form handlers).
func (r *Repo) GetAbilityByID(id int) (*Ability, error) {
	var a Ability
	err := r.db.QueryRow(
		`SELECT id, slug, name, description FROM abilities WHERE id=?`, id).
		Scan(&a.ID, &a.Slug, &a.Name, &a.Description)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("ability ID %d not found", id)
	}
	return &a, err
}

// GetItemByID returns an item by its integer ID (used by web form handlers).
func (r *Repo) GetItemByID(id int) (*Item, error) {
	var it Item
	err := r.db.QueryRow(
		`SELECT id, slug, name, description, is_banned, vp_cost, owned FROM items WHERE id=?`, id).
		Scan(&it.ID, &it.Slug, &it.Name, &it.Description, &it.IsBanned, &it.VPCost, &it.Owned)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("item ID %d not found", id)
	}
	return &it, err
}

// RemoveLearnsetMove removes a move from a species' Champions learnset.
func (r *Repo) RemoveLearnsetMove(speciesSlug, moveSlug string) error {
	_, err := r.db.Exec(
		`DELETE FROM champions_learnsets WHERE species_slug=? AND move_slug=?`,
		speciesSlug, moveSlug)
	return err
}

// RemoveSpeciesAbility removes an ability from a species' ability pool.
func (r *Repo) RemoveSpeciesAbility(speciesSlug, abilitySlug string) error {
	_, err := r.db.Exec(
		`DELETE FROM species_abilities WHERE species_slug=? AND ability_slug=?`,
		speciesSlug, abilitySlug)
	return err
}

// ── Scan helpers ──────────────────────────────────────────────────────────────

func (r *Repo) scanSpecies(row *sql.Row) (*Species, error) {
	var s Species
	var type2 string
	err := row.Scan(&s.Slug, &s.DexID, &s.Name, &s.Form, &s.Type1, &type2,
		&s.HP, &s.Attack, &s.Defense, &s.SpAttack, &s.SpDefense, &s.Speed,
		&s.Generation, &s.IsLegendary, &s.IsMythical, &s.IsFinalEvo, &s.IsRestricted, &s.Owned)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("species not found")
	}
	if err != nil {
		return nil, err
	}
	s.ID = s.DexID // keep ID populated for legacy compatibility
	s.Type2 = Type(type2)
	return &s, nil
}

func (r *Repo) scanSpeciesRows(rows *sql.Rows) ([]Species, error) {
	var out []Species
	for rows.Next() {
		var s Species
		var type2 string
		if err := rows.Scan(&s.Slug, &s.DexID, &s.Name, &s.Form, &s.Type1, &type2,
			&s.HP, &s.Attack, &s.Defense, &s.SpAttack, &s.SpDefense, &s.Speed,
			&s.Generation, &s.IsLegendary, &s.IsMythical, &s.IsFinalEvo, &s.IsRestricted, &s.Owned); err != nil {
			return nil, err
		}
		s.ID = s.DexID
		s.Type2 = Type(type2)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repo) scanMove(row *sql.Row) (*Move, error) {
	var m Move
	var power, accuracy sql.NullInt64
	err := row.Scan(&m.ID, &m.Slug, &m.Name, &m.Type, &m.Category,
		&power, &accuracy, &m.PP, &m.Priority, &m.Target, &m.Description)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("move not found")
	}
	if err != nil {
		return nil, err
	}
	if power.Valid {
		v := int(power.Int64); m.Power = &v
	}
	if accuracy.Valid {
		v := int(accuracy.Int64); m.Accuracy = &v
	}
	return &m, nil
}

func (r *Repo) scanMoveRows(rows *sql.Rows) ([]Move, error) {
	var out []Move
	for rows.Next() {
		var m Move
		var power, accuracy sql.NullInt64
		if err := rows.Scan(&m.ID, &m.Slug, &m.Name, &m.Type, &m.Category,
			&power, &accuracy, &m.PP, &m.Priority, &m.Target, &m.Description); err != nil {
			return nil, err
		}
		if power.Valid {
			v := int(power.Int64); m.Power = &v
		}
		if accuracy.Valid {
			v := int(accuracy.Int64); m.Accuracy = &v
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
