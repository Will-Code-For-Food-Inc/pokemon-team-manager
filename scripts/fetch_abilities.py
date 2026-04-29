#!/usr/bin/env python3
"""Backfill species_abilities (and abilities) from PokeAPI for ptm v2 schema.

Reads species missing ability rows from the canonical DB, fetches via PokeAPI,
upserts into abilities + species_abilities, then regenerates the canonical CSVs
in data/pokemon/.

PokeAPI courtesy guideline is ~100 req/min. We sleep 1.0s between species
(~60/min) to stay well below.
"""

import csv
import json
import os
import re
import sqlite3
import subprocess
import sys
import time

DB = os.environ.get("PTM_DB", "/home/alex/.local/share/ptm/ptm.db")
DATA_DIR = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "data", "pokemon")
POKEAPI = "https://pokeapi.co/api/v2/pokemon/{}"
SLEEP_BETWEEN = 1.0   # ~60 req/min; PokeAPI courtesy is ≤100/min

# Mapping from our slug convention → PokeAPI slug convention.
# We use full anglicised regional names; PokeAPI uses shortened ones.
REGIONAL_RENAMES = [
    ("-alolan", "-alola"),
    ("-hisuian", "-hisui"),
    ("-galarian", "-galar"),
    ("-paldean-combat", "-paldea-combat"),
    ("-paldean-blaze", "-paldea-blaze"),
    ("-paldean-aqua", "-paldea-aqua"),
    ("-paldean", "-paldea"),
]

# Species that have no PokeAPI form-specific entry; fall back to base slug.
FORM_FALLBACK_TO_BASE = {
    "floette-eternal": "floette",
}

# Direct overrides where PokeAPI's canonical slug differs from ours by more than
# the regional rename rules can capture (e.g. PokeAPI requires a default form name).
DIRECT_SLUG_MAP = {
    "aegislash":         "aegislash-shield",
    "gourgeist-medium":  "gourgeist-average",
    "maushold":          "maushold-family-of-four",
    "mimikyu":           "mimikyu-disguised",
    "morpeko":           "morpeko-full-belly",
    "mr. rime":          "mr-rime",
    "palafin":           "palafin-zero",
}


def to_pokeapi_slug(slug: str) -> list[str]:
    """Return ordered list of slugs to try against PokeAPI for our DB slug."""
    candidates = []
    if slug in DIRECT_SLUG_MAP:
        candidates.append(DIRECT_SLUG_MAP[slug])
    if slug in FORM_FALLBACK_TO_BASE:
        candidates.append(FORM_FALLBACK_TO_BASE[slug])
    converted = slug
    for ours, theirs in REGIONAL_RENAMES:
        converted = converted.replace(ours, theirs)
    candidates.append(converted)
    if converted != slug:
        candidates.append(slug)  # also try as-is
    # Final fallback: strip any form suffix and try the base species
    base = slug.split("-", 1)[0]
    if base != slug and base not in candidates:
        candidates.append(base)
    return candidates


def slugify(name: str) -> str:
    return re.sub(r"[^a-z0-9]+", "-", name.lower()).strip("-")


def fetch(url: str, retries: int = 3):
    for i in range(retries):
        result = subprocess.run(
            ["curl", "-sf", "--max-time", "15", url],
            capture_output=True, text=True,
        )
        if result.returncode == 0 and result.stdout:
            try:
                return json.loads(result.stdout)
            except json.JSONDecodeError:
                return None
        if result.returncode == 22:  # 404
            return None
        if i < retries - 1:
            time.sleep(2 ** i)
    return None


def get_abilities(pokeapi_slug: str):
    data = fetch(POKEAPI.format(pokeapi_slug))
    if not data:
        return []
    return [
        {
            "name": a["ability"]["name"].replace("-", " ").title(),
            "slug": a["ability"]["name"],
            "slot": a["slot"],
        }
        for a in data["abilities"]
    ]


def dump_csvs(con: sqlite3.Connection):
    cur = con.cursor()
    abilities_csv = os.path.join(DATA_DIR, "abilities.csv")
    sa_csv = os.path.join(DATA_DIR, "species_abilities.csv")

    cur.execute("SELECT id, name, slug, description FROM abilities ORDER BY id")
    rows = cur.fetchall()
    with open(abilities_csv, "w", newline="") as f:
        w = csv.writer(f, quoting=csv.QUOTE_NONNUMERIC)
        w.writerow(["id", "name", "slug", "description"])
        w.writerows(rows)

    cur.execute(
        "SELECT species_slug, ability_slug, slot FROM species_abilities "
        "ORDER BY species_slug, slot"
    )
    rows = cur.fetchall()
    with open(sa_csv, "w", newline="") as f:
        w = csv.writer(f, quoting=csv.QUOTE_NONNUMERIC)
        w.writerow(["species_slug", "ability_slug", "slot"])
        w.writerows(rows)

    print(f"\nWrote {abilities_csv} ({len(rows)} not counted; abilities table only)")
    print(f"Wrote {sa_csv} ({len(rows)} rows)")


def main():
    con = sqlite3.connect(DB)
    cur = con.cursor()

    cur.execute("""
        SELECT slug, name, form FROM species
        WHERE slug NOT IN (SELECT species_slug FROM species_abilities)
        ORDER BY slug
    """)
    missing = cur.fetchall()
    print(f"{len(missing)} species missing abilities")
    if not missing:
        dump_csvs(con)
        con.close()
        return

    cur.execute("SELECT slug FROM abilities")
    known_ability_slugs = {r[0] for r in cur.fetchall()}

    def ensure_ability(name: str, slug: str) -> str:
        if slug in known_ability_slugs:
            return slug
        cur.execute(
            "INSERT OR IGNORE INTO abilities(name, slug, description) VALUES(?, ?, '')",
            (name, slug),
        )
        known_ability_slugs.add(slug)
        return slug

    misses = []
    for i, (db_slug, name, form) in enumerate(missing, 1):
        candidates = to_pokeapi_slug(db_slug)
        abilities = []
        used = None
        for cand in candidates:
            abilities = get_abilities(cand)
            if abilities:
                used = cand
                break
            time.sleep(0.3)

        if not abilities:
            misses.append(db_slug)
            print(f"  [{i:3}/{len(missing)}] MISS  {db_slug:30s} (tried {candidates})")
            continue

        ability_names = []
        for a in abilities:
            ensure_ability(a["name"], a["slug"])
            cur.execute(
                "INSERT OR IGNORE INTO species_abilities(species_slug, ability_slug, slot) "
                "VALUES(?, ?, ?)",
                (db_slug, a["slug"], a["slot"]),
            )
            ability_names.append(a["name"])
        print(f"  [{i:3}/{len(missing)}] OK    {db_slug:30s} → {ability_names} (via {used})")
        time.sleep(SLEEP_BETWEEN)

    con.commit()
    dump_csvs(con)
    con.close()

    print(f"\nDone. {len(misses)} miss(es): {misses}")
    if misses:
        sys.exit(1)


if __name__ == "__main__":
    main()
