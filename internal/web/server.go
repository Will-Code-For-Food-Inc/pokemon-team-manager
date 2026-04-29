// Package web provides a server-rendered HTTP interface for the Pokemon Team Manager.
package web

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/user/pokemon-team-manager/internal/handlers"
	"github.com/user/pokemon-team-manager/internal/knowledge"
	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

//go:embed templates/*.html
var templateFS embed.FS

// Services holds all domain repos the web handlers need.
type Services struct {
	DB        *sql.DB
	Pokemon   *pokemon.Repo
	Team      *team.Repo
	Knowledge *knowledge.Repo
}

// Handlers returns a handlers.Services for use with the shared handler layer.
func (s *Services) Handlers() *handlers.Services {
	return &handlers.Services{DB: s.DB, Pokemon: s.Pokemon, Team: s.Team, Knowledge: s.Knowledge}
}

var funcMap = template.FuncMap{
	"safeHTML": func(s string) template.HTML { return template.HTML(s) },
	"statbar": func(v int) int {
		if v > 120 {
			return 120
		}
		return v
	},
	"moveAcc": func(acc *int) string {
		if acc == nil || *acc == 0 || *acc >= 101 {
			return "∞"
		}
		return fmt.Sprintf("%d", *acc)
	},
	"add":      func(a, b int) int { return a + b },
	"sub66":    func(v int) int { return 66 - v },
	"iterate":  func(n int) []int { s := make([]int, n); for i := range s { s[i] = i }; return s },
	"moveName": func(mv *pokemon.Move) string { if mv == nil { return "" }; return mv.Name },
	"index": func(moves []*pokemon.Move, i int) *pokemon.Move {
		if i < len(moves) {
			return moves[i]
		}
		return nil
	},
	"not": func(v any) bool {
		if v == nil {
			return true
		}
		switch x := v.(type) {
		case []knowledge.SearchResult:
			return len(x) == 0
		case bool:
			return !x
		}
		return false
	},
}

func pageTemplate(page string) *template.Template {
	return template.Must(
		template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/base.html", "templates/"+page),
	)
}

// New creates and returns an http.Handler for the ptm web UI.
// Clears any stale agent_thinking state left by a previous crash.
func New(svc *Services) http.Handler {
	// Clear stale thinking flag on startup so a crashed server doesn't leave
	// the UI permanently stuck in "thinking" state.
	if _, err := svc.DB.Exec(`INSERT INTO settings(key,value) VALUES('agent_thinking','0')
		ON CONFLICT(key) DO UPDATE SET value='0'`); err != nil {
		slog.Error("clear agent_thinking flag", "err", err)
	}

	mux := http.NewServeMux()
	h := &handler{svc: svc, startTime: time.Now()}

	mux.HandleFunc("/", h.dashboard)
	mux.HandleFunc("/version", h.version)
	mux.HandleFunc("/teams", h.teamsList)
	mux.HandleFunc("/teams/new", h.teamsNew)
	mux.HandleFunc("/teams/", h.teamRouter)
	mux.HandleFunc("/pokemon", h.pokemonList)
	mux.HandleFunc("/pokemon/", h.pokemonDetail)
	mux.HandleFunc("/api/pokemon/owned", h.togglePokemonOwned)
	mux.HandleFunc("/moves", h.movesList)
	mux.HandleFunc("/items", h.itemsList)
	mux.HandleFunc("/api/items/owned", h.toggleItemOwned)
	mux.HandleFunc("/kb", h.kbSearch)
	mux.HandleFunc("/chat", h.chatPage)
	mux.HandleFunc("/api/chat", h.chatAPI)
	mux.HandleFunc("/api/chat/cancel", h.chatCancel)
	mux.HandleFunc("/api/chat/history", h.chatHistory)
	mux.HandleFunc("/settings", h.settingsPage)
	mux.HandleFunc("/api/chat/archive", h.chatArchive)
	mux.HandleFunc("/regulations", h.regulationsPage)
	mux.HandleFunc("/api/db/backup", h.dbBackup)

	return loggingMiddleware(corsMiddleware(mux))
}

