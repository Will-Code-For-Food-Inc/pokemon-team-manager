package team

import (
	"database/sql"
	"fmt"
	"time"

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

// CreateTeam inserts a new team and returns its ID.
func (r *Repo) CreateTeam(name, regulation string) (int64, error) {
	res, err := r.db.Exec(
		`INSERT INTO teams(name, regulation) VALUES(?, ?)`, name, regulation)
	if err != nil {
		return 0, fmt.Errorf("CreateTeam: %w", err)
	}
	return res.LastInsertId()
}

// DeleteTeam deletes a team and all its members (CASCADE).
func (r *Repo) DeleteTeam(id int) error {
	_, err := r.db.Exec(`DELETE FROM teams WHERE id = ?`, id)
	return err
}

// UpdateTeamNotes sets the notes field on a team.
func (r *Repo) UpdateTeamNotes(id int, notes string) error {
	_, err := r.db.Exec(`UPDATE teams SET notes=?, updated_at=datetime('now') WHERE id=?`, notes, id)
	return err
}

// ListTeams returns all teams with member counts.
func (r *Repo) ListTeams() ([]TeamSummary, error) {
	rows, err := r.db.Query(`
		SELECT t.id, t.name, t.regulation, t.notes, t.created_at,
		       COUNT(tm.id) as member_count
		FROM teams t
		LEFT JOIN team_members tm ON tm.team_id = t.id
		GROUP BY t.id ORDER BY t.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TeamSummary
	for rows.Next() {
		var s TeamSummary
		var ca string
		if err := rows.Scan(&s.ID, &s.Name, &s.Regulation, &s.Notes, &ca, &s.MemberCount); err != nil {
			return nil, err
		}
		s.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", ca)
		out = append(out, s)
	}
	return out, rows.Err()
}

// TeamSummary is a lightweight team view used in list operations.
type TeamSummary struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Regulation  string    `json:"regulation"`
	Notes       string    `json:"notes"`
	MemberCount int       `json:"member_count"`
	CreatedAt   time.Time `json:"created_at"`
}

// GetTeam loads a team and all its members (with species, ability, item, moves).
func (r *Repo) GetTeam(id int) (*Team, error) {
	var t Team
	var ca, ua string
	err := r.db.QueryRow(`SELECT id, name, regulation, notes, created_at, updated_at FROM teams WHERE id=?`, id).
		Scan(&t.ID, &t.Name, &t.Regulation, &t.Notes, &ca, &ua)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("team %d not found", id)
	}
	if err != nil {
		return nil, err
	}
	t.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", ca)
	t.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", ua)

	members, err := r.loadMembers(id)
	if err != nil {
		return nil, err
	}
	t.Members = members
	return &t, nil
}

// rawMember holds raw IDs from the team_members table before we enrich them.
type rawMember struct {
	id        int
	teamID    int
	slot      int
	speciesID int
	abilityID int
	itemID    sql.NullInt64
	nickname  string
	teraType  string
	nature    string
	role      string
	notes     string
	evs       StatSpread
}

func (r *Repo) loadMembers(teamID int) ([]Member, error) {
	// Collect all raw member rows first, then close the cursor before
	// issuing further queries (SQLite single-connection safe).
	rows, err := r.db.Query(`
		SELECT id, team_id, slot, species_id, ability_id, item_id,
		       nickname, COALESCE(tera_type,''), nature, role, notes,
		       ev_hp, ev_atk, ev_def, ev_spa, ev_spd, ev_spe
		FROM team_members WHERE team_id=? ORDER BY slot`, teamID)
	if err != nil {
		return nil, err
	}

	var raws []rawMember
	for rows.Next() {
		var rm rawMember
		if err := rows.Scan(
			&rm.id, &rm.teamID, &rm.slot, &rm.speciesID, &rm.abilityID, &rm.itemID,
			&rm.nickname, &rm.teraType, &rm.nature, &rm.role, &rm.notes,
			&rm.evs.HP, &rm.evs.Atk, &rm.evs.Def, &rm.evs.SpA, &rm.evs.SpD, &rm.evs.Spe,
		); err != nil {
			rows.Close()
			return nil, err
		}
		raws = append(raws, rm)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Enrich each raw member with related objects (one query at a time).
	members := make([]Member, len(raws))
	for i, rm := range raws {
		m := Member{
			ID:       rm.id,
			TeamID:   rm.teamID,
			Slot:     rm.slot,
			Nickname: rm.nickname,
			TeraType: rm.teraType,
			Nature:   rm.nature,
			Role:     rm.role,
			Notes:    rm.notes,
			EVs:      rm.evs,
		}

		if sp, err := r.pokemon.GetSpeciesByID(rm.speciesID); err == nil {
			m.Species = sp
		}

		var ab pokemon.Ability
		if err := r.db.QueryRow(`SELECT id, name, description FROM abilities WHERE id=?`, rm.abilityID).
			Scan(&ab.ID, &ab.Name, &ab.Description); err == nil {
			m.Ability = &ab
		}

		if rm.itemID.Valid {
			var it pokemon.Item
			if err := r.db.QueryRow(`SELECT id, name, description, is_banned FROM items WHERE id=?`, rm.itemID.Int64).
				Scan(&it.ID, &it.Name, &it.Description, &it.IsBanned); err == nil {
				m.Item = &it
			}
		}

		// Load moves (close before next iteration).
		mrows, err := r.db.Query(`
			SELECT mv.id, mv.name, mv.type, mv.category, mv.power, mv.accuracy,
			       mv.pp, mv.priority, mv.target, mv.description
			FROM member_moves mm
			JOIN moves mv ON mv.id = mm.move_id
			WHERE mm.member_id=? ORDER BY mm.slot`, rm.id)
		if err == nil {
			for mrows.Next() {
				var mv pokemon.Move
				var power, accuracy sql.NullInt64
				if err := mrows.Scan(&mv.ID, &mv.Name, &mv.Type, &mv.Category,
					&power, &accuracy, &mv.PP, &mv.Priority, &mv.Target, &mv.Description); err == nil {
					if power.Valid {
						v := int(power.Int64)
						mv.Power = &v
					}
					if accuracy.Valid {
						v := int(accuracy.Int64)
						mv.Accuracy = &v
					}
					m.Moves = append(m.Moves, &mv)
				}
			}
			mrows.Close()
		}

		members[i] = m
	}
	return members, nil
}

// AddMember adds a Pokemon to a team in the next available slot (or errors if full).
func (r *Repo) AddMember(teamID, speciesID, abilityID int) (int64, error) {
	var maxSlot int
	r.db.QueryRow(`SELECT COALESCE(MAX(slot),0) FROM team_members WHERE team_id=?`, teamID).Scan(&maxSlot)
	if maxSlot >= 6 {
		return 0, fmt.Errorf("team is full (6 Pokemon maximum)")
	}
	res, err := r.db.Exec(`
		INSERT INTO team_members(team_id, slot, species_id, ability_id)
		VALUES(?, ?, ?, ?)`, teamID, maxSlot+1, speciesID, abilityID)
	if err != nil {
		return 0, fmt.Errorf("AddMember: %w", err)
	}
	r.touchTeam(teamID)
	return res.LastInsertId()
}

// RemoveMember removes a team member and re-slots remaining members.
func (r *Repo) RemoveMember(memberID int) error {
	var teamID int
	if err := r.db.QueryRow(`SELECT team_id FROM team_members WHERE id=?`, memberID).Scan(&teamID); err != nil {
		return fmt.Errorf("member %d not found", memberID)
	}
	if _, err := r.db.Exec(`DELETE FROM team_members WHERE id=?`, memberID); err != nil {
		return err
	}
	r.reSlot(teamID)
	r.touchTeam(teamID)
	return nil
}

// SetAbility updates the ability for a team member.
func (r *Repo) SetAbility(memberID, abilityID int) error {
	return r.updateMember(memberID, "ability_id", abilityID)
}

// SetNature updates the nature for a team member.
func (r *Repo) SetNature(memberID int, nature string) error {
	return r.updateMember(memberID, "nature", nature)
}

// SetItem updates the held item for a team member (pass 0 to clear).
func (r *Repo) SetItem(memberID, itemID int) error {
	if itemID == 0 {
		_, err := r.db.Exec(`UPDATE team_members SET item_id=NULL WHERE id=?`, memberID)
		return err
	}
	return r.updateMember(memberID, "item_id", itemID)
}

// SetMoves replaces all moves for a team member. Accepts 1–4 move IDs.
func (r *Repo) SetMoves(memberID int, moveIDs []int) error {
	if len(moveIDs) > 4 {
		return fmt.Errorf("a Pokemon can only have up to 4 moves")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM member_moves WHERE member_id=?`, memberID); err != nil {
		return err
	}
	for i, mid := range moveIDs {
		if _, err := tx.Exec(`INSERT INTO member_moves(member_id, slot, move_id) VALUES(?,?,?)`,
			memberID, i+1, mid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetEVs updates EV spread for a team member.
func (r *Repo) SetEVs(memberID int, evs StatSpread) error {
	_, err := r.db.Exec(`UPDATE team_members SET ev_hp=?,ev_atk=?,ev_def=?,ev_spa=?,ev_spd=?,ev_spe=? WHERE id=?`,
		evs.HP, evs.Atk, evs.Def, evs.SpA, evs.SpD, evs.Spe, memberID)
	return err
}

// SetIVs updates IV spread for a team member.
func (r *Repo) SetIVs(memberID int, ivs StatSpread) error {
	_, err := r.db.Exec(`UPDATE team_members SET iv_hp=?,iv_atk=?,iv_def=?,iv_spa=?,iv_spd=?,iv_spe=? WHERE id=?`,
		ivs.HP, ivs.Atk, ivs.Def, ivs.SpA, ivs.SpD, ivs.Spe, memberID)
	return err
}

// SetRole updates the role label for a team member.
func (r *Repo) SetRole(memberID int, role string) error {
	return r.updateMember(memberID, "role", role)
}

// SetNotes updates the notes field for a team member.
func (r *Repo) SetMemberNotes(memberID int, notes string) error {
	return r.updateMember(memberID, "notes", notes)
}

// SetNickname updates the nickname for a team member.
func (r *Repo) SetNickname(memberID int, nickname string) error {
	return r.updateMember(memberID, "nickname", nickname)
}

// SetTeraType updates the Tera type for a team member.
func (r *Repo) SetTeraType(memberID int, teraType string) error {
	return r.updateMember(memberID, "tera_type", teraType)
}

// GetMemberByTeamAndSpecies returns the member ID for a given species name on a team.
func (r *Repo) GetMemberByTeamAndSpecies(teamID int, speciesName string) (int, error) {
	var memberID int
	err := r.db.QueryRow(`
		SELECT tm.id FROM team_members tm
		JOIN species s ON s.id = tm.species_id
		WHERE tm.team_id=? AND lower(s.name)=lower(?)`, teamID, speciesName).Scan(&memberID)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("%s is not on team %d", speciesName, teamID)
	}
	return memberID, err
}

// GetRegulation loads a regulation from the database.
func (r *Repo) GetRegulation(id string) (*Regulation, error) {
	var reg Regulation
	var desc string
	err := r.db.QueryRow(`SELECT id, name, max_restricted, description FROM regulations WHERE id=?`, id).
		Scan(&reg.ID, &reg.Name, &reg.MaxRestricted, &desc)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("regulation %q not found", id)
	}
	if err != nil {
		return nil, err
	}

	// Load banned/restricted species.
	rows, err := r.db.Query(`SELECT species_id, rule FROM regulation_species_rules WHERE regulation_id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var banned, restricted []int
	for rows.Next() {
		var sid int
		var rule string
		rows.Scan(&sid, &rule)
		if rule == "banned" {
			banned = append(banned, sid)
		} else {
			restricted = append(restricted, sid)
		}
	}

	// Load banned moves.
	mrows, err := r.db.Query(`SELECT move_id FROM regulation_move_bans WHERE regulation_id=?`, id)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	var bannedMoves []int
	for mrows.Next() {
		var mid int
		mrows.Scan(&mid)
		bannedMoves = append(bannedMoves, mid)
	}

	return NewRegulation(reg.ID, reg.Name, reg.MaxRestricted, banned, restricted, bannedMoves), nil
}

// ListRegulations returns all regulation IDs and names.
func (r *Repo) ListRegulations() ([]RegulationSummary, error) {
	rows, err := r.db.Query(`SELECT id, name, start_date, end_date, description, max_restricted FROM regulations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RegulationSummary
	for rows.Next() {
		var s RegulationSummary
		var start, end sql.NullString
		rows.Scan(&s.ID, &s.Name, &start, &end, &s.Description, &s.MaxRestricted)
		s.StartDate = start.String
		s.EndDate = end.String
		out = append(out, s)
	}
	return out, rows.Err()
}

// RegulationSummary is a lightweight view of a regulation.
type RegulationSummary struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
	Description   string `json:"description"`
	MaxRestricted int    `json:"max_restricted"`
}

func (r *Repo) updateMember(memberID int, col string, val any) error {
	_, err := r.db.Exec(`UPDATE team_members SET `+col+`=? WHERE id=?`, val, memberID)
	if err != nil {
		return fmt.Errorf("updateMember %s: %w", col, err)
	}
	var teamID int
	r.db.QueryRow(`SELECT team_id FROM team_members WHERE id=?`, memberID).Scan(&teamID)
	r.touchTeam(teamID)
	return nil
}

func (r *Repo) touchTeam(teamID int) {
	r.db.Exec(`UPDATE teams SET updated_at=datetime('now') WHERE id=?`, teamID)
}

func (r *Repo) reSlot(teamID int) {
	rows, _ := r.db.Query(`SELECT id FROM team_members WHERE team_id=? ORDER BY slot`, teamID)
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
		r.db.Exec(`UPDATE team_members SET slot=? WHERE id=?`, i+1, id)
	}
}
