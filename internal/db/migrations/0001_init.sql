-- Squashed initial schema (replaces migrations 0001–0008).

CREATE TABLE IF NOT EXISTS species (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
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
    name        TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL,
    category    TEXT NOT NULL CHECK(category IN ('physical','special','status')),
    power       INTEGER,
    accuracy    INTEGER,
    pp          INTEGER NOT NULL,
    priority    INTEGER NOT NULL DEFAULT 0,
    target      TEXT NOT NULL DEFAULT 'selected-pokemon',
    description TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS abilities (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS items (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    is_banned   INTEGER NOT NULL DEFAULT 0,
    owned       INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS learnsets (
    species_id   INTEGER NOT NULL REFERENCES species(id),
    move_id      INTEGER NOT NULL REFERENCES moves(id),
    learn_method TEXT NOT NULL DEFAULT 'level-up',
    PRIMARY KEY (species_id, move_id, learn_method)
);

CREATE TABLE IF NOT EXISTS species_abilities (
    species_id INTEGER NOT NULL REFERENCES species(id),
    ability_id INTEGER NOT NULL REFERENCES abilities(id),
    slot       INTEGER NOT NULL CHECK(slot IN (1, 2, 3)),
    PRIMARY KEY (species_id, slot)
);

CREATE TABLE IF NOT EXISTS regulations (
    id             TEXT    PRIMARY KEY,
    name           TEXT    NOT NULL,
    start_date     TEXT,
    end_date       TEXT,
    description    TEXT    NOT NULL DEFAULT '',
    max_restricted INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS regulation_species_rules (
    regulation_id TEXT    NOT NULL REFERENCES regulations(id),
    species_id    INTEGER NOT NULL REFERENCES species(id),
    rule          TEXT    NOT NULL CHECK(rule IN ('banned','restricted')),
    PRIMARY KEY (regulation_id, species_id)
);

CREATE TABLE IF NOT EXISTS regulation_move_bans (
    regulation_id TEXT    NOT NULL REFERENCES regulations(id),
    move_id       INTEGER NOT NULL REFERENCES moves(id),
    PRIMARY KEY (regulation_id, move_id)
);

CREATE TABLE IF NOT EXISTS teams (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    regulation TEXT NOT NULL REFERENCES regulations(id),
    notes      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS team_members (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    team_id    INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    slot       INTEGER NOT NULL CHECK(slot BETWEEN 1 AND 6),
    species_id INTEGER NOT NULL REFERENCES species(id),
    nickname   TEXT    NOT NULL DEFAULT '',
    ability_id INTEGER NOT NULL REFERENCES abilities(id),
    item_id    INTEGER REFERENCES items(id),
    tera_type  TEXT,
    nature     TEXT    NOT NULL DEFAULT 'Hardy',
    role       TEXT    NOT NULL DEFAULT '',
    notes      TEXT    NOT NULL DEFAULT '',
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

CREATE TABLE IF NOT EXISTS member_moves (
    member_id INTEGER NOT NULL REFERENCES team_members(id) ON DELETE CASCADE,
    slot      INTEGER NOT NULL CHECK(slot BETWEEN 1 AND 4),
    move_id   INTEGER NOT NULL REFERENCES moves(id),
    PRIMARY KEY (member_id, slot)
);

CREATE TABLE IF NOT EXISTS kb_documents (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    title       TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT '',
    content     TEXT NOT NULL,
    chunk_index INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE VIRTUAL TABLE IF NOT EXISTS kb_fts USING fts5(
    title,
    content,
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

CREATE TABLE IF NOT EXISTS chat_messages (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    role       TEXT    NOT NULL,
    content    TEXT    NOT NULL,
    in_context INTEGER NOT NULL DEFAULT 0,
    created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS chat_context (
    id       INTEGER PRIMARY KEY CHECK (id = 1),
    messages TEXT    NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT OR IGNORE INTO settings(key, value) VALUES
    ('ollama_url',      'http://localhost:11434'),
    ('ollama_model',    'qwen3.5:9b'),
    ('ollama_num_ctx',  '16000'),
    ('ollama_temp',     '0.3'),
    ('ollama_top_p',    '0.7'),
    ('ollama_top_k',    '20'),
    ('ollama_repeat',   '1.1'),
    ('ollama_keep_alive','15m'),
    ('agent_lookback',  '10'),
    ('agent_prompt',    'You are a Pokemon VGC team building assistant for Pokemon Champions (2026).
Help the user analyse teams, compare Pokemon, calculate stats, and discuss strategy.
Be concise and data-focused. Use tools to look up real data before answering.

Key rules for Pokemon Champions:
- 66 total stat points per Pokemon, max 32 per stat
- All IVs are fixed at 31
- 21 Stat Alignments (natures) — Hardy/Docile/Bashful/Quirky removed, only Serious is neutral
- 6 Pokemon per team, bring 4 to battle, double battles
- Current regulation: I2 (no restricted Legendaries)');
