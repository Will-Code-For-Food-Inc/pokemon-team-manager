package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// seedSpecies is the JSON representation used in data/pokemon/species.json.
type seedSpecies struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Form         string `json:"form"`
	Type1        string `json:"type1"`
	Type2        string `json:"type2"`
	HP           int    `json:"hp"`
	Attack       int    `json:"attack"`
	Defense      int    `json:"defense"`
	SpAttack     int    `json:"sp_attack"`
	SpDefense    int    `json:"sp_defense"`
	Speed        int    `json:"speed"`
	Generation   int    `json:"generation"`
	IsLegendary  int    `json:"is_legendary"`
	IsMythical   int    `json:"is_mythical"`
	IsFinalEvo   int    `json:"is_final_evo"`
	IsRestricted int    `json:"is_restricted"`
}

type seedMove struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Category    string `json:"category"`
	Power       *int   `json:"power"`
	Accuracy    *int   `json:"accuracy"`
	PP          int    `json:"pp"`
	Priority    int    `json:"priority"`
	Target      string `json:"target"`
	Description string `json:"description"`
}

type seedAbility struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type seedItem struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsBanned    int    `json:"is_banned"`
}

type seedLearnset struct {
	SpeciesID   int    `json:"species_id"`
	MoveID      int    `json:"move_id"`
	LearnMethod string `json:"learn_method"`
}

type seedSpeciesAbility struct {
	SpeciesID int `json:"species_id"`
	AbilityID int `json:"ability_id"`
	Slot      int `json:"slot"`
}

type seedRegulation struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
	Description   string `json:"description"`
	MaxRestricted int    `json:"max_restricted"`
}

// Seed loads all JSON files from dataDir/pokemon/ and dataDir/knowledge/ and
// inserts them into the database. All inserts are INSERT OR IGNORE so the
// operation is idempotent.
func Seed(db *sql.DB, dataDir string) error {
	pokemonDir := filepath.Join(dataDir, "pokemon")

	if err := seedFile(db, filepath.Join(pokemonDir, "species.json"), seedSpeciesRows); err != nil {
		return fmt.Errorf("seed species: %w", err)
	}
	if err := seedFile(db, filepath.Join(pokemonDir, "abilities.json"), seedAbilityRows); err != nil {
		return fmt.Errorf("seed abilities: %w", err)
	}
	if err := seedFile(db, filepath.Join(pokemonDir, "moves.json"), seedMoveRows); err != nil {
		return fmt.Errorf("seed moves: %w", err)
	}
	if err := seedFile(db, filepath.Join(pokemonDir, "items.json"), seedItemRows); err != nil {
		return fmt.Errorf("seed items: %w", err)
	}
	if err := seedFile(db, filepath.Join(pokemonDir, "learnsets.json"), seedLearnsetRows); err != nil {
		return fmt.Errorf("seed learnsets: %w", err)
	}
	if err := seedFile(db, filepath.Join(pokemonDir, "species_abilities.json"), seedSpeciesAbilityRows); err != nil {
		return fmt.Errorf("seed species_abilities: %w", err)
	}
	if err := seedFile(db, filepath.Join(pokemonDir, "regulations.json"), seedRegulationRows); err != nil {
		return fmt.Errorf("seed regulations: %w", err)
	}
	return nil
}

func seedFile[T any](db *sql.DB, path string, fn func(*sql.Tx, []T) error) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // skip missing files gracefully
		}
		return err
	}
	var rows []T
	if err := json.Unmarshal(data, &rows); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx, rows); err != nil {
		return err
	}
	return tx.Commit()
}

func seedSpeciesRows(tx *sql.Tx, rows []seedSpecies) error {
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO species
		(id,name,form,type1,type2,hp,attack,defense,sp_attack,sp_defense,speed,
		 generation,is_legendary,is_mythical,is_final_evo,is_restricted)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.ID, r.Name, r.Form, r.Type1, r.Type2,
			r.HP, r.Attack, r.Defense, r.SpAttack, r.SpDefense, r.Speed,
			r.Generation, r.IsLegendary, r.IsMythical, r.IsFinalEvo, r.IsRestricted); err != nil {
			return err
		}
	}
	return nil
}

func seedMoveRows(tx *sql.Tx, rows []seedMove) error {
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO moves
		(id,name,type,category,power,accuracy,pp,priority,target,description)
		VALUES(?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.ID, r.Name, r.Type, r.Category,
			r.Power, r.Accuracy, r.PP, r.Priority, r.Target, r.Description); err != nil {
			return err
		}
	}
	return nil
}

func seedAbilityRows(tx *sql.Tx, rows []seedAbility) error {
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO abilities(id,name,description) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.ID, r.Name, r.Description); err != nil {
			return err
		}
	}
	return nil
}

func seedItemRows(tx *sql.Tx, rows []seedItem) error {
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO items(id,name,description,is_banned) VALUES(?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.ID, r.Name, r.Description, r.IsBanned); err != nil {
			return err
		}
	}
	return nil
}

func seedLearnsetRows(tx *sql.Tx, rows []seedLearnset) error {
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO learnsets(species_id,move_id,learn_method) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.SpeciesID, r.MoveID, r.LearnMethod); err != nil {
			return err
		}
	}
	return nil
}

func seedSpeciesAbilityRows(tx *sql.Tx, rows []seedSpeciesAbility) error {
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO species_abilities(species_id,ability_id,slot) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.SpeciesID, r.AbilityID, r.Slot); err != nil {
			return err
		}
	}
	return nil
}

func seedRegulationRows(tx *sql.Tx, rows []seedRegulation) error {
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO regulations(id,name,start_date,end_date,description,max_restricted)
		VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.ID, r.Name, r.StartDate, r.EndDate, r.Description, r.MaxRestricted); err != nil {
			return err
		}
	}
	return nil
}
