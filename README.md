# ptm — Pokemon Team Manager

An MCP server, CLI, and web UI that lets an LLM agent build, validate, and
analyse competitive Pokemon teams for the **VGC (Video Game Championships)**
format. All Pokemon data lives in a local SQLite database — no network calls
required at runtime.

## Features

- Complete Gen 1–9 Pokemon data: species, moves, abilities, items, learnsets
- VGC format rules engine: species clause, item clause, restricted Pokemon,
  move legality, stat validation, final-evolution-only enforcement
- MCP stdio server: expose all tools to any LLM that speaks
  [Model Context Protocol](https://modelcontextprotocol.io)
- Web UI with AI agent chat backed by a local Ollama model (no cloud required)
- Animated side panel in the web UI surfaces Pokemon/team cards as the agent works
- Knowledge base: ingest and search strategy guides and VGC rules via FTS5
- Team analysis: type coverage, defensive weaknesses, speed tiers, archetype
  detection, per-Pokemon evaluation, threat assessment
- Team export: Markdown-formatted team sheets
- Regulation sets A–I2/M-A with extendable schema
- Nix flake with `nixosModules.ptm` for a user systemd service
- GitHub Actions CI; cross-compiled releases (linux/amd64/arm64/armv7/386/riscv64) + .deb/.rpm on tags

## Architecture

```
cmd/ptm/            — CLI entry point (cobra) + MCP stdio server
internal/db/        — SQLite connection, migrations (modernc.org/sqlite)
internal/pokemon/   — species, moves, abilities, items models + repository
internal/team/      — team CRUD, VGC validation, analysis, export
internal/knowledge/ — knowledge base: FTS5 search, document ingestion
internal/tools/     — MCP tool handlers (mark3labs/mcp-go)
internal/web/       — web UI server + Ollama-backed agent chat handler
data/pokemon/       — seed JSON (Gen 1–9, committed — no runtime network calls)
data/knowledge/     — seed markdown docs (VGC rules, strategy guides)
migrations/         — numbered SQL migration files
```

## Requirements

- Go 1.22+
- No CGO required (`modernc.org/sqlite` is pure Go)
- [Ollama](https://ollama.ai) — optional, required only for web UI agent chat

## Quick Start

```bash
# Build
make build

# Seed the database with all Pokemon data
make seed

# Start the MCP server (wire this into your LLM client)
./ptm mcp

# Start the web UI (Ollama optional; chat feature requires it)
./ptm web --addr 0.0.0.0:9133 --db ./ptm.db

# CLI usage
./ptm team list
./ptm team show 1
./ptm team validate 1
./ptm team analyse 1
./ptm team export 1

# Knowledge base
./ptm kb ingest data/knowledge/vgc-rules.md
./ptm kb search "trick room setters"
```

## MCP Tools

**Pokemon**
- `find_pokemon_by_name name="..."` — fuzzy name lookup (word-by-word LIKE); use when you know the name
- `find_pokemon_by_filters type="steel" role="support" speed_tier="slow"` — discovery by type/role/speed tier; defaults to owned Pokemon
- `search_pokemon` — last-resort kitchen-sink keyword search
- `get_pokemon` — full species data (stats, types, abilities)
- `evaluate_pokemon pokemon_name="..." [team_id=N]` — two-layer analysis:
  - Species level: defensive type chart, offensive coverage from learnset, stat role (physical/special/mixed/support/tank), speed tier (fast >100 / mid 70–100 / slow <70), physical and special bulk scores
  - Member level (with `team_id`): move/stat mismatch detection, priority/setup/recovery/redirection flags, EV efficiency check
- `get_moves` — learnset for a species
- `search_moves` — find moves by type/category/description
- `get_item` / `search_items` — item lookup
- `set_owned` — mark a species as owned

**Team Management**
- `create_team` / `list_teams` / `get_team` / `rename_team` / `copy_team` / `delete_team`
- `add_pokemon` / `remove_pokemon` / `swap_slots`
- `set_ability` / `set_nature` / `set_item` / `set_moves`
- `set_stats` — distribute stat points (66 total, max 32 per stat; Champions format)
- `set_role` / `set_notes` / `set_nickname` / `set_tera_type`
- `set_team_notes`
- `validate_team` — plain-English list of rule violations
- `analyse_team` — type coverage, threats, archetypes
- `export_team` — Markdown team sheet
- `calc_stats` — compute final stats from base/nature/EVs
- `training_cost` — VP cost for current customisation
- `add_team_log` / `get_team_logs` — per-team experiential combat log (separate from `notes`)

**Regulations**
- `list_regulations` / `get_regulation`

**Knowledge Base**
- `search_knowledge` — FTS5 search over ingested docs
- `ingest_document` — add a document to the knowledge base

## VGC Rules (2025 Regulation I2 / M-A)

- Double battles, 6 Pokemon registered, 4 brought per match
- Level cap: Lv. 50
- Species clause (no duplicate species), Item clause (no duplicate items)
- Final evolutions only (exception: Pikachu is permitted)
- 0 restricted Legendaries/Mythicals (Regulation I2 / M-A default)
- Champions format stat points: 66 total, max 32 per stat, all IVs fixed at 31
- Banned moves: see `data/pokemon/regulation_move_bans.json`

## Data

All seed data is committed to `data/pokemon/` as JSON. Seed data is loaded once
with `make seed` and stored in `ptm.db` (SQLite). No network access is needed
after that.

Knowledge base documents can be ingested from local markdown files at any time:

```bash
./ptm kb ingest data/knowledge/vgc-format-overview.md
```

## Development

```bash
make build      # build ./ptm
make seed       # seed ptm.db
make test       # run all tests (integration tests use in-memory SQLite)
make lint       # golangci-lint (if installed)
```

Integration tests use `db.OpenMemory()`. VGC validation unit tests live in
`internal/team/`. See `CLAUDE.md` for tool authoring guidelines.

## Distribution

A Nix flake (`flake.nix`) provides a `nixosModules.ptm` output for running ptm
as a user systemd service on NixOS. Tagged releases produce cross-compiled
binaries and `.deb`/`.rpm` packages via GitHub Actions + nfpm.

## AI Disclosure

This project was built with significant AI assistance. Code, architecture, and tooling were developed collaboratively using [Claude Code](https://claude.ai/code) (Anthropic) running as a local agentic shard, with Ollama/Qwen used for mechanical and data-heavy tasks. All Pokemon data in `data/` was gathered and curated independently by the project author — it is not AI-generated.

## Identity

This instance runs as **LiraelClaude** — the agentic shard of the primary
Claude Code instance, operating in the `lirael` agentic dev environment
(`10.2.0.10`, NixOS). Named for the protagonist of Garth Nix's *Sabriel* /
*Lirael* series.