func (h *handler) setThinking(on bool) {
	v := "0"
	if on {
		v = "1"
	}
	h.svc.DB.Exec(`INSERT INTO settings(key,value) VALUES('agent_thinking',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, v)
}

func (h *handler) isThinking() bool {
	var v string
	h.svc.DB.QueryRow(`SELECT value FROM settings WHERE key='agent_thinking'`).Scan(&v)
	return v == "1"
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type handler struct {
	svc       *Services
	startTime time.Time
}

func (h *handler) render(w http.ResponseWriter, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl := pageTemplate(page)
	if err := tmpl.ExecuteTemplate(w, page, data); err != nil {
		http.Error(w, "template error: "+err.Error(), 500)
	}
}

func (h *handler) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	teams, _ := h.svc.Team.ListTeams()
	regs, _ := h.svc.Team.ListActiveRegulations()
	recent := teams
	if len(recent) > 5 {
		recent = recent[:5]
	}
	h.render(w, "dashboard.html", map[string]any{
		"TeamCount":   len(teams),
		"RecentTeams": recent,
		"Regulations": regs,
	})
}

// --- Teams ---

func (h *handler) teamsList(w http.ResponseWriter, r *http.Request) {
	teams, _ := h.svc.Team.ListTeams()
	regs, _ := h.svc.Team.ListActiveRegulations()
	h.render(w, "teams.html", map[string]any{
		"Teams":       teams,
		"Regulations": regs,
		"Flash":       r.URL.Query().Get("flash"),
		"Error":       r.URL.Query().Get("error"),
	})
}

func (h *handler) teamsNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/teams", http.StatusFound)
		return
	}
	r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))
	reg := strings.TrimSpace(r.FormValue("regulation"))
	if name == "" || reg == "" {
		http.Redirect(w, r, "/teams?error=Name+and+regulation+required", http.StatusFound)
		return
	}
	id, err := h.svc.Team.CreateTeam(name, reg)
	if err != nil {
		http.Redirect(w, r, "/teams?error="+urlEnc(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%d?flash=Team+created", id), http.StatusFound)
}

func (h *handler) teamRouter(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	teamID, err := strconv.Atoi(parts[1])
	if err != nil {
		http.NotFound(w, r)
		return
	}

	switch {
	case len(parts) == 2:
		h.teamView(w, r, teamID)
	case len(parts) == 3 && parts[2] == "delete" && r.Method == http.MethodPost:
		h.teamDelete(w, r, teamID)
	case len(parts) == 3 && parts[2] == "validate":
		h.teamValidate(w, r, teamID)
	case len(parts) == 3 && parts[2] == "analyse":
		h.teamAnalyse(w, r, teamID)
	case len(parts) == 3 && parts[2] == "export":
		h.teamExport(w, r, teamID)
	case len(parts) == 3 && parts[2] == "history":
		h.teamHistory(w, r, teamID)
	case len(parts) == 4 && parts[2] == "history" && parts[3] == "prune" && r.Method == http.MethodPost:
		h.teamHistoryPrune(w, r, teamID)
	case len(parts) == 4 && parts[2] == "history" && r.Method == http.MethodGet:
		snapID, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		h.teamHistoryView(w, r, teamID, snapID)
	case len(parts) == 4 && parts[2] == "members" && parts[3] == "add" && r.Method == http.MethodPost:
		h.memberAdd(w, r, teamID)
	case len(parts) == 5 && parts[2] == "configs" && parts[4] == "edit":
		configID, _ := strconv.Atoi(parts[3])
		h.memberEdit(w, r, teamID, configID)
	case len(parts) == 5 && parts[2] == "configs" && parts[4] == "delete" && r.Method == http.MethodPost:
		configID, _ := strconv.Atoi(parts[3])
		h.memberDelete(w, r, teamID, configID)
	default:
		http.NotFound(w, r)
	}
}

func (h *handler) teamView(w http.ResponseWriter, r *http.Request, teamID int) {
	t, err := h.svc.Team.GetTeam(teamID)
	if err != nil {
		http.Error(w, "Team not found", 404)
		return
	}
	allSpecies, _ := h.svc.Pokemon.SearchSpecies("", 2000, pokemon.SpeciesFilter{})
	h.render(w, "team.html", map[string]any{
		"Team":       t,
		"Flash":      r.URL.Query().Get("flash"),
		"Error":      r.URL.Query().Get("error"),
		"AllSpecies": allSpecies,
	})
}

func (h *handler) teamDelete(w http.ResponseWriter, r *http.Request, teamID int) {
	h.svc.Team.DeleteTeam(teamID)
	http.Redirect(w, r, "/teams?flash=Team+deleted", http.StatusFound)
}

func (h *handler) teamValidate(w http.ResponseWriter, _ *http.Request, teamID int) {
	t, err := h.svc.Team.GetTeam(teamID)
	if err != nil {
		http.Error(w, "Team not found", 404)
		return
	}
	reg, _ := h.svc.Team.GetRegulation(t.Regulation)
	violations := team.Validate(t, reg, h.svc.Pokemon)
	h.render(w, "validate.html", map[string]any{
		"Team":       t,
		"Violations": violations,
	})
}

func (h *handler) teamAnalyse(w http.ResponseWriter, _ *http.Request, teamID int) {
	t, err := h.svc.Team.GetTeam(teamID)
	if err != nil {
		http.Error(w, "Team not found", 404)
		return
	}
	h.render(w, "analyse.html", map[string]any{
		"Team":     t,
		"Analysis": team.Analyse(t),
	})
}

// teamHistory lists snapshots for a team, newest first.
func (h *handler) teamHistory(w http.ResponseWriter, r *http.Request, teamID int) {
	t, err := h.svc.Team.GetTeam(teamID)
	if err != nil {
		http.Error(w, "Team not found", 404)
		return
	}
	snaps, _ := h.svc.Team.ListTeamSnapshots(teamID, 200, 0)
	h.render(w, "team_history.html", map[string]any{
		"Team":      t,
		"Snapshots": snaps,
		"Flash":     r.URL.Query().Get("flash"),
	})
}

// teamHistoryView renders a single snapshot as if it were the current team.
func (h *handler) teamHistoryView(w http.ResponseWriter, _ *http.Request, teamID int, snapID int64) {
	snap, err := h.svc.Team.GetTeamSnapshot(snapID)
	if err != nil {
		http.Error(w, "Snapshot not found", 404)
		return
	}
	if snap.TeamID != teamID {
		http.Error(w, "Snapshot does not belong to this team", 400)
		return
	}
	var t team.Team
	if err := json.Unmarshal([]byte(snap.Payload), &t); err != nil {
		http.Error(w, "Snapshot payload corrupted", 500)
		return
	}
	h.render(w, "team_history_snapshot.html", map[string]any{
		"Team":     &t,
		"Snapshot": snap,
	})
}

// teamHistoryPrune is the user-only path to drop mutation snapshots beyond
// keepLastN. Checkpoints are preserved. No agent route reaches this.
func (h *handler) teamHistoryPrune(w http.ResponseWriter, r *http.Request, teamID int) {
	keep := 0
	if v, err := strconv.Atoi(r.FormValue("keep")); err == nil && v >= 0 {
		keep = v
	}
	dropped, err := h.svc.Team.PruneTeamSnapshots(teamID, keep)
	if err != nil {
		http.Error(w, "Prune failed: "+err.Error(), 500)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%d/history?flash=Pruned+%d+snapshot(s)", teamID, dropped), http.StatusSeeOther)
}

func (h *handler) teamExport(w http.ResponseWriter, _ *http.Request, teamID int) {
	t, err := h.svc.Team.GetTeam(teamID)
	if err != nil {
		http.Error(w, "Team not found", 404)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="team-%d.md"`, teamID))
	fmt.Fprint(w, team.ExportMarkdown(t))
}

