// Package handlers provides pure business-logic functions shared across the
// MCP, web agent, and CLI surfaces. Functions take explicit typed parameters
// and return Go structs — output formatting is the caller's responsibility.
package handlers

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/user/pokemon-team-manager/internal/knowledge"
	"github.com/user/pokemon-team-manager/internal/pokemon"
	"github.com/user/pokemon-team-manager/internal/team"
)

// Services holds the domain repos needed by handler functions.
type Services struct {
	DB        *sql.DB
	Pokemon   *pokemon.Repo
	Team      *team.Repo
	Knowledge *knowledge.Repo
}

// AgentConfig holds Ollama and agent behavioural settings.
type AgentConfig struct {
	OllamaURL   string
	Model       string
	NumCtx      int
	Temperature float64
	TopP        float64
	TopK        int
	Repeat      float64
	KeepAlive   string
	Lookback    int
	Prompt      string
}

// LoadConfig reads AgentConfig from env vars (preferred), then the settings table, falling back to defaults.
func LoadConfig(db *sql.DB) AgentConfig {
	get := func(key, envKey, def string) string {
		if env := os.Getenv(envKey); env != "" {
			return env
		}
		var v string
		if err := db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
			return def
		}
		return v
	}
	getNoEnv := func(key, def string) string {
		var v string
		if err := db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
			return def
		}
		return v
	}
	parseInt := func(key string, def int) int {
		v, err := strconv.Atoi(getNoEnv(key, ""))
		if err != nil {
			return def
		}
		return v
	}
	parseFloat := func(key string, def float64) float64 {
		v, err := strconv.ParseFloat(getNoEnv(key, ""), 64)
		if err != nil {
			return def
		}
		return v
	}
	return AgentConfig{
		OllamaURL:   get("ollama_url", "PTM_OLLAMA_URL", "http://localhost:11434"),
		Model:       get("ollama_model", "PTM_OLLAMA_MODEL", "qwen3.5:9b"),
		NumCtx:      parseInt("ollama_num_ctx", 20000),
		Temperature: parseFloat("ollama_temp", 0.3),
		TopP:        parseFloat("ollama_top_p", 0.7),
		TopK:        parseInt("ollama_top_k", 20),
		Repeat:      parseFloat("ollama_repeat", 1.1),
		KeepAlive:   getNoEnv("ollama_keep_alive", "15m"),
		Lookback:    parseInt("agent_lookback", 10),
		Prompt:      getNoEnv("agent_prompt", PtmSystemPrompt),
	}
}

