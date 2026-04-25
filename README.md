# ptm — Pokemon Team Manager

An MCP server and CLI tool that lets an LLM agent build, validate, and analyse
competitive Pokemon teams for the **VGC (Video Game Championships)** format.
All Pokemon data lives in a local SQLite database — no network calls required
at runtime.

## Features

- Complete Gen 1–9 Pokemon data: species, moves, abilities, items, learnsets
- VGC format rules engine: species clause, item clause, restricted Pokemon,
  move legality, EV/IV validation, final-evolution-only enforcement
- MCP stdio server: expose all tools to any LLM that speaks
  [Model Context Protocol](https://modelcontextprotocol.io)
- Knowledge base: ingest and semantically search strategy guides, tier lists,
  and VGC rules docs via FTS5 full-text search (vector embedding layer optional)
- Team analysis: type coverage, defensive weaknesses, speed tiers, archetype
  detection, and threat assessment
- Team export: Markdown-formatted team sheets
- Regulation sets A–H with extendable schema

## Architecture

```
cmd/ptm/            — CLI entry point (cobra) + MCP stdio server
internal/db/        — SQLite connection, migrations (modernc.org/sqlite)
internal/pokemon/   — species, moves, abilities, items models + repository
internal/team/      — team CRUD, VGC validation, analysis
internal/knowledge/ — knowledge base: FTS5 search, document ingestion
internal/tools/     — MCP tool handlers (mark3labs/mcp-go)
data/pokemon/       — seed JSON (Gen 1–9, committed — no runtime network calls)
data/knowledge/     — seed markdown docs (VGC rules, strategy guides)
migrations/         — numbered SQL migration files
```

## Requirements

- Go 1.22+
- No CGO required (`modernc.org/sqlite` is pure Go)

## Quick Start

```bash
# Build
make build

# Seed the database with all Pokemon data
make seed

# Start the MCP server (wire this into your LLM client)
./ptm mcp

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

The MCP server exposes the following tools to LLM clients:

**Pokemon Lookup**
- `search_pokemon` — keyword search over species
- `get_pokemon` — full species data (stats, types, abilities)
- `get_moves` — learnset for a species
- `search_moves` — find moves by type/category/description
- `get_item` / `search_items` — item lookup

**Team Management**
- `create_team` — new empty team for a regulation
- `list_teams` / `get_team` — browse teams
- `add_pokemon` / `remove_pokemon` — add/remove species by name
- `set_ability` / `set_nature` / `set_item` — configure a team member
- `set_moves` — assign up to 4 moves
- `set_stats` — distribute EVs equally across 2–3 stats (508 total)
- `set_role` / `set_notes` — label and annotate members
- `validate_team` — returns plain-English list of violations
- `analyse_team` — type coverage, threats, archetypes
- `export_team` — markdown team sheet
- `delete_team`

**Regulations**
- `list_regulations` / `get_regulation`

**Knowledge Base**
- `search_knowledge` — FTS5 search over ingested docs
- `ingest_document` — add a document to the knowledge base

## VGC Rules (2025 Regulation H)

- Double battles, 6 Pokemon registered, 4 brought per match
- Level cap: Lv. 50
- Species clause (no duplicate species), Item clause (no duplicate items)
- Final evolutions only
- Max 0 restricted Legendary/Mythical per team (Regulation H default)
- Banned moves: Swagger, etc.

See `SPEC.md` for the full specification.

## Data

All seed data is committed to `data/pokemon/` as JSON. Sources are derived from
the open PokeAPI dataset. Seed data is loaded once with `ptm seed` and stored
in `ptm.db` (SQLite). No network access is needed after that.

Knowledge base documents can be ingested from local markdown files at any time.

## Development

```bash
make test       # run all tests
make build      # build ./ptm
make seed       # seed ptm.db
make lint       # golangci-lint (if installed)
```

## Identity

This instance runs as **LiraelClaude** — the agentic shard of the primary
Claude Code instance, operating in the `lirael` agentic dev environment.
Named for the protagonist of Garth Nix's *Sabriel* / *Lirael* series.