// --- Members ---

func (h *handler) memberAdd(w http.ResponseWriter, r *http.Request, teamID int) {
	r.ParseForm()
	pokeName := strings.TrimSpace(r.FormValue("pokemon_name"))
	abilityName := strings.TrimSpace(r.FormValue("ability"))

	sp, err := h.svc.Pokemon.GetSpeciesByName(pokeName)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/teams/%d?error=Pokemon+%q+not+found", teamID, pokeName), http.StatusFound)
		return
	}
	if !sp.IsFinalEvo {
		http.Redirect(w, r, fmt.Sprintf("/teams/%d?error=%s+is+not+a+final+evolution", teamID, sp.Name), http.StatusFound)
		return
	}

	abilities, err := h.svc.Pokemon.GetAbilitiesForSpecies(sp.Slug)
	if err != nil || len(abilities) == 0 {
		http.Redirect(w, r, fmt.Sprintf("/teams/%d?error=No+abilities+found+for+%s", teamID, sp.Name), http.StatusFound)
		return
	}
	abilitySlug := abilities[0].Slug
	if abilityName != "" {
		ab, err := h.svc.Pokemon.GetAbilityByName(abilityName)
		if err != nil {
			http.Redirect(w, r, fmt.Sprintf("/teams/%d?error=Ability+%q+not+found", teamID, abilityName), http.StatusFound)
			return
		}
		if ok, _ := h.svc.Pokemon.HasAbility(sp.Slug, ab.Slug); !ok {
			http.Redirect(w, r, fmt.Sprintf("/teams/%d?error=%s+cannot+have+%s", teamID, sp.Name, ab.Name), http.StatusFound)
			return
		}
		abilitySlug = ab.Slug
	}

	if _, err := h.svc.Team.AddMember(teamID, sp.Slug, abilitySlug); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/teams/%d?error=%s", teamID, urlEnc(err.Error())), http.StatusFound)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%d?flash=%s+added", teamID, sp.Name), http.StatusFound)
}

