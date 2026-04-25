// Package web provides a server-rendered HTTP interface for the Pokemon Team Manager.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/user/pokemon-team-manager/internal/knowledge"
	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

//go:embed templates/*.html
var templateFS embed.FS

// Services holds all domain repos the web handlers need.
type Services struct {
	Pokemon   *pokemon.Repo
	Team      *team.Repo
	Knowledge *knowledge.Repo
}

var funcMap = template.FuncMap{
	"statbar": func(v int) int {
		if v > 120 {
			return 120
		}
		return v
	},
	"add":      func(a, b int) int { return a + b },
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
	mux.HandleFunc("/moves", h.movesList)
	mux.HandleFunc("/items", h.itemsList)
	mux.HandleFunc("/kb", h.kbSearch)

	return mux
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
	allSpecies, _ := h.svc.Pokemon.SearchSpecies("", 2000)
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

func (h *handler) teamValidate(w http.ResponseWriter, r *http.Request, teamID int) {
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

func (h *handler) teamAnalyse(w http.ResponseWriter, r *http.Request, teamID int) {
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

func (h *handler) teamExport(w http.ResponseWriter, r *http.Request, teamID int) {
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
	items, _ := h.svc.Pokemon.SearchItems("", 200)
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
	results, _ := h.svc.Pokemon.SearchSpecies(q, 50)
	h.render(w, "pokemon_list.html", map[string]any{
		"Query":   q,
		"Results": results,
	})
}

func (h *handler) pokemonDetail(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/pokemon/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sp, err := h.svc.Pokemon.GetSpeciesByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	abilities, _ := h.svc.Pokemon.GetAbilitiesForSpecies(sp.ID)
	learnset, _ := h.svc.Pokemon.GetLearnset(sp.ID)
	// Reuse pokemon_list template with a single result for now; a detail page could be added later.
	h.render(w, "pokemon_list.html", map[string]any{
		"Query":     sp.Name,
		"Results":   []pokemon.Species{*sp},
		"Abilities": abilities,
		"Learnset":  learnset,
	})
}

func (h *handler) movesList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	moveType := r.URL.Query().Get("type")
	category := r.URL.Query().Get("category")
	moves, _ := h.svc.Pokemon.SearchMoves(q, moveType, category, 50)
	h.render(w, "moves_list.html", map[string]any{
		"Query":   q,
		"Results": moves,
	})
}

func (h *handler) itemsList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	items, _ := h.svc.Pokemon.SearchItems(q, 50)
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
