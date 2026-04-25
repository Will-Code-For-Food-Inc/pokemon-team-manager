// Command seed-team registers the initial test team from the April 24 planning
// session into the database. Run after `ptm seed`.
//
// Usage: ./seed-team [--db ptm.db]
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	ptmdb "github.com/user/pokemon-team-manager/internal/db"
	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

func main() {
	var dbPath string
	root := &cobra.Command{
		Use:   "seed-team",
		Short: "Register the April 24 test team into the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := ptmdb.Open(dbPath)
			if err != nil {
				return fmt.Errorf("open db: %w", err)
			}
			defer db.Close()
			if err := ptmdb.Seed(db, "data"); err != nil {
				return fmt.Errorf("seed: %w", err)
			}
			return registerTeam(db)
		},
	}
	root.Flags().StringVar(&dbPath, "db", "ptm.db", "Path to SQLite database")
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func registerTeam(db *sql.DB) error {
	pr := pokemon.NewRepo(db)
	tr := team.NewRepo(db, pr)

	// Create the team.
	teamID, err := tr.CreateTeam("Apr 24 Sand Team", "H")
	if err != nil {
		return fmt.Errorf("create team: %w", err)
	}
	fmt.Printf("Created team ID %d\n", teamID)

	type memberDef struct {
		species  string
		ability  string
		item     string
		nature   string
		evStats  []string // 2 or 3 stats for set_stats distribution
		role     string
		notes    string
		moves    []string
	}

	members := []memberDef{
		{
			species: "Tyranitar",
			ability: "Sand Stream",
			item:    "Leftovers",
			nature:  "Adamant",
			evStats: []string{"attack", "hp"},
			role:    "sand_setter",
			notes:   "Sand setter. Leads most matchups. Rock/Dark offensive presence.",
			moves:   []string{"Crunch", "Rock Slide", "Earthquake", "Protect"},
		},
		{
			species: "Sandaconda",
			ability: "Sand Spit",
			item:    "",
			nature:  "Impish",
			evStats: []string{"defense", "hp"},
			role:    "sand_support",
			notes:   "Sand support and physical wall. Can set up with Coil or spread paralysis with Glare.",
			moves:   []string{"Coil", "Body Press", "Glare", "Protect"},
		},
		{
			species: "Aggron",
			ability: "Sturdy",
			item:    "Shuca Berry",
			nature:  "Relaxed",
			evStats: []string{"defense", "hp"},
			role:    "physical_wall",
			notes:   "Iron Defense + Body Press sweeper. Shuca Berry neutralises 4× Ground weakness. Relaxed nature for Trick Room speed inversion potential.",
			moves:   []string{"Iron Defense", "Body Press", "Heavy Slam", "Protect"},
		},
		{
			species: "Dragonite",
			ability: "Multiscale",
			item:    "",
			nature:  "Adamant",
			evStats: []string{"attack", "speed"},
			role:    "physical_sweeper",
			notes:   "Backline physical threat. Dragon Dance setup or Extreme Speed priority. Multiscale lets it take a hit before setting up.",
			moves:   []string{"Dragon Dance", "Extreme Speed", "Outrage", "Ice Punch"},
		},
		{
			species: "Arcanine",
			ability: "Intimidate",
			item:    "",
			nature:  "Timid",
			evStats: []string{"speed", "sp_attack"},
			role:    "flex_attacker",
			notes:   "Flexible firepower and Intimidate support. Brings in to soften physical attackers or add raw special damage.",
			moves:   []string{"Flare Blitz", "Flamethrower", "Protect", "Extreme Speed"},
		},
		{
			species: "Sylveon",
			ability: "Pixilate",
			item:    "Lum Berry",
			nature:  "Modest",
			evStats: []string{"hp", "sp_attack"},
			role:    "special_sweeper",
			notes:   "Pixilate converts Hyper Voice to Fairy-type STAB spread move. Calm Mind snowball. Lum Berry prevents Toxic stalling her out. Dragon immunity fills team gap.",
			moves:   []string{"Hyper Voice", "Moonblast", "Calm Mind", "Psychic"},
		},
	}

	evSpread := func(stats []string) team.StatSpread {
		var evs team.StatSpread
		if len(stats) == 0 {
			return evs
		}
		vals := make([]int, len(stats))
		switch len(stats) {
		case 2:
			vals[0], vals[1] = 252, 252
		case 3:
			vals[0], vals[1], vals[2] = 172, 172, 164
		}
		for i, s := range stats {
			switch s {
			case "hp":
				evs.HP = vals[i]
			case "attack":
				evs.Atk = vals[i]
			case "defense":
				evs.Def = vals[i]
			case "sp_attack":
				evs.SpA = vals[i]
			case "sp_defense":
				evs.SpD = vals[i]
			case "speed":
				evs.Spe = vals[i]
			}
		}
		return evs
	}

	for i, m := range members {
		slot := i + 1

		sp, err := pr.GetSpeciesByName(m.species)
		if err != nil {
			return fmt.Errorf("species %q not found (run `ptm seed` first): %w", m.species, err)
		}

		ab, err := pr.GetAbilityByName(m.ability)
		if err != nil {
			return fmt.Errorf("ability %q not found: %w", m.ability, err)
		}

		memberID, err := tr.AddMember(int(teamID), sp.ID, ab.ID)
		if err != nil {
			return fmt.Errorf("add member %s: %w", m.species, err)
		}
		mid := int(memberID)

		if err := tr.SetNature(mid, m.nature); err != nil {
			return fmt.Errorf("set nature %s: %w", m.species, err)
		}
		if err := tr.SetRole(mid, m.role); err != nil {
			return fmt.Errorf("set role %s: %w", m.species, err)
		}
		if err := tr.SetMemberNotes(mid, m.notes); err != nil {
			return fmt.Errorf("set notes %s: %w", m.species, err)
		}

		if m.item != "" {
			item, err := pr.GetItemByName(m.item)
			if err != nil {
				log.Printf("warn: item %q not found for %s, skipping", m.item, m.species)
			} else {
				if err := tr.SetItem(mid, item.ID); err != nil {
					return fmt.Errorf("set item %s: %w", m.species, err)
				}
			}
		}

		evs := evSpread(m.evStats)
		if err := tr.SetEVs(mid, evs); err != nil {
			return fmt.Errorf("set EVs %s: %w", m.species, err)
		}

		var moveIDs []int
		for _, moveName := range m.moves {
			mv, err := pr.GetMoveByName(moveName)
			if err != nil {
				log.Printf("warn: move %q not found for %s, skipping", moveName, m.species)
				continue
			}
			moveIDs = append(moveIDs, mv.ID)
		}
		if len(moveIDs) > 0 {
			if err := tr.SetMoves(mid, moveIDs); err != nil {
				return fmt.Errorf("set moves %s: %w", m.species, err)
			}
		}

		fmt.Printf("  Slot %d: %s (member ID %d)\n", slot, m.species, memberID)
	}

	fmt.Printf("\nTeam registered. View with: ptm team show %d\n", teamID)
	return nil
}
