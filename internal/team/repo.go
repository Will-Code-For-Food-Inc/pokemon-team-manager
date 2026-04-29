package team

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/user/pokemon-team-manager/internal/pokemon"
)

// Repo provides database access for team management.
type Repo struct {
	db      *sql.DB
	pokemon *pokemon.Repo
}

// NewRepo creates a new team Repo.
func NewRepo(db *sql.DB, pr *pokemon.Repo) *Repo {
	return &Repo{db: db, pokemon: pr}
}

// snapshotMutationKeep caps the number of mutation snapshots retained per team.
// Checkpoints are never auto-pruned. User-triggered prune passes its own keep N.
const snapshotMutationKeep = 50

// snapshot writes a JSON dump of the team's current state to team_snapshots.
// Best-effort: errors are logged via slog but never bubble up — a snapshot
// failure cannot break a user's mutation. After a 'mutation' write, older
// mutation rows beyond snapshotMutationKeep are pruned; checkpoints survive.
//
// label is a free-form human-readable hint ("set_moves: Hatterene"); created_by
// is currently always "system" — agent vs user attribution is a follow-up.
func (r *Repo) snapshot(teamID int, kind, label, by string) {
	if teamID <= 0 {
		return
	}
	t, err := r.GetTeam(teamID)
	if err != nil {
		slog.Warn("snapshot: get_team failed", "team_id", teamID, "err", err)
		return
	}
	payload, err := json.Marshal(t)
	if err != nil {
		slog.Warn("snapshot: marshal failed", "team_id", teamID, "err", err)
		return
	}
	if _, err := r.db.Exec(
		`INSERT INTO team_snapshots(team_id, kind, label, payload, created_by) VALUES(?, ?, ?, ?, ?)`,
		teamID, kind, label, string(payload), by); err != nil {
		slog.Warn("snapshot: insert failed", "team_id", teamID, "err", err)
		return
	}
	if kind == "mutation" {
		_, _ = r.db.Exec(`
			DELETE FROM team_snapshots
			WHERE team_id = ? AND kind = 'mutation'
			  AND id NOT IN (
			      SELECT id FROM team_snapshots
			      WHERE team_id = ? AND kind = 'mutation'
			      ORDER BY id DESC LIMIT ?
			  )`, teamID, teamID, snapshotMutationKeep)
	}
}

// snapshotByConfig is a convenience for repo methods that take a configID
// rather than a teamID. Looks up the owning team, then snapshots.
func (r *Repo) snapshotByConfig(configID int, kind, label, by string) {
	var teamID int
	if err := r.db.QueryRow(
		`SELECT team_id FROM team_slots WHERE config_id=?`, configID).Scan(&teamID); err != nil {
		// Config might have been deleted (e.g. by RemoveMember); silently skip.
		return
	}
	r.snapshot(teamID, kind, label, by)
}

// ── Teams ─────────────────────────────────────────────────────────────────────

// CreateTeam inserts a new team and returns its ID.
func (r *Repo) CreateTeam(name, regulation string) (int64, error) {
	res, err := r.db.Exec(`INSERT INTO teams(name, regulation) VALUES(?, ?)`, name, regulation)
	if err != nil {
		return 0, fmt.Errorf("CreateTeam: %w", err)
	}
	return res.LastInsertId()
}

// DeleteTeam removes a team and all its slots (configs are cascade-deleted via team_slots).
func (r *Repo) DeleteTeam(id int) error {
	_, err := r.db.Exec(`DELETE FROM teams WHERE id = ?`, id)
	return err
}

// RenameTeam changes a team's name.
func (r *Repo) RenameTeam(id int, name string) error {
	if _, err := r.db.Exec(`UPDATE teams SET name=?, updated_at=datetime('now') WHERE id=?`, name, id); err != nil {
		return err
	}
	r.snapshot(id, "mutation", "rename_team", "system")
	return nil
}

// UpdateTeamStrategy sets the strategy field on a team.
func (r *Repo) UpdateTeamStrategy(id int, strategy string) error {
	if _, err := r.db.Exec(`UPDATE teams SET strategy=?, updated_at=datetime('now') WHERE id=?`, strategy, id); err != nil {
		return err
	}
	r.snapshot(id, "mutation", "set_team_notes", "system")
	return nil
}

