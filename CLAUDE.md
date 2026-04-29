# CLAUDE.md — ptm (Pokemon Team Manager)

## Identity

This project is worked on by **LiraelClaude** — a Claude Code shard running on
the `lirael` agentic dev VM (NixOS, `10.2.0.10`), named for the protagonist of
Garth Nix's *Sabriel* / *Lirael* series.

## Project Summary

`ptm` is a Go MCP server + CLI that lets an LLM build and manage VGC (Video
Game Championships) Pokemon teams. All data is local SQLite — no network calls
at runtime.

## Key Constraints

- **SQLite only** — no external databases, no network at runtime
- **VGC format only** — no other Pokemon formats are supported
- **Go** — godoc on all exported symbols
- **Small model target** — tool descriptions must be concise; `get_help` tool
  provides a compact overview; don't bloat responses

## Build & Run

```bash
make build        # builds ./ptm
make seed         # seeds ptm.db from data/
make test         # runs all tests
./ptm mcp         # starts MCP stdio server
./ptm team list   # CLI

# Web UI — always port 9133; DB is ~/.local/share/ptm/ptm.db (not the repo db)
nohup ./ptm web --addr 0.0.0.0:9133 > /tmp/ptm.log 2>&1 &
# To restart: lsof -i -P -n | grep ptm  →  kill <PID>  →  re-run above
```

## Architecture

```
cmd/ptm/          main.go — cobra CLI + MCP server wiring
internal/db/      db.go (open/migrate), seed.go (JSON → SQLite), migrations/*.sql
internal/pokemon/ models.go (types/natures/structs), repo.go (CRUD queries)
internal/team/    models.go, repo.go (CRUD), validate.go (VGC rules), analyse.go, export.go
internal/knowledge/ repo.go (FTS5 search, chunked ingestion)
internal/tools/   tools.go (all MCP tool handlers + get_help; see issue #32 to split)
data/pokemon/     seed JSON (species, moves, abilities, items, learnsets, regulations)
data/knowledge/   strategy docs (VGC rules, archetypes, meta, tool guide)
```

## Data Seeding

Seed JSON lives in `data/pokemon/`. Run `make seed` (idempotent). To add more
seed data, edit the JSON files and re-run.

Knowledge base docs live in `data/knowledge/`. Ingest them with:
```bash
./ptm kb ingest data/knowledge/vgc-format-overview.md
```

## Testing

Integration tests use in-memory SQLite (`db.OpenMemory()`). All VGC validation
logic has unit tests in `internal/team/`. Run `make test`.

## Combat Logs

Teams have a separate combat/session log (`team_logs` table, migration 0002).
Two tools: `add_team_log` (record an experience) and `get_team_logs` (retrieve history).
Logs are intentionally separate from `teams.notes` — notes are strategic, logs are experiential.

## Adding a New Tool

1. Add handler in `internal/tools/tools.go` inside the appropriate `register*` function
2. Keep descriptions ≤ 100 chars — small models have limited context
3. Return plain-English errors (model must understand without ambiguity)
4. Add a test in `internal/tools/tools_test.go`

## VGC Rules Quick Reference

- 6 Pokemon, bring 4, double battles
- Level 50 cap in battle
- Species clause, item clause
- Roster is curated per regulation (regulation_species table); not every Pokemon exists. There is no general "final-evolution-only" rule — Pikachu is in the roster despite not being a final evo.
- Stat points: 0-32 per stat, 66 total; no IV adjustment in Champions
- Moves selected from a curated per-species pool (not traditional learnset); use `get_moves` to check
- Customisation costs VP: 2/stat point, 100/move, 200/nature, 400/ability
- Regulation H (2025): 0 restricted Legendaries

## Regulations Banlist Source

Regulation rules are seeded from `data/pokemon/regulations.json`. Banned/restricted
species and moves per regulation are in `data/pokemon/regulation_species_rules.json`
and `data/pokemon/regulation_move_bans.json` (create these files to populate).