func (h *handler) memberEdit(w http.ResponseWriter, r *http.Request, teamID, configID int) {
	t, err := h.svc.Team.GetTeam(teamID)
	if err != nil {
		http.Error(w, "Team not found", 404)
		return
	}
	var member *team.Member
	for i := range t.Members {
		if t.Members[i].ConfigID == configID {
			member = &t.Members[i]
			break
		}
	}
	if member == nil || member.Config == nil {
		http.Error(w, "Member not found", 404)
		return
	}

	if r.Method == http.MethodPost {
		h.memberSave(w, r, t, member)
		return
	}

	abilities, _ := h.svc.Pokemon.GetAbilitiesForSpecies(member.Config.Species.Slug)
	learnset, _ := h.svc.Pokemon.GetChampionsLearnset(member.Config.Species.Slug)
	items, _ := h.svc.Pokemon.SearchItems("", 200, pokemon.ItemFilter{})
	allTypes := []string{"normal", "fire", "water", "electric", "grass", "ice", "fighting", "poison",
		"ground", "flying", "psychic", "bug", "rock", "ghost", "dragon", "dark", "steel", "fairy"}

	h.render(w, "member_edit.html", map[string]any{
		"Team":      t,
		"Member":    member,
		"Abilities": abilities,
		"Learnset":  learnset,
		"Items":     items,
		"Natures":   pokemon.AllNatures,
		"Types":     allTypes,
		"Error":     r.URL.Query().Get("error"),
	})
}

func (h *handler) memberSave(w http.ResponseWriter, r *http.Request, t *team.Team, member *team.Member) {
	r.ParseForm()
	cid := member.ConfigID
	sp := member.Config.Species

	// Ability — form sends ability_id (integer), look up to get slug
	if abilityIDStr := r.FormValue("ability_id"); abilityIDStr != "" {
		if aid, err := strconv.Atoi(abilityIDStr); err == nil {
			if ab, err := h.svc.Pokemon.GetAbilityByID(aid); err == nil {
				h.svc.Team.SetAbility(cid, ab.Slug)
			}
		}
	}

	// Nature
	if nature := r.FormValue("nature"); nature != "" {
		h.svc.Team.SetNature(cid, nature)
	}

	// Item — form sends item_id (integer)
	if itemIDStr := r.FormValue("item_id"); itemIDStr != "" {
		if iid, err := strconv.Atoi(itemIDStr); err == nil {
			if iid == 0 {
				h.svc.Team.SetItem(cid, "")
			} else if it, err := h.svc.Pokemon.GetItemByID(iid); err == nil {
				// Item clause check
				itemOK := true
				for _, m := range t.Members {
					if m.ConfigID != cid && m.Config != nil && m.Config.Item != nil && m.Config.Item.Slug == it.Slug {
						http.Redirect(w, r,
							fmt.Sprintf("/teams/%d/configs/%d/edit?error=Item+clause:+another+member+holds+that+item", t.ID, cid),
							http.StatusFound)
						itemOK = false
						break
					}
				}
				if itemOK {
					h.svc.Team.SetItem(cid, it.Slug)
				} else {
					return
				}
			}
		}
	}

	h.svc.Team.SetRole(cid, r.FormValue("role"))
	h.svc.Team.SetConfigNotes(cid, r.FormValue("notes"))
	h.svc.Team.SetNickname(cid, r.FormValue("nickname"))

	// EVs — Champions uses stat points (0-66 total, max 32 each)
	evs := team.StatSpread{
		HP:  formInt(r, "ev_hp"),
		Atk: formInt(r, "ev_atk"),
		Def: formInt(r, "ev_def"),
		SpA: formInt(r, "ev_spa"),
		SpD: formInt(r, "ev_spd"),
		Spe: formInt(r, "ev_spe"),
	}
	if evs.Total() > 66 {
		http.Redirect(w, r,
			fmt.Sprintf("/teams/%d/configs/%d/edit?error=SP+total+%d+exceeds+66", t.ID, cid, evs.Total()),
			http.StatusFound)
		return
	}
	h.svc.Team.SetEVs(cid, evs)

	// Moves — validate Champions learnset
	var moveSlugs []string
	for i := 1; i <= 4; i++ {
		name := strings.TrimSpace(r.FormValue(fmt.Sprintf("move%d", i)))
		if name == "" {
			continue
		}
		mv, err := h.svc.Pokemon.GetMoveByName(name)
		if err != nil {
			http.Redirect(w, r,
				fmt.Sprintf("/teams/%d/configs/%d/edit?error=Move+%q+not+found", t.ID, cid, name),
				http.StatusFound)
			return
		}
		ok, _ := h.svc.Pokemon.CanLearnMove(sp.Slug, mv.Slug)
		if !ok {
			http.Redirect(w, r,
				fmt.Sprintf("/teams/%d/configs/%d/edit?error=%s+cannot+learn+%s+in+Champions+format", t.ID, cid, sp.Name, mv.Name),
				http.StatusFound)
			return
		}
		moveSlugs = append(moveSlugs, mv.Slug)
	}
	if len(moveSlugs) > 0 {
		h.svc.Team.SetMoves(cid, moveSlugs)
	}

	http.Redirect(w, r, fmt.Sprintf("/teams/%d?flash=Saved+%s", t.ID, sp.Name), http.StatusFound)
}

