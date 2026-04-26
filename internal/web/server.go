// Package web provides a server-rendered HTTP interface for the Pokemon Team Manager.
package web

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

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
		if v == nil { return true }
		switch x := v.(type) {
		case []knowledge.SearchResult: return len(x) == 0
		case bool: return !x
		}
		return false
	},
}

// pageTemplate parses base.html + the named page file as an isolated template
// set so that {{define "content"}} blocks don't collide across pages.
func pageTemplate(page string) *template.Template {
	return template.Must(
		template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/base.html", "templates/"+page),
	)
}

// New creates and returns an http.Handler for the ptm web UI.
func New(svc *Services) http.Handler {
	mux := http.NewServeMux()
	h := &handler{svc: svc}

	mux.HandleFunc("/", h.dashboard)
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
	mux.HandleFunc("/api/chat/history", h.chatHistory)
	mux.HandleFunc("/settings", h.settingsPage)
	mux.HandleFunc("/api/db/backup", h.dbBackup)

	return corsMiddleware(mux)
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
	svc *Services
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
	regs, _ := h.svc.Team.ListRegulations()
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
	regs, _ := h.svc.Team.ListRegulations()
	flash := r.URL.Query().Get("flash")
	errMsg := r.URL.Query().Get("error")
	h.render(w, "teams.html", map[string]any{
		"Teams":       teams,
		"Regulations": regs,
		"Flash":       flash,
		"Error":       errMsg,
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
		http.Redirect(w, r, "/teams?error="+url(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%d?flash=Team+created", id), http.StatusFound)
}

// teamRouter dispatches /teams/{id}/... sub-routes.
func (h *handler) teamRouter(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// parts: ["teams", id, ...]
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
	case len(parts) == 4 && parts[2] == "members" && parts[3] == "add" && r.Method == http.MethodPost:
		h.memberAdd(w, r, teamID)
	case len(parts) == 5 && parts[2] == "members" && parts[4] == "edit":
		memberID, _ := strconv.Atoi(parts[3])
		h.memberEdit(w, r, teamID, memberID)
	case len(parts) == 5 && parts[2] == "members" && parts[4] == "delete" && r.Method == http.MethodPost:
		memberID, _ := strconv.Atoi(parts[3])
		h.memberDelete(w, r, teamID, memberID)
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

	abilities, err := h.svc.Pokemon.GetAbilitiesForSpecies(sp.ID)
	if err != nil || len(abilities) == 0 {
		http.Redirect(w, r, fmt.Sprintf("/teams/%d?error=No+abilities+found+for+%s", teamID, sp.Name), http.StatusFound)
		return
	}
	abilityID := abilities[0].ID
	if abilityName != "" {
		ab, err := h.svc.Pokemon.GetAbilityByName(abilityName)
		if err != nil {
			http.Redirect(w, r, fmt.Sprintf("/teams/%d?error=Ability+%q+not+found", teamID, abilityName), http.StatusFound)
			return
		}
		ok, _ := h.svc.Pokemon.HasAbility(sp.ID, ab.ID)
		if !ok {
			http.Redirect(w, r, fmt.Sprintf("/teams/%d?error=%s+cannot+have+%s", teamID, sp.Name, ab.Name), http.StatusFound)
			return
		}
		abilityID = ab.ID
	}

	if _, err := h.svc.Team.AddMember(teamID, sp.ID, abilityID); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/teams/%d?error=%s", teamID, url(err.Error())), http.StatusFound)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%d?flash=%s+added", teamID, sp.Name), http.StatusFound)
}

func (h *handler) memberEdit(w http.ResponseWriter, r *http.Request, teamID, memberID int) {
	t, err := h.svc.Team.GetTeam(teamID)
	if err != nil {
		http.Error(w, "Team not found", 404)
		return
	}
	var member *team.Member
	for i := range t.Members {
		if t.Members[i].ID == memberID {
			member = &t.Members[i]
			break
		}
	}
	if member == nil {
		http.Error(w, "Member not found", 404)
		return
	}

	if r.Method == http.MethodPost {
		h.memberSave(w, r, t, member)
		return
	}

	abilities, _ := h.svc.Pokemon.GetAbilitiesForSpecies(member.Species.ID)
	learnset, _ := h.svc.Pokemon.GetLearnset(member.Species.ID)
	items, _ := h.svc.Pokemon.SearchItems("", 200, pokemon.ItemFilter{})
	allTypes := []string{"normal","fire","water","electric","grass","ice","fighting","poison","ground","flying","psychic","bug","rock","ghost","dragon","dark","steel","fairy"}

	h.render(w, "member_edit.html", map[string]any{
		"Team":       t,
		"Member":     member,
		"Abilities":  abilities,
		"Learnset":   learnset,
		"Items":      items,
		"Natures":    pokemon.AllNatures,
		"Types":      allTypes,
		"Error":      r.URL.Query().Get("error"),
		"Violations": nil,
	})
}

func (h *handler) memberSave(w http.ResponseWriter, r *http.Request, t *team.Team, member *team.Member) {
	r.ParseForm()
	mid := member.ID

	// Ability
	if abilityIDStr := r.FormValue("ability_id"); abilityIDStr != "" {
		if aid, err := strconv.Atoi(abilityIDStr); err == nil {
			h.svc.Team.SetAbility(mid, aid)
		}
	}

	// Nature
	if nature := r.FormValue("nature"); nature != "" {
		h.svc.Team.SetNature(mid, nature)
	}

	// Item
	if itemIDStr := r.FormValue("item_id"); itemIDStr != "" {
		if iid, err := strconv.Atoi(itemIDStr); err == nil {
			// Item clause: check no other member holds it.
			if iid != 0 {
				for _, m := range t.Members {
					if m.ID != mid && m.Item != nil && m.Item.ID == iid {
						http.Redirect(w, r,
							fmt.Sprintf("/teams/%d/members/%d/edit?error=Item+clause+violation:+another+member+holds+that+item", t.ID, mid),
							http.StatusFound)
						return
					}
				}
			}
			h.svc.Team.SetItem(mid, iid)
		}
	}

	// Tera type
	h.svc.Team.SetTeraType(mid, r.FormValue("tera_type"))

	// Role & notes
	h.svc.Team.SetRole(mid, r.FormValue("role"))
	h.svc.Team.SetMemberNotes(mid, r.FormValue("notes"))
	h.svc.Team.SetNickname(mid, r.FormValue("nickname"))

	// EVs
	evs := team.StatSpread{
		HP:  formInt(r, "ev_hp"),
		Atk: formInt(r, "ev_atk"),
		Def: formInt(r, "ev_def"),
		SpA: formInt(r, "ev_spa"),
		SpD: formInt(r, "ev_spd"),
		Spe: formInt(r, "ev_spe"),
	}
	// EV validation before saving.
	if evs.Total() > 508 {
		http.Redirect(w, r,
			fmt.Sprintf("/teams/%d/members/%d/edit?error=EV+total+%d+exceeds+508", t.ID, mid, evs.Total()),
			http.StatusFound)
		return
	}
	h.svc.Team.SetEVs(mid, evs)

	// IVs
	ivs := team.StatSpread{
		HP:  formInt(r, "iv_hp"),
		Atk: formInt(r, "iv_atk"),
		Def: formInt(r, "iv_def"),
		SpA: formInt(r, "iv_spa"),
		SpD: formInt(r, "iv_spd"),
		Spe: formInt(r, "iv_spe"),
	}
	h.svc.Team.SetIVs(mid, ivs)

	// Moves — validate learnset before saving.
	var moveIDs []int
	for i := 1; i <= 4; i++ {
		name := strings.TrimSpace(r.FormValue(fmt.Sprintf("move%d", i)))
		if name == "" {
			continue
		}
		mv, err := h.svc.Pokemon.GetMoveByName(name)
		if err != nil {
			http.Redirect(w, r,
				fmt.Sprintf("/teams/%d/members/%d/edit?error=Move+%q+not+found", t.ID, mid, name),
				http.StatusFound)
			return
		}
		ok, _ := h.svc.Pokemon.CanLearnMove(member.Species.ID, mv.ID)
		if !ok {
			http.Redirect(w, r,
				fmt.Sprintf("/teams/%d/members/%d/edit?error=%s+cannot+learn+%s", t.ID, mid, member.Species.Name, mv.Name),
				http.StatusFound)
			return
		}
		moveIDs = append(moveIDs, mv.ID)
	}
	if len(moveIDs) > 0 {
		h.svc.Team.SetMoves(mid, moveIDs)
	}

	http.Redirect(w, r, fmt.Sprintf("/teams/%d?flash=Saved+%s", t.ID, member.Species.Name), http.StatusFound)
}

func (h *handler) memberDelete(w http.ResponseWriter, r *http.Request, teamID, memberID int) {
	h.svc.Team.RemoveMember(memberID)
	http.Redirect(w, r, fmt.Sprintf("/teams/%d?flash=Pokemon+removed", teamID), http.StatusFound)
}

// --- Pokemon, Moves, Items, KB ---

func (h *handler) pokemonList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	results, _ := h.svc.Pokemon.SearchSpecies(q, 50, pokemon.SpeciesFilter{})
	h.render(w, "pokemon_list.html", map[string]any{
		"Query":   q,
		"Results": results,
	})
}

func (h *handler) pokemonDetail(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/pokemon/")
	if name == "" {
		http.NotFound(w, r)
		return
	}

	// Try lookup by name first; fall back to numeric ID for legacy links.
	var sp *pokemon.Species
	var err error
	if id, convErr := strconv.Atoi(name); convErr == nil {
		sp, err = h.svc.Pokemon.GetSpeciesByID(id)
	} else {
		sp, err = h.svc.Pokemon.GetSpeciesByName(name)
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}

	abilities, _ := h.svc.Pokemon.GetAbilitiesForSpecies(sp.ID)
	learnset, _ := h.svc.Pokemon.GetLearnset(sp.ID)

	// Find all teams that contain this species.
	type teamMembership struct {
		TeamID   int
		TeamName string
		Regulation string
		Member   team.Member
	}
	var memberships []teamMembership
	if teams, err := h.svc.Team.ListTeams(); err == nil {
		for _, ts := range teams {
			t, err := h.svc.Team.GetTeam(ts.ID)
			if err != nil {
				continue
			}
			for _, m := range t.Members {
				if m.Species != nil && m.Species.ID == sp.ID {
					memberships = append(memberships, teamMembership{
						TeamID:     t.ID,
						TeamName:   t.Name,
						Regulation: t.Regulation,
						Member:     m,
					})
				}
			}
		}
	}

	h.render(w, "pokemon_dex.html", map[string]any{
		"Species":      sp,
		"Abilities":    abilities,
		"Learnset":     learnset,
		"Memberships":  memberships,
	})
}

func (h *handler) movesList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	moveType := r.URL.Query().Get("type")
	category := r.URL.Query().Get("category")
	moves, _ := h.svc.Pokemon.SearchMoves(q, 50, pokemon.MoveFilter{Type: moveType, Category: category})
	h.render(w, "moves_list.html", map[string]any{
		"Query":   q,
		"Results": moves,
	})
}

