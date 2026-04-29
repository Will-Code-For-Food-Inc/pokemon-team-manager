// Package db handles database initialization, migrations, and seeding.
package db

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

// Seed loads all CSV files from dataDir/pokemon/ into the database.
// Inserts are INSERT OR IGNORE (idempotent). Used for tests and CLI seeding.
func Seed(db *sql.DB, dataDir string) error {
	dir := filepath.Join(dataDir, "pokemon")
	for _, step := range seedSteps {
		path := filepath.Join(dir, step.file)
		if err := seedCSV(db, path, step.table, step.insert); err != nil {
			return fmt.Errorf("seed %s: %w", step.file, err)
		}
	}
	// Legacy JSON seeds: resolve PokeAPI internal IDs to slugs via JOIN.
	if err := seedSpeciesAbilitiesJSON(db, filepath.Join(dir, "species_abilities.json")); err != nil {
		return fmt.Errorf("seed species_abilities.json: %w", err)
	}
	if err := seedLearnsetJSON(db, filepath.Join(dir, "learnsets.json")); err != nil {
		return fmt.Errorf("seed learnsets.json: %w", err)
	}
	return nil
}

// seedSpeciesAbilitiesJSON seeds species_abilities from the legacy JSON format.
// species_id in the JSON is the national dex number; we join to get the slug.
func seedSpeciesAbilitiesJSON(db *sql.DB, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var rows []struct {
		SpeciesID int `json:"species_id"`
		AbilityID int `json:"ability_id"`
		Slot      int `json:"slot"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO species_abilities(species_slug, ability_slug, slot)
		SELECT s.slug, a.slug, ?
		FROM species s, abilities a
		WHERE s.dex_id=? AND s.form='' AND a.id=?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.Slot, r.SpeciesID, r.AbilityID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// seedLearnsetJSON seeds learnsets from the legacy JSON format.
func seedLearnsetJSON(db *sql.DB, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var rows []struct {
		SpeciesID   int    `json:"species_id"`
		MoveID      int    `json:"move_id"`
		LearnMethod string `json:"learn_method"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO learnsets(species_slug, move_slug)
		SELECT s.slug, m.slug
		FROM species s, moves m
		WHERE s.dex_id=? AND s.form='' AND m.id=?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.SpeciesID, r.MoveID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type seedStep struct {
	file   string
	table  string
	insert func(tx *sql.Tx, row []string) error
}

var seedSteps = []seedStep{
	{"species.csv", "species", insertSpecies},
	{"moves.csv", "moves", insertMove},
	{"abilities.csv", "abilities", insertAbility},
	{"items.csv", "items", insertItem},
	{"regulations.csv", "regulations", insertRegulation},
	{"champions_learnsets.csv", "champions_learnsets", insertChampionsLearnset},
	{"learnsets.csv", "learnsets", insertLearnset},
	{"species_abilities.csv", "species_abilities", insertSpeciesAbility},
	{"regulation_species.csv", "regulation_species", insertRegulationSpecies},
	{"mega_stones.csv", "mega_stones", insertMegaStone},
}

func seedCSV(db *sql.DB, path string, _ string, insert func(*sql.Tx, []string) error) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // skip optional files
		}
		return err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.LazyQuotes = true
	// Skip header row.
	if _, err := r.Read(); err != nil {
		return fmt.Errorf("read header: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read row: %w", err)
		}
		if err := insert(tx, row); err != nil {
			return fmt.Errorf("insert row %v: %w", row, err)
		}
	}
	return tx.Commit()
}

// col returns row[i] or "" if out of bounds.
func col(row []string, i int) string {
	if i < len(row) {
		return row[i]
	}
	return ""
}

// intCol parses row[i] as int, returns 0 on failure.
func intCol(row []string, i int) int {
	v, _ := strconv.Atoi(col(row, i))
	return v
}

// nullIntCol returns nil if empty, else *int.
func nullIntCol(row []string, i int) *int {
	s := col(row, i)
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &v
}

