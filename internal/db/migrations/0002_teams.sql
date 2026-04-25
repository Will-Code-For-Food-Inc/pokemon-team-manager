-- Team management tables

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
