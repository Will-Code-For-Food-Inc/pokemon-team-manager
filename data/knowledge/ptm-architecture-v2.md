# ptm Architecture v2

## Schema Overview

### Config-centric team model

Previously all build data (ability, nature, moves, EVs) lived inline on `team_members`. Now:

```
pokemon_configs   — owns every build decision for one Pokemon
config_moves      — moves for a config (up to 4, ordered by slot)
team_slots        — links a team to configs; each slot = one config, no sharing
teams             — metadata + strategy (long-form gameplan notes)
```

A `team.Member` in Go is just a slot number + pointer to a `Config`. All the build data lives on `Config`.

### Slug-based references

All junction tables use `TEXT` slugs instead of integer FKs:
- `learnsets(species_slug, move_slug)` — PokeAPI learnset fallback
- `champions_learnsets(species_slug, move_slug)` — authoritative Champions learnset
- `species_abilities(species_slug, ability_slug, slot)`
- `regulation_species(regulation_id, species_slug)`

Champions-sourced tables have **no FK constraints** because the Champions builder uses shortened form names (e.g. `arcanine-hisui` vs our `arcanine-hisuian`).

## Seeding

CSV files in `data/pokemon/` are the source of truth for static game data:

```
make seed   # runs sqlite3 ptm.db < scripts/seed.sql
```

For in-memory tests, `db.Seed(db, dataDir)` reads the same CSVs via Go's `encoding/csv`. Junction tables for species_abilities and learnsets also resolve legacy JSON (PokeAPI internal IDs) via SQL JOIN.

### Owned state stays DB-only

Species.owned and Item.owned are database-only user state — not in CSVs. Export with a future `ptm export` command if needed.

## VP Costs (Champions format)

| Cost | Amount |
|------|--------|
| Recruit | 800 VP / Pokemon |
| Stat point | 5 VP each |
| Nature (non-Serious) | 500 VP |
| Move | 250 VP each |
| Hidden ability | 500 VP |
| Common item | 700 VP |
| Berry (resist/cure) | 400 VP |
| Special item | 1000 VP |
| Mega Stone | 2000 VP |

Serious is the free neutral nature (0 VP). Hardy/Docile/Bashful/Quirky are not valid in Champions format.

## File Layout

```
internal/
  pokemon/
    models.go         — Species, Move, Ability, Item (all have Slug field)
    repo.go           — GetSpeciesBySlug, GetChampionsLearnset, CanLearnMove(slug,slug), HasAbility(slug,slug)
  team/
    models.go         — Team, Member, Config, StatSpread
    repo.go           — AddMember(teamID, speciesSlug, abilitySlug), SetMoves([]string slugs)
    validate.go       — Regulation uses slug maps; RepoChecker interface is slug-based
    analyse.go        — Accesses config data via m.Config.*
  handlers/
    handlers.go       — Pure business logic: ValidateTeam, CalcStats, EvaluatePokemon
    agent.go          — OllamaClient, RunAgent, EmbedAndSave, wire types
    agent_tools.go    — BuildPtmTools() — all ~35 agent tool definitions
  tools/              — MCP tool layer (team_tools.go, pokemon_tools.go, etc.)
  db/
    seed.go           — CSV seeder + legacy JSON seed for junction tables
    migrations/0001_init.sql — Full squashed schema
data/pokemon/
  *.csv               — Source of truth for game data (species, moves, abilities, items, etc.)
  champions_learnsets.csv — From Champions builder (authoritative move pool per species)
  regulation_species.csv  — Legal roster for M-A regulation
  mega_stones.csv     — Mega stone data with type/ability overrides
scripts/
  seed.sql            — sqlite3 .import commands for all CSV files
```

## Key API Changes from v1

- `AddMember(teamID int, speciesSlug, abilitySlug string)` — takes slugs
- `SetAbility(configID int, abilitySlug string)` — takes slug
- `SetItem(configID int, itemSlug string)` — takes slug (empty string to clear)
- `SetMoves(configID int, moveSlugs []string)` — takes slug slice
- `GetConfigByTeamAndSpecies(teamID int, speciesName string) (int, error)`
- `UpdateTeamStrategy(id int, strategy string)` — was UpdateTeamNotes
- `SetConfigNotes(configID int, notes string)` — was SetMemberNotes

## Open Issues

- #45: Structured meta_rules table for non-stochastic VGC heuristics
- Species slug convention mismatch between Champions builder (alola/hisui) and PokeAPI (alolan/hisuian) — affects alternate form data quality
- 51 Champion-legal moves not yet in moves.csv (silently handled by LEFT JOIN in GetChampionsLearnset)
