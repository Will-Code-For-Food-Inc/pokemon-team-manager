# Pokemon Team Manager — Specification

## Overview

A tool enabling an LLM to create and manage competitive Pokemon teams for the
**Pokemon Video Game Championships (VGC)** format (also marketed as "Pokemon
Championships"). The system stores a complete snapshot of all Pokemon data in a
local SQLite database and exposes an MCP (Model Context Protocol) interface so
an LLM agent can build, validate, and analyse teams with no network calls at
runtime.

## Format: VGC (Video Game Championships)

VGC is the only supported format. Key rules (2025 Regulation H baseline):

- **Battle format**: Double battles (2 vs 2 active Pokemon)
- **Team size**: 6 Pokemon registered, 4 brought to each match
- **Level cap**: All Pokemon set to Lv. 50 in battle
- **Item clause**: No two Pokemon on the same team may hold the same item
- **Species clause**: No two Pokemon of the same species (same National Dex number)
- **Restricted Pokemon**: Each regulation set defines which Legendary/Mythical
  Pokemon are allowed and how many (typically 0–2 restricted slots per team)
- **Banned moves**: Certain moves are banned per regulation (e.g. Swagger)
- **Final evolutions only**: Only fully-evolved Pokemon are legal

Regulation sets stored in the database: A, B, C, D, E, F, G, H (extendable).

## Architecture

```
pokemon-team-manager/
├── cmd/
│   └── ptm/            # main entry point — cobra CLI + MCP stdio server
├── internal/
│   ├── db/             # SQLite connection, migrations, sqlite-vec bindings
│   ├── pokemon/        # species, moves, abilities, items — models + repository
│   ├── team/           # team models, CRUD, VGC validation, analysis
│   ├── knowledge/      # knowledge base: document ingestion + semantic search
│   └── tools/          # MCP tool handlers (JSON-RPC over stdio)
├── data/
│   ├── pokemon/        # seed JSON: species, moves, abilities, items, learnsets
│   └── knowledge/      # seed markdown: VGC rules, strategy guides, tier lists
├── migrations/         # numbered SQL migration files (*.sql)
├── CLAUDE.md
├── SPEC.md
└── README.md
```

### Technology Stack

| Concern | Choice | Notes |
|---|---|---|
| Language | Go 1.22+ | |
| Database | SQLite via `modernc.org/sqlite` | pure Go, no CGO required |
| Vector search | `github.com/asg017/sqlite-vec` | virtual table for embeddings |
| MCP server | `github.com/mark3labs/mcp-go` | MCP over stdio |
| CLI | `cobra` | |
| Testing | `testing` + `testify/assert` | in-memory SQLite for integration tests |
| Godoc | throughout all exported symbols | |

## Database Schema

### Core Tables

```sql
-- Pokemon species (National Dex)
CREATE TABLE species (
    id           INTEGER PRIMARY KEY,   -- National Dex number
    name         TEXT NOT NULL UNIQUE,
    form         TEXT NOT NULL DEFAULT '',
    type1        TEXT NOT NULL,
    type2        TEXT,
    hp           INTEGER NOT NULL,
    attack       INTEGER NOT NULL,
    defense      INTEGER NOT NULL,
    sp_attack    INTEGER NOT NULL,
    sp_defense   INTEGER NOT NULL,
    speed        INTEGER NOT NULL,
    generation   INTEGER NOT NULL,
    is_legendary INTEGER NOT NULL DEFAULT 0,
    is_mythical  INTEGER NOT NULL DEFAULT 0,
    is_final_evo INTEGER NOT NULL DEFAULT 0,
    is_restricted INTEGER NOT NULL DEFAULT 0
);

-- Moves
CREATE TABLE moves (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL,
    category    TEXT NOT NULL CHECK(category IN ('physical','special','status')),
    power       INTEGER,
    accuracy    INTEGER,
    pp          INTEGER NOT NULL,
    priority    INTEGER NOT NULL DEFAULT 0,
    target      TEXT NOT NULL,
    description TEXT NOT NULL
);

-- Abilities
CREATE TABLE abilities (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL
);

-- Items
CREATE TABLE items (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    is_banned   INTEGER NOT NULL DEFAULT 0
);

-- Species learnsets
CREATE TABLE learnsets (
    species_id   INTEGER NOT NULL REFERENCES species(id),
    move_id      INTEGER NOT NULL REFERENCES moves(id),
    learn_method TEXT NOT NULL,
    PRIMARY KEY (species_id, move_id, learn_method)
);

-- Species-ability mapping (up to 3 slots: 1, 2, hidden=3)
CREATE TABLE species_abilities (
    species_id INTEGER NOT NULL REFERENCES species(id),
    ability_id INTEGER NOT NULL REFERENCES abilities(id),
    slot       INTEGER NOT NULL CHECK(slot IN (1,2,3)),
    PRIMARY KEY (species_id, slot)
);

-- VGC Regulation sets
CREATE TABLE regulations (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    start_date     TEXT,
    end_date       TEXT,
    description    TEXT NOT NULL,
    max_restricted INTEGER NOT NULL DEFAULT 0
);

-- Per-regulation species rules
CREATE TABLE regulation_species_rules (
    regulation_id TEXT    NOT NULL REFERENCES regulations(id),
    species_id    INTEGER NOT NULL REFERENCES species(id),
    rule          TEXT    NOT NULL CHECK(rule IN ('banned','restricted')),
    PRIMARY KEY (regulation_id, species_id)
);

-- Per-regulation move bans
CREATE TABLE regulation_move_bans (
    regulation_id TEXT    NOT NULL REFERENCES regulations(id),
    move_id       INTEGER NOT NULL REFERENCES moves(id),
    PRIMARY KEY (regulation_id, move_id)
);
```

### Team Tables

```sql
CREATE TABLE teams (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    regulation TEXT NOT NULL REFERENCES regulations(id),
    notes      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE team_members (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    team_id    INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    slot       INTEGER NOT NULL CHECK(slot BETWEEN 1 AND 6),
    species_id INTEGER NOT NULL REFERENCES species(id),
    nickname   TEXT NOT NULL DEFAULT '',
    ability_id INTEGER NOT NULL REFERENCES abilities(id),
    item_id    INTEGER REFERENCES items(id),
    tera_type  TEXT,
    nature     TEXT NOT NULL,
    role       TEXT NOT NULL DEFAULT '',
    notes      TEXT NOT NULL DEFAULT '',
    ev_hp      INTEGER NOT NULL DEFAULT 0,
    ev_atk     INTEGER NOT NULL DEFAULT 0,
    ev_def     INTEGER NOT NULL DEFAULT 0,
    ev_spa     INTEGER NOT NULL DEFAULT 0,
    ev_spd     INTEGER NOT NULL DEFAULT 0,
    ev_spe     INTEGER NOT NULL DEFAULT 0,
    iv_hp      INTEGER NOT NULL DEFAULT 31,
    iv_atk     INTEGER NOT NULL DEFAULT 31,
    iv_def     INTEGER NOT NULL DEFAULT 31,
    iv_spa     INTEGER NOT NULL DEFAULT 31,
    iv_spd     INTEGER NOT NULL DEFAULT 31,
    iv_spe     INTEGER NOT NULL DEFAULT 31,
    UNIQUE(team_id, slot)
);

CREATE TABLE member_moves (
    member_id INTEGER NOT NULL REFERENCES team_members(id) ON DELETE CASCADE,
    slot      INTEGER NOT NULL CHECK(slot BETWEEN 1 AND 4),
    move_id   INTEGER NOT NULL REFERENCES moves(id),
    PRIMARY KEY (member_id, slot)
);
```

### Knowledge Base Tables

```sql
CREATE TABLE kb_documents (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    title       TEXT NOT NULL,
    source      TEXT NOT NULL,
    content     TEXT NOT NULL,
    chunk_index INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- sqlite-vec virtual table for semantic search (384-dim, all-MiniLM-L6-v2)
CREATE VIRTUAL TABLE kb_embeddings USING vec0(
    document_id INTEGER,
    embedding   FLOAT[384]
);
```

## MCP Tool Interface

All tools exposed over MCP stdio. Tool names use snake_case.

### Pokemon Lookup
| Tool | Inputs | Description |
|---|---|---|
| `search_pokemon` | `query`, `limit` | Semantic + keyword search over species |
| `get_pokemon` | `name_or_id` | Full species data: stats, types, abilities |
| `get_moves` | `pokemon_name` | Full learnset for a species |
| `search_moves` | `query`, `type`, `category` | Find moves by description/type/category |
| `get_item` | `name` | Item details |
| `search_items` | `query` | Find items by name or effect |

### Team Management
| Tool | Inputs | Description |
|---|---|---|
| `create_team` | `name`, `regulation` | Create a new empty team |
| `list_teams` | — | List all teams with member counts |
| `get_team` | `id` | Full team with members and moves |
| `add_pokemon` | `team_id`, `name` | Add Pokemon to next open slot |
| `remove_pokemon` | `team_id`, `name` | Remove Pokemon from team |
| `set_ability` | `team_id`, `pokemon`, `ability` | Set ability |
| `set_nature` | `team_id`, `pokemon`, `nature` | Set nature |
| `set_item` | `team_id`, `pokemon`, `item` | Set item (validates no duplicate) |
| `set_moves` | `team_id`, `pokemon`, `move1..4` | Set up to 4 moves |
| `set_stats` | `team_id`, `pokemon`, `stat1`, `stat2`, `stat3` | Distribute EVs equally across 2–3 stats |
| `set_role` | `team_id`, `pokemon`, `role` | Set strategic role label |
| `set_notes` | `team_id`, `pokemon`, `notes` | Set free-text notes |
| `update_member` | `member_id`, fields... | Update any member field |
| `delete_team` | `id` | Delete team and all members |
| `validate_team` | `team_id` | Check VGC rules, return violations |
| `analyse_team` | `team_id` | Type coverage, threats, archetypes |
| `export_team` | `team_id` | Export team as markdown |

### Knowledge Base
| Tool | Inputs | Description |
|---|---|---|
| `search_knowledge` | `query`, `limit` | Semantic search over strategy docs |
| `ingest_document` | `title`, `source`, `content` | Add document to KB |

### Regulations
| Tool | Inputs | Description |
|---|---|---|
| `list_regulations` | — | List all regulation sets |
| `get_regulation` | `id` | Details, banned/restricted species and moves |

## VGC Validation Rules

`validate_team` checks (returns list of plain-English violation strings):

1. Team has exactly 6 members
2. No duplicate species (same National Dex number)
3. No duplicate items held
4. All moves are in each species' learnset
5. All abilities are legal for the species
6. EVs: each stat 0–252, total ≤ 508
7. IVs: each stat 0–31
8. Restricted count ≤ regulation max
9. No banned moves for the regulation
10. No banned species for the regulation
11. All species are final evolutions

## Team Analysis

`analyse_team` returns:
- Offensive type coverage (types the team hits super-effectively)
- Defensive weaknesses (types that 2+ members share a weakness to)
- Speed tiers per member at Lv. 50 (base speed × nature modifier)
- Archetypes detected (Trick Room, Hyper Offense, Balance, Weather)
- EV investment summary per member
- Top suggested threats based on meta knowledge base

## EV Distribution (`set_stats`)

Stats are distributed as 508 EVs equally across 2 or 3 chosen stats:
- 2 stats: 252 / 252 (4 remaining assigned to first stat → 252/252/4 split handled internally)
- 3 stats: 172 / 172 / 164

## Natures

All 25 natures stored in code (not DB). Each nature boosts one stat ×1.1 and
reduces another ×0.9 (neutral natures leave all stats unchanged). The `set_nature`
tool validates that the named nature exists.

## Data Seeding

Seed data lives in `data/pokemon/` as JSON files committed to the repo. No
network calls at runtime. The `ptm seed` command is idempotent (INSERT OR IGNORE).

```
data/pokemon/
├── species.json     # 1025 species (Gen 1–9)
├── moves.json       # all moves through Gen 9
├── abilities.json   # all abilities
├── items.json       # competitive items
├── learnsets.json   # species → moves
└── regulations.json # VGC regulation metadata
```

## CLI

```
ptm seed               # seed DB from data/
ptm mcp                # start MCP stdio server
ptm team list          # list teams
ptm team show <id>     # show team
ptm team validate <id> # validate team
ptm team analyse <id>  # analyse team
ptm team export <id>   # export team as markdown
ptm kb ingest <file>   # ingest markdown doc
ptm kb search <query>  # semantic search
```

## Testing

- Unit tests: all validation and analysis logic in `internal/team`
- Integration tests: DB queries against in-memory SQLite (`:memory:`)
- Tool tests: MCP handler tests with JSON fixture inputs
- `make test` — run all tests
- `make seed` — seed `ptm.db` in project root
- `make build` — build `ptm` binary

## Non-Goals

- Online battle simulation
- Showdown import/export (stretch goal)
- Formats other than VGC
- Authentication / multi-user
- Real-time meta updates (data is static, seeded from committed files)