func insertSpecies(tx *sql.Tx, row []string) error {
	// slug,dex_id,name,form,type1,type2,hp,attack,defense,sp_attack,sp_defense,speed,
	// generation,is_legendary,is_mythical,is_final_evo,is_restricted
	_, err := tx.Exec(`INSERT OR IGNORE INTO species
		(slug,dex_id,name,form,type1,type2,hp,attack,defense,sp_attack,sp_defense,speed,
		 generation,is_legendary,is_mythical,is_final_evo,is_restricted)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		col(row, 0), intCol(row, 1), col(row, 2), col(row, 3),
		col(row, 4), col(row, 5),
		intCol(row, 6), intCol(row, 7), intCol(row, 8),
		intCol(row, 9), intCol(row, 10), intCol(row, 11),
		intCol(row, 12), intCol(row, 13), intCol(row, 14),
		intCol(row, 15), intCol(row, 16))
	return err
}

func insertMove(tx *sql.Tx, row []string) error {
	// id,name,slug,type,category,power,accuracy,pp,priority,target,description
	_, err := tx.Exec(`INSERT OR REPLACE INTO moves
		(id,name,slug,type,category,power,accuracy,pp,priority,target,description)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		intCol(row, 0), col(row, 1), col(row, 2),
		col(row, 3), col(row, 4),
		nullIntCol(row, 5), nullIntCol(row, 6),
		intCol(row, 7), intCol(row, 8),
		col(row, 9), col(row, 10))
	return err
}

func insertAbility(tx *sql.Tx, row []string) error {
	// id,name,slug,description
	_, err := tx.Exec(`INSERT OR IGNORE INTO abilities(id,name,slug,description) VALUES(?,?,?,?)`,
		intCol(row, 0), col(row, 1), col(row, 2), col(row, 3))
	return err
}

func insertItem(tx *sql.Tx, row []string) error {
	// id,name,slug,description,is_banned,vp_cost
	_, err := tx.Exec(`INSERT OR IGNORE INTO items(id,name,slug,description,is_banned,vp_cost) VALUES(?,?,?,?,?,?)`,
		intCol(row, 0), col(row, 1), col(row, 2), col(row, 3),
		intCol(row, 4), intCol(row, 5))
	return err
}

func insertRegulation(tx *sql.Tx, row []string) error {
	// id,name,start_date,end_date,description,max_restricted,active
	active := 1
	if len(row) > 6 {
		active = intCol(row, 6)
	}
	_, err := tx.Exec(`INSERT OR IGNORE INTO regulations(id,name,start_date,end_date,description,max_restricted,active) VALUES(?,?,?,?,?,?,?)`,
		col(row, 0), col(row, 1), col(row, 2), col(row, 3), col(row, 4), intCol(row, 5), active)
	return err
}

func insertChampionsLearnset(tx *sql.Tx, row []string) error {
	// species_slug,move_slug
	_, err := tx.Exec(`INSERT OR IGNORE INTO champions_learnsets(species_slug,move_slug) VALUES(?,?)`,
		col(row, 0), col(row, 1))
	return err
}

func insertLearnset(tx *sql.Tx, row []string) error {
	// species_slug,move_slug
	_, err := tx.Exec(`INSERT OR IGNORE INTO learnsets(species_slug,move_slug) VALUES(?,?)`,
		col(row, 0), col(row, 1))
	return err
}

func insertSpeciesAbility(tx *sql.Tx, row []string) error {
	// species_slug,ability_slug,slot
	_, err := tx.Exec(`INSERT OR IGNORE INTO species_abilities(species_slug,ability_slug,slot) VALUES(?,?,?)`,
		col(row, 0), col(row, 1), intCol(row, 2))
	return err
}

func insertRegulationSpecies(tx *sql.Tx, row []string) error {
	// regulation_id,species_slug
	_, err := tx.Exec(`INSERT OR IGNORE INTO regulation_species(regulation_id,species_slug) VALUES(?,?)`,
		col(row, 0), col(row, 1))
	return err
}

func insertMegaStone(tx *sql.Tx, row []string) error {
	// pokemon_slug,stone_name,type1_override,type2_override,mega_ability
	_, err := tx.Exec(`INSERT OR IGNORE INTO mega_stones(pokemon_slug,stone_name,type1_override,type2_override,mega_ability) VALUES(?,?,?,?,?)`,
		col(row, 0), col(row, 1), col(row, 2), col(row, 3), col(row, 4))
	return err
}