func (h *handler) memberDelete(w http.ResponseWriter, r *http.Request, teamID, configID int) {
	h.svc.Team.RemoveMember(configID)
	http.Redirect(w, r, fmt.Sprintf("/teams/%d?flash=Pokemon+removed", teamID), http.StatusFound)
}

// --- Pokemon, Moves, Items, KB ---

func (h *handler) pokemonList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := pokemon.SpeciesFilter{
		Type:         q.Get("type"),
		FinalEvoOnly: q.Get("final") == "1",
	}
	if g, err := strconv.Atoi(q.Get("gen")); err == nil {
		f.Generation = g
	}
	switch q.Get("owned") {
	case "1":
		t := true; f.Owned = &t
	case "0":
		fv := false; f.Owned = &fv
	}
	results, _ := h.svc.Pokemon.SearchSpecies(q.Get("q"), 300, f)
	types := []string{"normal", "fire", "water", "electric", "grass", "ice", "fighting", "poison",
		"ground", "flying", "psychic", "bug", "rock", "ghost", "dragon", "dark", "steel", "fairy"}
	h.render(w, "pokemon_list.html", map[string]any{
		"Query":       q.Get("q"),
		"Results":     results,
		"Types":       types,
		"Gens":        []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"},
		"FilterType":  q.Get("type"),
		"FilterGen":   q.Get("gen"),
		"FilterOwned": q.Get("owned"),
		"FilterFinal": q.Get("final"),
	})
}

func (h *handler) pokemonDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/pokemon/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.SplitN(path, "/", 3)
	nameOrSlug := parts[0]

	sp, err := h.svc.Pokemon.GetSpeciesByName(nameOrSlug)
	if err != nil {
		// try as dex ID
		if id, convErr := strconv.Atoi(nameOrSlug); convErr == nil {
			sp, err = h.svc.Pokemon.GetSpeciesByDexID(id)
		}
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if len(parts) == 3 && r.Method == http.MethodPost {
		switch parts[1] + "/" + parts[2] {
		case "learnset/add":
			moveName := r.FormValue("move_name")
			mvs, _ := h.svc.Pokemon.SearchMoves(moveName, 1, pokemon.MoveFilter{})
			if len(mvs) == 0 {
				http.Redirect(w, r, "/pokemon/"+nameOrSlug+"?error=Move+not+found", http.StatusFound)
				return
			}
			h.svc.Pokemon.AddLearnsetMove(sp.Slug, mvs[0].Slug)
			http.Redirect(w, r, "/pokemon/"+nameOrSlug+"?flash=Move+added", http.StatusFound)
			return
		case "learnset/remove":
			moveSlug := r.FormValue("move_slug")
			if moveSlug == "" {
				// fallback: look up by ID
				if mid, err := strconv.Atoi(r.FormValue("move_id")); err == nil {
					if mv, err := h.svc.Pokemon.GetMoveBySlug(strconv.Itoa(mid)); err == nil {
						moveSlug = mv.Slug
					}
				}
			}
			h.svc.Pokemon.RemoveLearnsetMove(sp.Slug, moveSlug)
			http.Redirect(w, r, "/pokemon/"+nameOrSlug+"?flash=Move+removed", http.StatusFound)
			return
		case "abilities/add":
			_, err := h.svc.Pokemon.AddSpeciesAbility(sp.Slug, r.FormValue("ability_name"), r.FormValue("description"))
			if err != nil {
				http.Redirect(w, r, "/pokemon/"+nameOrSlug+"?error="+urlEnc(err.Error()), http.StatusFound)
				return
			}
			http.Redirect(w, r, "/pokemon/"+nameOrSlug+"?flash=Ability+added", http.StatusFound)
			return
		case "abilities/remove":
			abilitySlug := r.FormValue("ability_slug")
			if abilitySlug == "" {
				if aid, err := strconv.Atoi(r.FormValue("ability_id")); err == nil {
					if ab, err := h.svc.Pokemon.GetAbilityByID(aid); err == nil {
						abilitySlug = ab.Slug
					}
				}
			}
			h.svc.Pokemon.RemoveSpeciesAbility(sp.Slug, abilitySlug)
			http.Redirect(w, r, "/pokemon/"+nameOrSlug+"?flash=Ability+removed", http.StatusFound)
			return
		}
		http.NotFound(w, r)
		return
	}

	abilities, _ := h.svc.Pokemon.GetAbilitiesForSpecies(sp.Slug)
	learnset, _ := h.svc.Pokemon.GetChampionsLearnset(sp.Slug)

	type teamMembership struct {
		TeamID     int
		TeamName   string
		Regulation string
		Member     team.Member
	}
	var memberships []teamMembership
	if teams, err := h.svc.Team.ListTeams(); err == nil {
		for _, ts := range teams {
			t, err := h.svc.Team.GetTeam(ts.ID)
			if err != nil {
				continue
			}
			for _, m := range t.Members {
				if m.Config != nil && m.Config.Species != nil && m.Config.Species.Slug == sp.Slug {
					memberships = append(memberships, teamMembership{
						TeamID: t.ID, TeamName: t.Name, Regulation: t.Regulation, Member: m,
					})
				}
			}
		}
	}

	allMoves, _ := h.svc.Pokemon.SearchMoves("", 1000, pokemon.MoveFilter{})
	h.render(w, "pokemon_dex.html", map[string]any{
		"Species":     sp,
		"Abilities":   abilities,
		"Learnset":    learnset,
		"Memberships": memberships,
		"AllMoves":    allMoves,
		"Flash":       r.URL.Query().Get("flash"),
		"Error":       r.URL.Query().Get("error"),
	})
}