// ListTeams returns all teams with member counts.
func (r *Repo) ListTeams() ([]TeamSummary, error) {
	rows, err := r.db.Query(`
		SELECT t.id, t.name, t.regulation, t.strategy, t.created_at,
		       COUNT(ts.id) AS member_count
		FROM teams t
		LEFT JOIN team_slots ts ON ts.team_id = t.id
		GROUP BY t.id ORDER BY t.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TeamSummary
	for rows.Next() {
		var s TeamSummary
		var ca string
		if err := rows.Scan(&s.ID, &s.Name, &s.Regulation, &s.Strategy, &ca, &s.MemberCount); err != nil {
			return nil, err
		}
		s.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", ca)
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetTeam loads a team and all its members with fully-resolved configs.
func (r *Repo) GetTeam(id int) (*Team, error) {
	var t Team
	var ca, ua string
	err := r.db.QueryRow(
		`SELECT id, name, regulation, strategy, created_at, updated_at FROM teams WHERE id=?`, id).
		Scan(&t.ID, &t.Name, &t.Regulation, &t.Strategy, &ca, &ua)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("team %d not found", id)
	}
	if err != nil {
		return nil, err
	}
	t.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", ca)
	t.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", ua)

	var buf bytes.Buffer
	if mdErr := goldmark.Convert([]byte(t.Strategy), &buf); mdErr == nil {
		t.StrategyHTML = buf.String()
	}

	members, err := r.loadMembers(id)
	if err != nil {
		return nil, err
	}
	t.Members = members
	return &t, nil
}

func (r *Repo) loadMembers(teamID int) ([]Member, error) {
	rows, err := r.db.Query(
		`SELECT slot, config_id FROM team_slots WHERE team_id=? ORDER BY slot`, teamID)
	if err != nil {
		return nil, err
	}
	type slotRow struct{ slot, configID int }
	var slots []slotRow
	for rows.Next() {
		var s slotRow
		if err := rows.Scan(&s.slot, &s.configID); err != nil {
			rows.Close()
			return nil, err
		}
		slots = append(slots, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	members := make([]Member, len(slots))
	for i, s := range slots {
		cfg, err := r.GetConfig(s.configID)
		if err != nil {
			return nil, err
		}
		members[i] = Member{Slot: s.slot, ConfigID: s.configID, Config: cfg}
	}
	return members, nil
}

// ── Configs ───────────────────────────────────────────────────────────────────

// CreateConfig inserts a new pokemon config and returns its ID.
func (r *Repo) CreateConfig(speciesSlug, abilitySlug string) (int64, error) {
	res, err := r.db.Exec(
		`INSERT INTO pokemon_configs(species_slug, ability_slug) VALUES(?, ?)`,
		speciesSlug, abilitySlug)
	if err != nil {
		return 0, fmt.Errorf("CreateConfig: %w", err)
	}
	return res.LastInsertId()
}

// GetConfig loads a fully-resolved config (species, ability, item, moves).
func (r *Repo) GetConfig(id int) (*Config, error) {
	var c Config
	var speciesSlug, abilitySlug string
	var itemSlug sql.NullString
	var ca, ua string
	err := r.db.QueryRow(`
		SELECT id, species_slug, nickname, nature, ability_slug, item_slug,
		       tera_type, role, notes,
		       sp_hp, sp_atk, sp_def, sp_spa, sp_spd, sp_spe,
		       created_at, updated_at
		FROM pokemon_configs WHERE id=?`, id).Scan(
		&c.ID, &speciesSlug, &c.Nickname, &c.Nature, &abilitySlug, &itemSlug,
		&c.TeraType, &c.Role, &c.Notes,
		&c.EVs.HP, &c.EVs.Atk, &c.EVs.Def, &c.EVs.SpA, &c.EVs.SpD, &c.EVs.Spe,
		&ca, &ua)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("config %d not found", id)
	}
	if err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", ca)
	c.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", ua)

	if sp, err := r.pokemon.GetSpeciesBySlug(speciesSlug); err == nil {
		c.Species = sp
	}

	var ab pokemon.Ability
	if err := r.db.QueryRow(
		`SELECT id, name, slug, description FROM abilities WHERE slug=?`, abilitySlug).
		Scan(&ab.ID, &ab.Name, &ab.Slug, &ab.Description); err == nil {
		c.Ability = &ab
	}

	if itemSlug.Valid {
		var it pokemon.Item
		if err := r.db.QueryRow(
			`SELECT id, name, slug, description, is_banned, vp_cost FROM items WHERE slug=?`, itemSlug.String).
			Scan(&it.ID, &it.Name, &it.Slug, &it.Description, &it.IsBanned, &it.VPCost); err == nil {
			c.Item = &it
		}
	}

	mrows, err := r.db.Query(`
		SELECT mv.id, mv.name, mv.slug, mv.type, mv.category,
		       mv.power, mv.accuracy, mv.pp, mv.priority, mv.target, mv.description
		FROM config_moves cm
		JOIN moves mv ON mv.slug = cm.move_slug
		WHERE cm.config_id=? ORDER BY cm.slot`, id)
	if err == nil {
		defer mrows.Close()
		for mrows.Next() {
			var mv pokemon.Move
			var power, accuracy sql.NullInt64
			if err := mrows.Scan(&mv.ID, &mv.Name, &mv.Slug, &mv.Type, &mv.Category,
				&power, &accuracy, &mv.PP, &mv.Priority, &mv.Target, &mv.Description); err == nil {
				if power.Valid {
					v := int(power.Int64); mv.Power = &v
				}
				if accuracy.Valid {
					v := int(accuracy.Int64); mv.Accuracy = &v
				}
				c.Moves = append(c.Moves, &mv)
			}
		}
	}
	return &c, nil
}

// configUpdate applies a single-column UPDATE to pokemon_configs.
func (r *Repo) configUpdate(configID int, col string, val any) error {
	_, err := r.db.Exec(
		`UPDATE pokemon_configs SET `+col+`=?, updated_at=datetime('now') WHERE id=?`, val, configID)
	if err != nil {
		return fmt.Errorf("configUpdate %s: %w", col, err)
	}
	// Touch the owning team and write a history snapshot.
	var teamID int
	if err := r.db.QueryRow(
		`SELECT team_id FROM team_slots WHERE config_id=?`, configID).Scan(&teamID); err == nil {
		r.db.Exec(`UPDATE teams SET updated_at=datetime('now') WHERE id=?`, teamID)
		r.snapshot(teamID, "mutation", col, "system")
	}
	return nil
}

// SetAbility updates the ability for a config.
func (r *Repo) SetAbility(configID int, abilitySlug string) error {
	return r.configUpdate(configID, "ability_slug", abilitySlug)
}

// SetNature updates the nature for a config.
func (r *Repo) SetNature(configID int, nature string) error {
	return r.configUpdate(configID, "nature", nature)
}

// SetItem updates the held item for a config. Pass "" to clear.
func (r *Repo) SetItem(configID int, itemSlug string) error {
	if itemSlug == "" {
		if _, err := r.db.Exec(`UPDATE pokemon_configs SET item_slug=NULL, updated_at=datetime('now') WHERE id=?`, configID); err != nil {
			return err
		}
		r.snapshotByConfig(configID, "mutation", "clear_item", "system")
		return nil
	}
	return r.configUpdate(configID, "item_slug", itemSlug)
}

// SetMoves replaces all moves for a config. Accepts 1–4 move slugs.
func (r *Repo) SetMoves(configID int, moveSlugs []string) error {
	if len(moveSlugs) > 4 {
		return fmt.Errorf("a Pokemon can only have up to 4 moves")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM config_moves WHERE config_id=?`, configID); err != nil {
		return err
	}
	for i, slug := range moveSlugs {
		if _, err := tx.Exec(
			`INSERT INTO config_moves(config_id, slot, move_slug) VALUES(?,?,?)`,
			configID, i+1, slug); err != nil {
			return err
		}
	}
	tx.Exec(`UPDATE pokemon_configs SET updated_at=datetime('now') WHERE id=?`, configID)
	if err := tx.Commit(); err != nil {
		return err
	}
	r.snapshotByConfig(configID, "mutation", "set_moves", "system")
	return nil
}

