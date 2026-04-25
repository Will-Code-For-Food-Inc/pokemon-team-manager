-- Core Pokemon data tables

CREATE TABLE IF NOT EXISTS species (
    id            INTEGER PRIMARY KEY,
    name          TEXT    NOT NULL UNIQUE,
    form          TEXT    NOT NULL DEFAULT '',
    type1         TEXT    NOT NULL,
    type2         TEXT,
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
    is_restricted INTEGER NOT NULL DEFAULT 0
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
    is_banned   INTEGER NOT NULL DEFAULT 0
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