func (h *handler) movesList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	moves, _ := h.svc.Pokemon.SearchMoves(q, 50, pokemon.MoveFilter{
		Type:     r.URL.Query().Get("type"),
		Category: r.URL.Query().Get("category"),
	})
	h.render(w, "moves_list.html", map[string]any{"Query": q, "Results": moves})
}

func (h *handler) itemsList(w http.ResponseWriter, r *http.Request) {
	items, _ := h.svc.Pokemon.SearchItems(r.URL.Query().Get("q"), 50, pokemon.ItemFilter{})
	h.render(w, "items_list.html", map[string]any{"Query": r.URL.Query().Get("q"), "Results": items})
}

func (h *handler) kbSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	var results []knowledge.SearchResult
	if q != "" {
		results, _ = handlers.SearchKnowledge(h.svc.Handlers(), q, 10)
	}
	h.render(w, "kb.html", map[string]any{"Query": q, "Results": results})
}

// --- Chat ---

func (h *handler) chatPage(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig(h.svc.DB)
	chatCount, _ := (&chatRepo{db: h.svc.DB}).chatRepo().LiveStats()
	h.render(w, "chat.html", map[string]any{
		"Cfg":          cfg,
		"ChatCount":    chatCount,
		"ChatArchived": archivedFromQuery(r),
		"SettingsSaved": r.URL.Query().Get("saved") == "1",
	})
}

// archivedFromQuery returns the "archived" query param as int, 0 if missing.
func archivedFromQuery(r *http.Request) int {
	v, _ := strconv.Atoi(r.URL.Query().Get("archived"))
	return v
}

func (h *handler) chatHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	repo := &chatRepo{db: h.svc.DB}
	w.Header().Set("Content-Type", "application/json")
	msgs, err := repo.messages()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	if msgs == nil {
		msgs = []displayMessage{}
	}
	json.NewEncoder(w).Encode(map[string]any{
		"messages": lastN(msgs, 10),
		"thinking": h.isThinking(),
	})
}