// SetEVs updates the SP spread for a config.
func (r *Repo) SetEVs(configID int, evs StatSpread) error {
	if _, err := r.db.Exec(
		`UPDATE pokemon_configs SET sp_hp=?,sp_atk=?,sp_def=?,sp_spa=?,sp_spd=?,sp_spe=?,updated_at=datetime('now') WHERE id=?`,
		evs.HP, evs.Atk, evs.Def, evs.SpA, evs.SpD, evs.Spe, configID); err != nil {
		return err
	}
	r.snapshotByConfig(configID, "mutation", "set_stats", "system")
	return nil
}

// SetRole updates the role label for a config.
func (r *Repo) SetRole(configID int, role string) error {
	return r.configUpdate(configID, "role", role)
}

// SetConfigNotes updates the notes for a config.
func (r *Repo) SetConfigNotes(configID int, notes string) error {
	return r.configUpdate(configID, "notes", notes)
}

// SetNickname updates the nickname for a config.
func (r *Repo) SetNickname(configID int, nickname string) error {
	return r.configUpdate(configID, "nickname", nickname)
}

// SetTeraType updates the Tera type for a config.
func (r *Repo) SetTeraType(configID int, teraType string) error {
	return r.configUpdate(configID, "tera_type", teraType)
}

// ── Team slots ────────────────────────────────────────────────────────────────

