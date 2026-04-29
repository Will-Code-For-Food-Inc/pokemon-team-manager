-- Squashed schema v2. Clean break — all prior migrations replaced.
-- Junction tables use text slugs for direct CSV importability.

PRAGMA foreign_keys = ON;

-- ── Static game data ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS species (
    slug          TEXT    PRIMARY KEY,
    dex_id        INTEGER NOT NULL,
    name          TEXT    NOT NULL,
    form          TEXT    NOT NULL DEFAULT '',
    type1         TEXT    NOT NULL,
    type2         TEXT    NOT NULL DEFAULT '',
    hp            INTEGER NOT NULL,
    attack        INTEGER NOT NULL,
    defense       INTEGER NOT NULL,
    sp_attack     INTEGER NOT NULL,
    sp_defense    INTEGER NOT NULL,
    speed         INTEGER NOT NULL,
    generation    INTEGER NOT NULL,
    is_legendary  INTEGER NOT NULL DEFAULT 0,
    is_mythical   INTEGER NOT NULL DEFAULT 0,
    is_final_evo  INTEGER NOT NULL DEFAULT 0,
    is_restricted INTEGER NOT NULL DEFAULT 0,
    owned         INTEGER NOT NULL DEFAULT 0,
    UNIQUE(dex_id, form)
);