func (h *handler) itemsList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	items, _ := h.svc.Pokemon.SearchItems(q, 50, pokemon.ItemFilter{})
	h.render(w, "items_list.html", map[string]any{
		"Query":   q,
		"Results": items,
	})
}

func (h *handler) kbSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	var results []knowledge.SearchResult
	if q != "" {
		results, _ = h.svc.Knowledge.Search(q, 10)
	}
	h.render(w, "kb.html", map[string]any{
		"Query":   q,
		"Results": results,
	})
}

// --- helpers ---

func formInt(r *http.Request, key string) int {
	v, _ := strconv.Atoi(r.FormValue(key))
	return v
}

func url(s string) string {
	return strings.ReplaceAll(s, " ", "+")
}

func (h *handler) chatPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "chat.html", nil)
}

func (h *handler) chatHistory(w http.ResponseWriter, r *http.Request) {
	repo := &chatRepo{db: h.svc.DB}
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		msgs, err := repo.messages()
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		if msgs == nil {
			msgs = []displayMessage{}
		}
		json.NewEncoder(w).Encode(map[string]any{"messages": lastN(msgs, 10)})
	case http.MethodDelete:
		if err := repo.clear(); err != nil {
			json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

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

	repo := &chatRepo{db: h.svc.DB}

	fullCtx, err := repo.loadContext()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	cfg := loadConfig(h.svc.DB)
	hist := fullCtx
	if len(hist) > cfg.Lookback {
		hist = hist[len(hist)-cfg.Lookback:]
	}

	ollama := newOllamaClient(cfg)
	produced, newCtx, view := runAgent(ollama, h.svc, hist, req.Message)

	// Persist display messages.
	_, _ = repo.appendMessage("user", req.Message)
	for _, m := range produced {
		_, _ = repo.appendMessage(m.Role, m.Content)
	}

	// Persist full accumulated context.
	_ = repo.saveContext(append(fullCtx, newCtx...))

	all, _ := repo.messages()
	resp := map[string]any{"messages": lastN(all, 10)}
	if view != nil {
		switch view.Type {
		case "team":
			if t, err := h.svc.Team.GetTeam(view.TeamID); err == nil {
				resp["view"] = map[string]any{"type": "team", "team": t}
			}
		case "pokemon":
			var sp *pokemon.Species
			if s, err := h.svc.Pokemon.GetSpeciesByName(view.PokemonName); err == nil {
				sp = s
			} else if results, err2 := h.svc.Pokemon.SearchSpecies(view.PokemonName, 1, pokemon.SpeciesFilter{}); err2 == nil && len(results) > 0 {
				sp = &results[0]
			}
			if sp != nil {
				abilities, _ := h.svc.Pokemon.GetAbilitiesForSpecies(sp.ID)
				learnset, _ := h.svc.Pokemon.GetLearnset(sp.ID)
				ev := team.EvaluateSpecies(sp, learnset)
				resp["view"] = map[string]any{"type": "pokemon", "species": sp, "abilities": abilities, "eval": ev}
			}
		case "evaluate":
			sp, err := h.svc.Pokemon.GetSpeciesByName(view.PokemonName)
			if err != nil {
				if results, err2 := h.svc.Pokemon.SearchSpecies(view.PokemonName, 1, pokemon.SpeciesFilter{}); err2 == nil && len(results) > 0 {
					sp = &results[0]
					err = nil
				}
			}
			if err == nil {
				if view.TeamID > 0 {
					if t, err2 := h.svc.Team.GetTeam(view.TeamID); err2 == nil {
						for i := range t.Members {
							if t.Members[i].Species != nil &&
								strings.EqualFold(t.Members[i].Species.Name, sp.Name) {
								ev := team.EvaluateMember(&t.Members[i])
								resp["view"] = map[string]any{"type": "evaluate", "eval": ev, "member": true}
								break
							}
						}
					}
				}
				if _, set := resp["view"]; !set {
					learnset, _ := h.svc.Pokemon.GetLearnset(sp.ID)
					ev := team.EvaluateSpecies(sp, learnset)
					resp["view"] = map[string]any{"type": "evaluate", "eval": ev, "member": false}
				}
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *handler) settingsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", 400)
			return
		}
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
			NumCtx:      parseInt("ollama_num_ctx", 16000),
			Temperature: parseFloat("ollama_temp", 0.3),
			TopP:        parseFloat("ollama_top_p", 0.7),
			TopK:        parseInt("ollama_top_k", 20),
			Repeat:      parseFloat("ollama_repeat", 1.1),
			KeepAlive:   r.FormValue("ollama_keep_alive"),
			Lookback:    parseInt("agent_lookback", 10),
			Prompt:      r.FormValue("agent_prompt"),
		}
		_ = saveConfig(h.svc.DB, cfg)
		http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
		return
	}
	cfg := loadConfig(h.svc.DB)
	dbPath, _ := h.svc.DB.Query(`PRAGMA database_list`)
	var path string
	if dbPath != nil {
		defer dbPath.Close()
		var seq int
		var name string
		if dbPath.Next() {
			_ = dbPath.Scan(&seq, &name, &path)
		}
	}
	h.render(w, "settings.html", map[string]any{
		"Cfg":    cfg,
		"DBPath": path,
		"Saved":  r.URL.Query().Get("saved") == "1",
	})
}

func (h *handler) togglePokemonOwned(w http.ResponseWriter, r *http.Request) {
	id := formInt(r, "id")
	owned := r.FormValue("owned") == "1"
	w.Header().Set("Content-Type", "application/json")
	if err := h.svc.Pokemon.SetSpeciesOwned(id, owned); err != nil {
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (h *handler) toggleItemOwned(w http.ResponseWriter, r *http.Request) {
	id := formInt(r, "id")
	owned := r.FormValue("owned") == "1"
	w.Header().Set("Content-Type", "application/json")
	if err := h.svc.Pokemon.SetItemOwned(id, owned); err != nil {
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