// AddMember creates a config for the given species+ability and assigns it to
// the next available slot on the team.
func (r *Repo) AddMember(teamID int, speciesSlug, abilitySlug string) (configID int64, err error) {
	var maxSlot int
	r.db.QueryRow(`SELECT COALESCE(MAX(slot),0) FROM team_slots WHERE team_id=?`, teamID).Scan(&maxSlot)
	if maxSlot >= 6 {
		return 0, fmt.Errorf("team is full (6 Pokemon maximum)")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO pokemon_configs(species_slug, ability_slug) VALUES(?, ?)`,
		speciesSlug, abilitySlug)
	if err != nil {
		return 0, fmt.Errorf("AddMember config: %w", err)
	}
	configID, _ = res.LastInsertId()

	if _, err := tx.Exec(
		`INSERT INTO team_slots(team_id, slot, config_id) VALUES(?,?,?)`,
		teamID, maxSlot+1, configID); err != nil {
		return 0, fmt.Errorf("AddMember slot: %w", err)
	}
	tx.Exec(`UPDATE teams SET updated_at=datetime('now') WHERE id=?`, teamID)
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	r.snapshot(teamID, "mutation", "add_pokemon", "system")
	return configID, nil
}

// RemoveMember removes the slot containing the given config and re-slots remaining members.
func (r *Repo) RemoveMember(configID int) error {
	var teamID int
	if err := r.db.QueryRow(
		`SELECT team_id FROM team_slots WHERE config_id=?`, configID).Scan(&teamID); err != nil {
		return fmt.Errorf("config %d not on any team", configID)
	}
	if _, err := r.db.Exec(`DELETE FROM team_slots WHERE config_id=?`, configID); err != nil {
		return err
	}
	// config_moves cascade from pokemon_configs, which cascades from team_slots.
	if _, err := r.db.Exec(`DELETE FROM pokemon_configs WHERE id=?`, configID); err != nil {
		return err
	}
	r.reSlot(teamID)
	r.db.Exec(`UPDATE teams SET updated_at=datetime('now') WHERE id=?`, teamID)
	r.snapshot(teamID, "mutation", "remove_pokemon", "system")
	return nil
}

// ReplaceMember atomically swaps the species at (teamID, slot) for a fresh
// config built from speciesSlug + abilitySlug. The slot ordering is preserved.
// All previous config (nature, item, moves, stats, role, nickname, notes) is
// discarded — none of it transfers cleanly across species, and the model can
// re-set anything it wants on the new config_id this returns.
func (r *Repo) ReplaceMember(teamID, slot int, speciesSlug, abilitySlug string) (int64, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var oldConfigID int
	if err := tx.QueryRow(
		`SELECT config_id FROM team_slots WHERE team_id=? AND slot=?`,
		teamID, slot).Scan(&oldConfigID); err != nil {
		return 0, fmt.Errorf("ReplaceMember: slot %d on team %d not found", slot, teamID)
	}

	res, err := tx.Exec(
		`INSERT INTO pokemon_configs(species_slug, ability_slug) VALUES(?, ?)`,
		speciesSlug, abilitySlug)
	if err != nil {
		return 0, fmt.Errorf("ReplaceMember new config: %w", err)
	}
	newConfigID, _ := res.LastInsertId()

	if _, err := tx.Exec(
		`UPDATE team_slots SET config_id=? WHERE team_id=? AND slot=?`,
		newConfigID, teamID, slot); err != nil {
		return 0, fmt.Errorf("ReplaceMember slot update: %w", err)
	}

	// Now safe to drop the orphaned old config; cascade clears its config_moves.
	if _, err := tx.Exec(`DELETE FROM pokemon_configs WHERE id=?`, oldConfigID); err != nil {
		return 0, fmt.Errorf("ReplaceMember old config delete: %w", err)
	}

	tx.Exec(`UPDATE teams SET updated_at=datetime('now') WHERE id=?`, teamID)
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	r.snapshot(teamID, "mutation", "replace_pokemon", "system")
	return newConfigID, nil
}

// SwapSlots swaps two slot numbers on the same team.
func (r *Repo) SwapSlots(teamID, slotA, slotB int) error {
	var idA, idB int
	if err := r.db.QueryRow(
		`SELECT id FROM team_slots WHERE team_id=? AND slot=?`, teamID, slotA).Scan(&idA); err != nil {
		return fmt.Errorf("slot %d not found", slotA)
	}
	if err := r.db.QueryRow(
		`SELECT id FROM team_slots WHERE team_id=? AND slot=?`, teamID, slotB).Scan(&idB); err != nil {
		return fmt.Errorf("slot %d not found", slotB)
	}
	// Swap via a temporary slot value to avoid UNIQUE conflict.
	r.db.Exec(`PRAGMA ignore_check_constraints = ON`)
	r.db.Exec(`UPDATE team_slots SET slot=7 WHERE id=?`, idA)
	r.db.Exec(`UPDATE team_slots SET slot=? WHERE id=?`, slotA, idB)
	r.db.Exec(`UPDATE team_slots SET slot=? WHERE id=?`, slotB, idA)
	r.db.Exec(`PRAGMA ignore_check_constraints = OFF`)
	r.db.Exec(`UPDATE teams SET updated_at=datetime('now') WHERE id=?`, teamID)
	r.snapshot(teamID, "mutation", "swap_slots", "system")
	return nil
}

// CopyTeam duplicates a team and all its configs under a new name.
func (r *Repo) CopyTeam(srcID int, newName string) (int64, error) {
	src, err := r.GetTeam(srcID)
	if err != nil {
		return 0, err
	}
	newID, err := r.CreateTeam(newName, src.Regulation)
	if err != nil {
		return 0, err
	}
	r.db.Exec(`UPDATE teams SET strategy=? WHERE id=?`, src.Strategy, newID)

	for _, m := range src.Members {
		if m.Config == nil || m.Config.Species == nil {
			continue
		}
		c := m.Config
		abilitySlug := ""
		if c.Ability != nil {
			abilitySlug = c.Ability.Slug
		}
		newCfgID, err := r.AddMember(int(newID), c.Species.Slug, abilitySlug)
		if err != nil {
			continue
		}
		nid := int(newCfgID)
		r.SetNature(nid, c.Nature)
		r.SetRole(nid, c.Role)
		r.SetConfigNotes(nid, c.Notes)
		r.SetNickname(nid, c.Nickname)
		r.SetTeraType(nid, c.TeraType)
		r.SetEVs(nid, c.EVs)
		if c.Item != nil {
			r.SetItem(nid, c.Item.Slug)
		}
		var moveSlugs []string
		for _, mv := range c.Moves {
			if mv != nil {
				moveSlugs = append(moveSlugs, mv.Slug)
			}
		}
		if len(moveSlugs) > 0 {
			r.SetMoves(nid, moveSlugs)
		}
	}
	return newID, nil
}

// GetConfigByTeamAndSpecies returns the config ID for a species on a team.
func (r *Repo) GetConfigByTeamAndSpecies(teamID int, speciesName string) (int, error) {
	var configID int
	err := r.db.QueryRow(`
		SELECT ts.config_id FROM team_slots ts
		JOIN pokemon_configs pc ON pc.id = ts.config_id
		WHERE ts.team_id=? AND lower(pc.species_slug)=lower(?)`, teamID, strings.ToLower(speciesName)).
		Scan(&configID)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("%s is not on team %d", speciesName, teamID)
	}
	return configID, err
}

// ── Regulations ───────────────────────────────────────────────────────────────

// GetRegulation loads a regulation with its banned/restricted species and move bans.
func (r *Repo) GetRegulation(id string) (*Regulation, error) {
	var reg Regulation
	err := r.db.QueryRow(
		`SELECT id, name, max_restricted, description FROM regulations WHERE id=?`, id).
		Scan(&reg.ID, &reg.Name, &reg.MaxRestricted, &reg.Description)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("regulation %q not found", id)
	}
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(
		`SELECT species_slug, rule FROM regulation_species_rules WHERE regulation_id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var slug, rule string
		rows.Scan(&slug, &rule)
		if rule == "banned" {
			reg.BannedSlugs = append(reg.BannedSlugs, slug)
		} else {
			reg.RestrictedSlugs = append(reg.RestrictedSlugs, slug)
		}
	}

	mrows, err := r.db.Query(
		`SELECT move_slug FROM regulation_move_bans WHERE regulation_id=?`, id)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var slug string
		mrows.Scan(&slug)
		reg.BannedMoveSlugs = append(reg.BannedMoveSlugs, slug)
	}

	return NewRegulation(reg.ID, reg.Name, reg.Description, reg.MaxRestricted,
		reg.BannedSlugs, reg.RestrictedSlugs, reg.BannedMoveSlugs), nil
}