CREATE TABLE IF NOT EXISTS moves (
    id          INTEGER PRIMARY KEY,
    name        TEXT    NOT NULL UNIQUE,
    slug        TEXT    NOT NULL UNIQUE,
    type        TEXT    NOT NULL,
    category    TEXT    NOT NULL CHECK(category IN ('physical','special','status')),
    power       INTEGER,
    accuracy    INTEGER,
    pp          INTEGER NOT NULL,
    priority    INTEGER NOT NULL DEFAULT 0,
    target      TEXT    NOT NULL DEFAULT 'selected-pokemon',
    description TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS abilities (
    id          INTEGER PRIMARY KEY,
    name        TEXT    NOT NULL UNIQUE,
    slug        TEXT    NOT NULL UNIQUE,
    description TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS items (
    id          INTEGER PRIMARY KEY,
    name        TEXT    NOT NULL UNIQUE,
    slug        TEXT    NOT NULL UNIQUE,
    description TEXT    NOT NULL DEFAULT '',
    is_banned   INTEGER NOT NULL DEFAULT 0,
    vp_cost     INTEGER NOT NULL DEFAULT 700,
    owned       INTEGER NOT NULL DEFAULT 0
);

-- PokeAPI learnsets (fallback; Champions-specific learnsets are authoritative).
CREATE TABLE IF NOT EXISTS learnsets (
    species_slug TEXT NOT NULL REFERENCES species(slug) ON DELETE CASCADE,
    move_slug    TEXT NOT NULL REFERENCES moves(slug)   ON DELETE CASCADE,
    learn_method TEXT NOT NULL DEFAULT 'level-up',
    PRIMARY KEY (species_slug, move_slug)
);

-- Champions-specific curated move pool (authoritative for legality checks).
-- No FKs: Champions uses its own slug conventions which may differ from our static data.
CREATE TABLE IF NOT EXISTS champions_learnsets (
    species_slug TEXT NOT NULL,
    move_slug    TEXT NOT NULL,
    PRIMARY KEY (species_slug, move_slug)
);

CREATE TABLE IF NOT EXISTS species_abilities (
    species_slug TEXT    NOT NULL REFERENCES species(slug)   ON DELETE CASCADE,
    ability_slug TEXT    NOT NULL REFERENCES abilities(slug) ON DELETE CASCADE,
    slot         INTEGER NOT NULL CHECK(slot IN (1, 2, 3)),
    PRIMARY KEY (species_slug, slot)
);

CREATE TABLE IF NOT EXISTS regulations (
    id             TEXT    PRIMARY KEY,
    name           TEXT    NOT NULL,
    start_date     TEXT,
    end_date       TEXT,
    description    TEXT    NOT NULL DEFAULT '',
    max_restricted INTEGER NOT NULL DEFAULT 0,
    active         INTEGER NOT NULL DEFAULT 1
);

-- Species legal in a specific regulation (the allowed roster).
-- No FK on species_slug: Champions uses shortened form names (e.g. alola vs alolan).
CREATE TABLE IF NOT EXISTS regulation_species (
    regulation_id TEXT NOT NULL REFERENCES regulations(id) ON DELETE CASCADE,
    species_slug  TEXT NOT NULL,
    PRIMARY KEY (regulation_id, species_slug)
);

-- Per-regulation bans/restrictions.
CREATE TABLE IF NOT EXISTS regulation_species_rules (
    regulation_id TEXT NOT NULL REFERENCES regulations(id) ON DELETE CASCADE,
    species_slug  TEXT NOT NULL,
    rule          TEXT NOT NULL CHECK(rule IN ('banned','restricted')),
    PRIMARY KEY (regulation_id, species_slug)
);

CREATE TABLE IF NOT EXISTS regulation_move_bans (
    regulation_id TEXT NOT NULL REFERENCES regulations(id) ON DELETE CASCADE,
    move_slug     TEXT NOT NULL,
    PRIMARY KEY (regulation_id, move_slug)
);

-- No FK on pokemon_slug: Champions may use shortened form names.
CREATE TABLE IF NOT EXISTS mega_stones (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    pokemon_slug   TEXT    NOT NULL,
    stone_name     TEXT    NOT NULL,
    type1_override TEXT    NOT NULL DEFAULT '',
    type2_override TEXT    NOT NULL DEFAULT '',
    mega_ability   TEXT    NOT NULL DEFAULT '',
    UNIQUE(pokemon_slug, stone_name)
);

-- ── Pokemon configurations ────────────────────────────────────────────────────

-- A config is a fully-specified build for one Pokemon.
-- Configs belong to teams via team_slots; they are not shared across teams.
CREATE TABLE IF NOT EXISTS pokemon_configs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    species_slug TEXT    NOT NULL REFERENCES species(slug),
    nickname     TEXT    NOT NULL DEFAULT '',
    nature       TEXT    NOT NULL DEFAULT 'Serious',
    ability_slug TEXT    NOT NULL REFERENCES abilities(slug),
    item_slug    TEXT    REFERENCES items(slug),
    tera_type    TEXT    NOT NULL DEFAULT '',
    role         TEXT    NOT NULL DEFAULT '',
    notes        TEXT    NOT NULL DEFAULT '',
    sp_hp        INTEGER NOT NULL DEFAULT 0,
    sp_atk       INTEGER NOT NULL DEFAULT 0,
    sp_def       INTEGER NOT NULL DEFAULT 0,
    sp_spa       INTEGER NOT NULL DEFAULT 0,
    sp_spd       INTEGER NOT NULL DEFAULT 0,
    sp_spe       INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS config_moves (
    config_id INTEGER NOT NULL REFERENCES pokemon_configs(id) ON DELETE CASCADE,
    slot      INTEGER NOT NULL CHECK(slot BETWEEN 1 AND 4),
    move_slug TEXT    NOT NULL REFERENCES moves(slug),
    PRIMARY KEY (config_id, slot)
);

-- ── Teams ─────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS teams (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL,
    regulation TEXT    NOT NULL REFERENCES regulations(id),
    strategy   TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Each slot on a team points to exactly one config.
-- A config may only occupy one slot (UNIQUE config_id enforces no sharing).
CREATE TABLE IF NOT EXISTS team_slots (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    team_id   INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    slot      INTEGER NOT NULL CHECK(slot BETWEEN 1 AND 6),
    config_id INTEGER NOT NULL UNIQUE REFERENCES pokemon_configs(id) ON DELETE CASCADE,
    UNIQUE(team_id, slot)
);

CREATE TABLE IF NOT EXISTS team_logs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    team_id    INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    entry      TEXT    NOT NULL,
    created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- ── Knowledge base ────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS kb_documents (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    title       TEXT    NOT NULL,
    source      TEXT    NOT NULL DEFAULT '',
    content     TEXT    NOT NULL,
    chunk_index INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    expires_at  TEXT    NOT NULL DEFAULT (datetime('now', '+6 months')),
    reviewed_at TEXT
);

CREATE VIRTUAL TABLE IF NOT EXISTS kb_fts USING fts5(
    title, content,
    content='kb_documents',
    content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS kb_fts_insert AFTER INSERT ON kb_documents BEGIN
    INSERT INTO kb_fts(rowid, title, content) VALUES (new.id, new.title, new.content);
END;
CREATE TRIGGER IF NOT EXISTS kb_fts_delete AFTER DELETE ON kb_documents BEGIN
    INSERT INTO kb_fts(kb_fts, rowid, title, content) VALUES ('delete', old.id, old.title, old.content);
END;
CREATE TRIGGER IF NOT EXISTS kb_fts_update AFTER UPDATE ON kb_documents BEGIN
    INSERT INTO kb_fts(kb_fts, rowid, title, content) VALUES ('delete', old.id, old.title, old.content);
    INSERT INTO kb_fts(rowid, title, content) VALUES (new.id, new.title, new.content);
END;

CREATE TABLE IF NOT EXISTS kb_embeddings (
    document_id INTEGER NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
    embedding   BLOB    NOT NULL,
    PRIMARY KEY (document_id)
);

-- ── Chat ──────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS chat_messages (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    role       TEXT    NOT NULL,
    content    TEXT    NOT NULL,
    in_context INTEGER NOT NULL DEFAULT 0,
    embedding  BLOB,
    created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS chat_context (
    id       INTEGER PRIMARY KEY CHECK (id = 1),
    messages TEXT    NOT NULL DEFAULT '[]'
);

-- ── Settings ──────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT OR IGNORE INTO settings(key, value) VALUES
    ('ollama_url',       'http://localhost:11434'),
    ('ollama_model',     'qwen3.5:9b'),
    ('ollama_num_ctx',   '20000'),
    ('ollama_temp',      '0.3'),
    ('ollama_top_p',     '0.7'),
    ('ollama_top_k',     '20'),
    ('ollama_repeat',    '1.1'),
    ('ollama_keep_alive','15m'),
    ('agent_lookback',   '10'),
    ('agent_prompt',     'You are a Pokemon VGC team building assistant for Pokemon Champions (2026).
Help the user analyse teams, compare Pokemon, calculate stats, and discuss strategy.
Be concise and data-focused. Use tools to look up real data before answering.

Key rules for Pokemon Champions:
- 66 total stat points per Pokemon, max 32 per stat
- All IVs are fixed at 31
- 21 natures — Serious is the free neutral (no VP cost); all others cost 500 VP
- 6 Pokemon per team, bring 4 to battle, double battles
- Current regulation: M-A (no restricted Legendaries)
- Use add_team_log to record battle results and learnings.');
