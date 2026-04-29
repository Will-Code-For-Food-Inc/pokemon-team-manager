-- Seed ptm.db from CSV files in data/pokemon/.
-- Usage: sqlite3 ptm.db < scripts/seed.sql
--
-- Requires SQLite 3.32+ for .import --skip 1 (header row skipping).
-- Run after `ptm migrate` so tables already exist.
--
-- Staging tables are used where the live schema has extra columns the CSVs
-- intentionally don't carry (user-state like `owned`, AUTOINCREMENT ids).
-- Source data lives in CSVs only — `owned` flags are user state, not source.

.mode csv

-- ── species: CSV omits `owned` (user-state column, default 0) ────────────────
CREATE TEMP TABLE _species_csv (
    slug TEXT, dex_id INTEGER, name TEXT, form TEXT,
    type1 TEXT, type2 TEXT,
    hp INTEGER, attack INTEGER, defense INTEGER,
    sp_attack INTEGER, sp_defense INTEGER, speed INTEGER,
    generation INTEGER,
    is_legendary INTEGER, is_mythical INTEGER,
    is_final_evo INTEGER, is_restricted INTEGER
);
.import --skip 1 data/pokemon/species.csv _species_csv
INSERT OR IGNORE INTO species(
    slug, dex_id, name, form, type1, type2,
    hp, attack, defense, sp_attack, sp_defense, speed,
    generation, is_legendary, is_mythical, is_final_evo, is_restricted
) SELECT
    slug, dex_id, name, form, type1, type2,
    hp, attack, defense, sp_attack, sp_defense, speed,
    generation, is_legendary, is_mythical, is_final_evo, is_restricted
FROM _species_csv;

-- ── items: CSV omits `owned` (user-state) ────────────────────────────────────
CREATE TEMP TABLE _items_csv (
    id INTEGER, name TEXT, slug TEXT, description TEXT,
    is_banned INTEGER, vp_cost INTEGER
);
.import --skip 1 data/pokemon/items.csv _items_csv
INSERT OR IGNORE INTO items(id, name, slug, description, is_banned, vp_cost)
SELECT id, name, slug, description, is_banned, vp_cost FROM _items_csv;

-- ── mega_stones: CSV omits AUTOINCREMENT id ──────────────────────────────────
CREATE TEMP TABLE _mega_stones_csv (
    pokemon_slug TEXT, stone_name TEXT,
    type1_override TEXT, type2_override TEXT,
    mega_ability TEXT
);
.import --skip 1 data/pokemon/mega_stones.csv _mega_stones_csv
INSERT OR IGNORE INTO mega_stones(
    pokemon_slug, stone_name, type1_override, type2_override, mega_ability
) SELECT
    pokemon_slug, stone_name, type1_override, type2_override, mega_ability
FROM _mega_stones_csv;

-- ── direct imports: CSV columns match table columns ──────────────────────────
.import --skip 1 data/pokemon/moves.csv                 moves
.import --skip 1 data/pokemon/abilities.csv             abilities
.import --skip 1 data/pokemon/regulations.csv           regulations
.import --skip 1 data/pokemon/learnsets.csv             learnsets
.import --skip 1 data/pokemon/champions_learnsets.csv   champions_learnsets
.import --skip 1 data/pokemon/species_abilities.csv     species_abilities
.import --skip 1 data/pokemon/regulation_species.csv    regulation_species

-- .import stores empty CSV cells as TEXT ''. Coerce nullable INTEGER columns
-- back to NULL so Go scans (sql.NullInt64) don't choke on "" → int64.
UPDATE moves SET power    = NULL WHERE typeof(power)    = 'text' AND power    = '';
UPDATE moves SET accuracy = NULL WHERE typeof(accuracy) = 'text' AND accuracy = '';

SELECT 'species loaded: '     || count(*) FROM species;
SELECT 'moves loaded: '       || count(*) FROM moves;
SELECT 'abilities loaded: '   || count(*) FROM abilities;
SELECT 'items loaded: '       || count(*) FROM items;
SELECT 'regulations loaded: ' || count(*) FROM regulations;
SELECT 'champions learnsets: '|| count(*) FROM champions_learnsets;
SELECT 'mega stones: '        || count(*) FROM mega_stones;
SELECT 'species_abilities: '  || count(*) FROM species_abilities;
SELECT 'learnsets: '          || count(*) FROM learnsets;
SELECT 'regulation_species: ' || count(*) FROM regulation_species;