// ListRegulations returns all regulation summaries.
func (r *Repo) ListRegulations() ([]RegulationSummary, error) {
	rows, err := r.db.Query(
		`SELECT id, name, start_date, end_date, description, max_restricted, active FROM regulations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RegulationSummary
	for rows.Next() {
		var s RegulationSummary
		var start, end sql.NullString
		rows.Scan(&s.ID, &s.Name, &start, &end, &s.Description, &s.MaxRestricted, &s.Active)
		s.StartDate = start.String
		s.EndDate = end.String
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListActiveRegulations returns only regulations marked active.
func (r *Repo) ListActiveRegulations() ([]RegulationSummary, error) {
	rows, err := r.db.Query(
		`SELECT id, name, start_date, end_date, description, max_restricted, active FROM regulations WHERE active=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RegulationSummary
	for rows.Next() {
		var s RegulationSummary
		var start, end sql.NullString
		rows.Scan(&s.ID, &s.Name, &start, &end, &s.Description, &s.MaxRestricted, &s.Active)
		s.StartDate = start.String
		s.EndDate = end.String
		out = append(out, s)
	}
	return out, rows.Err()
}

// SetRegulationActive enables or disables a regulation.
func (r *Repo) SetRegulationActive(id string, active bool) error {
	v := 0
	if active {
		v = 1
	}
	_, err := r.db.Exec(`UPDATE regulations SET active=? WHERE id=?`, v, id)
	return err
}

// RegulationSummary is a lightweight view of a regulation.
type RegulationSummary struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
	Description   string `json:"description"`
	MaxRestricted int    `json:"max_restricted"`
	Active        bool   `json:"active"`
}