// SaveConfig writes AgentConfig fields to the settings table.
func SaveConfig(db *sql.DB, cfg AgentConfig) error {
	pairs := [][2]string{
		{"ollama_url", cfg.OllamaURL},
		{"ollama_model", cfg.Model},
		{"ollama_num_ctx", strconv.Itoa(cfg.NumCtx)},
		{"ollama_temp", strconv.FormatFloat(cfg.Temperature, 'f', -1, 64)},
		{"ollama_top_p", strconv.FormatFloat(cfg.TopP, 'f', -1, 64)},
		{"ollama_top_k", strconv.Itoa(cfg.TopK)},
		{"ollama_repeat", strconv.FormatFloat(cfg.Repeat, 'f', -1, 64)},
		{"ollama_keep_alive", cfg.KeepAlive},
		{"agent_lookback", strconv.Itoa(cfg.Lookback)},
		{"agent_prompt", cfg.Prompt},
	}
	for _, p := range pairs {
		if _, err := db.Exec(
			`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
			p[0], p[1],
		); err != nil {
			return err
		}
	}
	return nil
}

// FindPokemonByName searches species by name with owned=true and limit=10 as defaults.
func FindPokemonByName(svc *Services, name string, owned *bool, limit int) ([]pokemon.Species, error) {
	t := true
	if owned == nil {
		owned = &t
	}
	if limit <= 0 {
		limit = 10
	}
	return svc.Pokemon.SearchSpecies(name, limit, pokemon.SpeciesFilter{Owned: owned})
}

// FindPokemonByFilters searches species with filter criteria (owned=true, limit=20 as defaults).
func FindPokemonByFilters(svc *Services, f pokemon.SpeciesFilter, limit int) ([]pokemon.Species, error) {
	return svc.Pokemon.SearchSpeciesForAgent("", limit, f)
}

// ListTeams returns all teams with member counts.
func ListTeams(svc *Services) ([]team.TeamSummary, error) {
	return svc.Team.ListTeams()
}

// GetTeam returns full team details including members, moves, items, and abilities.
func GetTeam(svc *Services, teamID int) (*team.Team, error) {
	return svc.Team.GetTeam(teamID)
}

// ValidateTeam checks a team against VGC rules. Returns an empty slice if legal.
func ValidateTeam(svc *Services, teamID int) ([]team.Violation, error) {
	t, err := svc.Team.GetTeam(teamID)
	if err != nil {
		return nil, err
	}
	reg, _ := svc.Team.GetRegulation(t.Regulation)
	return team.Validate(t, reg, svc.Pokemon), nil
}

// AnalyseTeam returns the team and its analysis result.
func AnalyseTeam(svc *Services, teamID int) (*team.Team, *team.Analysis, error) {
	t, err := svc.Team.GetTeam(teamID)
	if err != nil {
		return nil, nil, err
	}
	return t, team.Analyse(t), nil
}

// SetOwned marks a Pokemon or item as owned/unowned by name.
func SetOwned(svc *Services, kind, name string, owned bool) (string, error) {
	status := "unowned"
	if owned {
		status = "owned"
	}
	switch kind {
	case "pokemon":
		sp, err := svc.Pokemon.GetSpeciesByName(name)
		if err != nil {
			return "", fmt.Errorf("Pokemon not found: %s", name)
		}
		if err := svc.Pokemon.SetSpeciesOwned(sp.Slug, owned); err != nil {
			return "", err
		}
		return sp.Name + " marked as " + status, nil
	case "item":
		it, err := svc.Pokemon.GetItemByName(name)
		if err != nil {
			return "", fmt.Errorf("item not found: %s", name)
		}
		if err := svc.Pokemon.SetItemOwned(it.Slug, owned); err != nil {
			return "", err
		}
		return it.Name + " marked as " + status, nil
	default:
		return "", fmt.Errorf("type must be 'pokemon' or 'item'")
	}
}

// SearchKnowledge searches the strategy knowledge base. Uses vector similarity
// if embeddings exist, falls back to FTS5.
func SearchKnowledge(svc *Services, query string, limit int) ([]knowledge.SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	n, _ := svc.Knowledge.EmbeddingCount()
	if n > 0 {
		cfg := LoadConfig(svc.DB)
		ollama := NewOllamaClient(cfg)
		if vec, err := ollama.Embed(query); err == nil {
			return svc.Knowledge.SearchWithVector(vec, query, limit)
		}
	}
	return svc.Knowledge.Search(query, limit)
}

// StatRow holds the computed values for one stat at Lv50.
type StatRow struct {
	Name  string
	Base  int
	SP    int
	Final int
}

// CalcStatsResult holds the full stat calculation output for a species.
type CalcStatsResult struct {
	SpeciesName string
	Nature      string
	Boosted     pokemon.Stat
	Reduced     pokemon.Stat
	Rows        [6]StatRow
	TotalSP     int
}

// CalcStats computes final Lv50 stats for a species given stat points and nature.
func CalcStats(svc *Services, speciesName string, spread team.StatSpread, nature string) (*CalcStatsResult, error) {
	sp, err := svc.Pokemon.GetSpeciesByName(speciesName)
	if err != nil {
		return nil, err
	}

	bases := [6]int{sp.HP, sp.Attack, sp.Defense, sp.SpAttack, sp.SpDefense, sp.Speed}
	sps := [6]int{spread.HP, spread.Atk, spread.Def, spread.SpA, spread.SpD, spread.Spe}
	statNames := [6]string{"HP", "Attack", "Defense", "Sp.Atk", "Sp.Def", "Speed"}
	statKeys := [6]pokemon.Stat{pokemon.StatHP, pokemon.StatAtk, pokemon.StatDef, pokemon.StatSpA, pokemon.StatSpD, pokemon.StatSpe}

	var boosted, reduced pokemon.Stat
	if nature != "" {
		n, ok := pokemon.NatureByName(nature)
		if !ok {
			return nil, fmt.Errorf("%q is not a valid nature", nature)
		}
		boosted = n.Boosted
		reduced = n.Reduced
	}

	var rows [6]StatRow
	for i := range 6 {
		mult := 1.0
		if statKeys[i] == boosted {
			mult = 1.1
		} else if statKeys[i] == reduced {
			mult = 0.9
		}
		rows[i] = StatRow{
			Name:  statNames[i],
			Base:  bases[i],
			SP:    sps[i],
			Final: team.CalcStat(bases[i], sps[i], statKeys[i] == pokemon.StatHP, mult),
		}
	}

	return &CalcStatsResult{
		SpeciesName: sp.Name,
		Nature:      nature,
		Boosted:     boosted,
		Reduced:     reduced,
		Rows:        rows,
		TotalSP:     spread.Total(),
	}, nil
}

// EvalKind distinguishes the type of EvaluatePokemon result.
type EvalKind int

const (
	EvalSpecies   EvalKind = iota
	EvalMember
	EvalNotOnTeam
)

// EvalResult holds the output of EvaluatePokemon.
type EvalResult struct {
	Kind        EvalKind
	SpeciesEval team.SpeciesEval
	MemberEval  team.MemberEval
	SpeciesName string
	TeamID      int
}

// EvaluatePokemon evaluates a species, or a specific team member if teamID > 0.
func EvaluatePokemon(svc *Services, speciesName string, teamID int) (*EvalResult, error) {
	sp, err := svc.Pokemon.GetSpeciesByName(speciesName)
	if err != nil {
		return nil, err
	}
	if teamID > 0 {
		t, err := svc.Team.GetTeam(teamID)
		if err != nil {
			return nil, fmt.Errorf("loading team: %w", err)
		}
		for i := range t.Members {
			m := &t.Members[i]
			if m.Config != nil && m.Config.Species != nil &&
				strings.EqualFold(m.Config.Species.Name, sp.Name) {
				ev := team.EvaluateMember(m)
				return &EvalResult{Kind: EvalMember, MemberEval: ev}, nil
			}
		}
		return &EvalResult{Kind: EvalNotOnTeam, SpeciesName: sp.Name, TeamID: teamID}, nil
	}
	learnset, _ := svc.Pokemon.GetChampionsLearnset(sp.Slug)
	ev := team.EvaluateSpecies(sp, learnset)
	return &EvalResult{Kind: EvalSpecies, SpeciesEval: ev}, nil
}

// AddTeamLog appends a battle/session journal entry.
func AddTeamLog(svc *Services, teamID int, entry string) (int64, error) {
	return svc.Team.AddLog(teamID, entry)
}

// GetTeamLogs retrieves all journal entries for a team, newest first.
func GetTeamLogs(svc *Services, teamID int) ([]team.TeamLog, error) {
	return svc.Team.GetLogs(teamID)
}