func (h *handler) chatCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.setThinking(false)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// chatAPI streams agent responses using Server-Sent Events.
// Each message is pushed as "data: <json>\n\n" as it is produced.
// The final event has type "done" and carries the view payload and context.
func (h *handler) chatAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"error": "missing message"})
		return
	}

	// SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering if present
	flusher, canFlush := w.(http.Flusher)

	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if canFlush {
			flusher.Flush()
		}
	}

	// Echo the user message immediately so the UI can display it without waiting.
	send(map[string]any{"role": "user", "content": req.Message})

	repo := &chatRepo{db: h.svc.DB}
	cfg := loadConfig(h.svc.DB)
	ollama := newOllamaClient(cfg)

	queryVec, _ := ollama.Embed(req.Message)
	hist, _ := repo.semanticHistory(queryVec, cfg.Lookback, cfg.Lookback)
	kbResults, _ := handlers.SearchKnowledge(h.svc.Handlers(), req.Message, 3)

	// Mark thinking so a page reload shows the indicator.
	h.setThinking(true)
	// Run the agent loop, streaming each message as it arrives.
	// view_hint messages are resolved into full view payloads and streamed immediately.
	// streamed captures every message routed through onMsg — including sub-agent
	// tool indicators (already prefixed with "[Title] " by runSubAgent). runAgent's
	// returned `produced` covers only the orchestrator state's own messages, so
	// without this the inner layers vanish on page reload.
	var streamed []chatMessage
	_, newCtx, _ := runAgent(ollama, h.svc, hist, req.Message, func(m chatMessage) {
		if m.Role == "view" {
			// Resolve the view hint into a full payload and stream it now.
			var hint struct {
				Type string          `json:"type"`
				View *agentView      `json:"view"`
			}
			if err := json.Unmarshal([]byte(m.Content), &hint); err == nil && hint.View != nil {
				payload := h.resolveView(hint.View)
				if payload != nil {
					send(map[string]any{"type": "view", "view": payload})
				}
			}
			return
		}
		streamed = append(streamed, m)
		send(map[string]any{"role": m.Role, "content": m.Content})
	})
	h.setThinking(false)

	// Persist to DB and embed asynchronously.
	if userID, err := repo.appendMessage("user", req.Message); err == nil {
		go handlers.EmbedAndSave(ollama, repo.chatRepo(), userID, req.Message)
	}
	for _, m := range streamed {
		if id, err := repo.appendMessage(m.Role, m.Content); err == nil && (m.Role == "assistant" || m.Role == "tool") {
			go handlers.EmbedAndSave(ollama, repo.chatRepo(), id, m.Content)
		}
	}
	fullCtx, _ := repo.loadContext()
	_ = repo.saveContext(append(fullCtx, newCtx...))

	// No view in done — mid-turn view events already rendered each card.
	send(map[string]any{
		"type":          "done",
		"agent_context": map[string]any{"history": hist, "knowledge": kbResults},
	})
}

// --- Settings ---

func (h *handler) settingsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.ParseForm()
		parseInt := func(key string, def int) int {
			v, err := strconv.Atoi(r.FormValue(key))
			if err != nil {
				return def
			}
			return v
		}
		parseFloat := func(key string, def float64) float64 {
			v, err := strconv.ParseFloat(r.FormValue(key), 64)
			if err != nil {
				return def
			}
			return v
		}
		cfg := agentConfig{
			OllamaURL:   r.FormValue("ollama_url"),
			Model:       r.FormValue("ollama_model"),
			NumCtx:      parseInt("ollama_num_ctx", 20000),
			Temperature: parseFloat("ollama_temp", 0.3),
			TopP:        parseFloat("ollama_top_p", 0.7),
			TopK:        parseInt("ollama_top_k", 20),
			Repeat:      parseFloat("ollama_repeat", 1.1),
			KeepAlive:   r.FormValue("ollama_keep_alive"),
			Lookback:    parseInt("agent_lookback", 10),
			Prompt:      r.FormValue("agent_prompt"),
		}
		_ = saveConfig(h.svc.DB, cfg)
		// Settings live on the page they affect — redirect back to the page
		// the form was submitted from. Falls back to /settings if the Referer
		// is missing (e.g. someone POSTs from curl).
		http.Redirect(w, r, redirectAfterSettings(r, "saved=1"), http.StatusSeeOther)
		return
	}
	cfg := loadConfig(h.svc.DB)
	var dbPath string
	// MaxOpenConns is 1, so we must close the rows iterator before issuing the
	// next query — a deferred Close would deadlock the LiveStats call below.
	if rows, err := h.svc.DB.Query(`PRAGMA database_list`); err == nil && rows != nil {
		var seq int
		var name string
		if rows.Next() {
			_ = rows.Scan(&seq, &name, &dbPath)
		}
		rows.Close()
	}
	chatCount, _ := (&chatRepo{db: h.svc.DB}).chatRepo().LiveStats()
	archived := 0
	if v := r.URL.Query().Get("archived"); v != "" {
		archived, _ = strconv.Atoi(v)
	}
	h.render(w, "settings.html", map[string]any{
		"Cfg":          cfg,
		"DBPath":       dbPath,
		"Saved":        r.URL.Query().Get("saved") == "1",
		"ChatCount":    chatCount,
		"ChatArchived": archived,
	})
}