// ── Logs ──────────────────────────────────────────────────────────────────────

// AddLog inserts a battle/session journal entry for a team.
func (r *Repo) AddLog(teamID int, entry string) (int64, error) {
	res, err := r.db.Exec(
		`INSERT INTO team_logs(team_id, entry) VALUES(?, ?)`, teamID, entry)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetLogs returns all journal entries for a team, newest first.
func (r *Repo) GetLogs(teamID int) ([]TeamLog, error) {
	rows, err := r.db.Query(
		`SELECT id, team_id, entry, created_at FROM team_logs WHERE team_id=? ORDER BY created_at DESC`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []TeamLog
	for rows.Next() {
		var l TeamLog
		if err := rows.Scan(&l.ID, &l.TeamID, &l.Entry, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// ── Snapshot history ──────────────────────────────────────────────────────────

// TeamSnapshot is a row from the team_snapshots table.
type TeamSnapshot struct {
	ID        int64     `json:"id"`
	TeamID    int       `json:"team_id"`
	Kind      string    `json:"kind"`  // "mutation" | "checkpoint"
	Label     string    `json:"label"`
	Payload   string    `json:"payload"` // JSON of the team at that moment
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
}

// snapshotTimeLayouts lists the formats SQLite emits via datetime('now') —
// ISO-8601 with optional fractional seconds. We try each in order.
var snapshotTimeLayouts = []string{
	"2006-01-02 15:04:05",
	"2006-01-02 15:04:05.999999999",
	time.RFC3339Nano,
	time.RFC3339,
}

func parseSnapshotTime(s string) time.Time {
	for _, layout := range snapshotTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ListTeamSnapshots returns snapshots for a team, newest first, paginated.
func (r *Repo) ListTeamSnapshots(teamID, limit, offset int) ([]TeamSnapshot, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(
		`SELECT id, team_id, kind, label, payload, created_at, created_by
		 FROM team_snapshots WHERE team_id=?
		 ORDER BY id DESC LIMIT ? OFFSET ?`, teamID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TeamSnapshot
	for rows.Next() {
		var s TeamSnapshot
		var createdAt string
		if err := rows.Scan(&s.ID, &s.TeamID, &s.Kind, &s.Label, &s.Payload, &createdAt, &s.CreatedBy); err != nil {
			return nil, err
		}
		s.CreatedAt = parseSnapshotTime(createdAt)
		out = append(out, s)
	}
	return out, rows.Err()
}

// CountTeamSnapshots returns the total number of snapshots for a team.
func (r *Repo) CountTeamSnapshots(teamID int) (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM team_snapshots WHERE team_id=?`, teamID).Scan(&n)
	return n, err
}

// GetTeamSnapshot returns one snapshot by id.
func (r *Repo) GetTeamSnapshot(snapshotID int64) (*TeamSnapshot, error) {
	var s TeamSnapshot
	var createdAt string
	err := r.db.QueryRow(
		`SELECT id, team_id, kind, label, payload, created_at, created_by
		 FROM team_snapshots WHERE id=?`, snapshotID).
		Scan(&s.ID, &s.TeamID, &s.Kind, &s.Label, &s.Payload, &createdAt, &s.CreatedBy)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("snapshot %d not found", snapshotID)
	}
	if err != nil {
		return nil, err
	}
	s.CreatedAt = parseSnapshotTime(createdAt)
	return &s, nil
}

// PruneTeamSnapshots is the user-only path that drops mutation snapshots
// beyond keepLastN. Checkpoints are never touched. Returns the number of
// rows deleted.
//
// This is intentionally not callable from any agent tool — pruning is a
// destructive operation that mirrors the team-deletion boundary.
func (r *Repo) PruneTeamSnapshots(teamID, keepLastN int) (int64, error) {
	if keepLastN < 0 {
		keepLastN = 0
	}
	res, err := r.db.Exec(`
		DELETE FROM team_snapshots
		WHERE team_id = ? AND kind = 'mutation'
		  AND id NOT IN (
		      SELECT id FROM team_snapshots
		      WHERE team_id = ? AND kind = 'mutation'
		      ORDER BY id DESC LIMIT ?
		  )`, teamID, teamID, keepLastN)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (r *Repo) reSlot(teamID int) {
	rows, _ := r.db.Query(`SELECT id FROM team_slots WHERE team_id=? ORDER BY slot`, teamID)
	if rows == nil {
		return
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	for i, id := range ids {
		r.db.Exec(`UPDATE team_slots SET slot=? WHERE id=?`, i+1, id)
	}
}