// chatArchive moves the live chat backlog into the archive tables under a
// single archive_id (UnixMicro of the request) and clears the live chat
// state. Redirects back to /settings with the count for confirmation.
func (h *handler) chatArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	repo := (&chatRepo{db: h.svc.DB}).chatRepo()
	archiveID := time.Now().UnixMicro()
	moved, err := repo.ArchiveAndClear(archiveID)
	if err != nil {
		http.Error(w, "archive failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, redirectAfterSettings(r, fmt.Sprintf("archived=%d", moved)), http.StatusSeeOther)
}

// redirectAfterSettings returns the Referer with `qs` appended (or replacing
// existing query). Falls back to /settings if no Referer is set. Used by
// settings-style POST handlers so the user lands back on the page that owned
// the drawer they submitted, not on the global /settings page.
func redirectAfterSettings(r *http.Request, qs string) string {
	ref := r.Referer()
	if ref == "" {
		return "/settings?" + qs
	}
	// Strip any existing query so flash params don't pile up.
	if i := strings.Index(ref, "?"); i >= 0 {
		ref = ref[:i]
	}
	return ref + "?" + qs
}

func (h *handler) togglePokemonOwned(w http.ResponseWriter, r *http.Request) {
	slug := r.FormValue("slug")
	owned := r.FormValue("owned") == "1"
	w.Header().Set("Content-Type", "application/json")
	if slug == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "missing slug parameter"})
		return
	}
	if err := h.svc.Pokemon.SetSpeciesOwned(slug, owned); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (h *handler) toggleItemOwned(w http.ResponseWriter, r *http.Request) {
	slug := r.FormValue("slug")
	owned := r.FormValue("owned") == "1"
	w.Header().Set("Content-Type", "application/json")
	if slug == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "missing slug parameter"})
		return
	}
	if err := h.svc.Pokemon.SetItemOwned(slug, owned); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (h *handler) dbBackup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="ptm-backup.db"`)
	if _, err := h.svc.DB.Exec(`VACUUM INTO '/tmp/ptm-backup.db'`); err != nil {
		http.Error(w, "backup failed: "+err.Error(), 500)
		return
	}
	http.ServeFile(w, r, "/tmp/ptm-backup.db")
}

// resolveView fetches the data for a view hint and returns the full payload map,
// or nil if the data cannot be loaded.
func (h *handler) resolveView(v *agentView) map[string]any {
	switch v.Type {
	case "team":
		if t, err := h.svc.Team.GetTeam(v.TeamID); err == nil {
			return map[string]any{"type": "team", "team": t}
		}
	case "pokemon":
		sp, err := h.svc.Pokemon.GetSpeciesByName(v.PokemonName)
		if err != nil {
			if results, err2 := h.svc.Pokemon.SearchSpecies(v.PokemonName, 1, pokemon.SpeciesFilter{}); err2 == nil && len(results) > 0 {
				sp = &results[0]
				err = nil
			}
		}
		if err == nil {
			abilities, _ := h.svc.Pokemon.GetAbilitiesForSpecies(sp.Slug)
			learnset, _ := h.svc.Pokemon.GetChampionsLearnset(sp.Slug)
			return map[string]any{"type": "pokemon", "species": sp, "abilities": abilities, "eval": team.EvaluateSpecies(sp, learnset)}
		}
	case "evaluate":
		sp, err := h.svc.Pokemon.GetSpeciesByName(v.PokemonName)
		if err != nil {
			if results, err2 := h.svc.Pokemon.SearchSpecies(v.PokemonName, 1, pokemon.SpeciesFilter{}); err2 == nil && len(results) > 0 {
				sp = &results[0]
				err = nil
			}
		}
		if err == nil {
			if v.TeamID > 0 {
				if t, err2 := h.svc.Team.GetTeam(v.TeamID); err2 == nil {
					for i := range t.Members {
						m := &t.Members[i]
						if m.Config != nil && m.Config.Species != nil && strings.EqualFold(m.Config.Species.Name, sp.Name) {
							return map[string]any{"type": "evaluate", "eval": team.EvaluateMember(m), "member": true}
						}
					}
				}
			}
			learnset, _ := h.svc.Pokemon.GetChampionsLearnset(sp.Slug)
			return map[string]any{"type": "evaluate", "eval": team.EvaluateSpecies(sp, learnset), "member": false}
		}
	case "item":
		if it, err := h.svc.Pokemon.GetItemByName(v.ItemName); err == nil {
			return map[string]any{"type": "item", "item": it}
		}
	}
	return nil
}

func (h *handler) regulationsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.ParseForm()
		// Build set of IDs that were checked (active)
		checked := map[string]bool{}
		for _, id := range r.Form["active"] {
			checked[id] = true
		}
		// Toggle all regulations
		all, _ := h.svc.Team.ListRegulations()
		for _, reg := range all {
			h.svc.Team.SetRegulationActive(reg.ID, checked[reg.ID])
		}
		http.Redirect(w, r, "/regulations?saved=1", http.StatusSeeOther)
		return
	}
	all, _ := h.svc.Team.ListRegulations()
	h.render(w, "regulations.html", map[string]any{
		"Regulations": all,
		"Saved":       r.URL.Query().Get("saved") == "1",
	})
}

// --- helpers ---

func formInt(r *http.Request, key string) int {
	v, _ := strconv.Atoi(r.FormValue(key))
	return v
}

func urlEnc(s string) string {
	return strings.ReplaceAll(s, " ", "+")
}
