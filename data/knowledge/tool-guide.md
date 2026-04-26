# ptm Tool Guide

Quick reference for the MCP tools available in the Pokemon Team Manager.

## Workflow: Building a Team

1. `list_regulations` — pick a regulation (use "H" for current 2025)
2. `create_team name="My Team" regulation="H"` — creates empty team, returns team_id
3. `copy_team team_id=1 new_name="My Team v2"` — copy an existing team before editing
4. `find_pokemon_by_name name="Garchomp"` — find a species by name
5. `add_pokemon team_id=1 pokemon_name="Garchomp"` — adds to next open slot
6. `remove_pokemon team_id=1 pokemon_name="Garchomp"` — removes a member from the team
7. `set_ability team_id=1 pokemon_name="Garchomp" ability="Rough Skin"`
8. `set_nature team_id=1 pokemon_name="Garchomp" nature="Jolly"`
9. `set_item team_id=1 pokemon_name="Garchomp" item="Choice Scarf"`
10. `set_moves team_id=1 pokemon_name="Garchomp" move1="Earthquake" move2="Rock Slide" move3="Dragon Claw" move4="Protect"`
11. `set_stats team_id=1 pokemon_name="Garchomp" hp=22 attack=32 speed=12` — Champions stat points (max 32/stat, 66 total)
12. `set_role team_id=1 pokemon_name="Garchomp" role="physical attacker"`
13. Repeat steps 5–12 for all 6 slots
14. `validate_team team_id=1` — check for violations
15. `analyse_team team_id=1` — type coverage, speed tiers, archetypes
16. `export_team team_id=1` — markdown team sheet

## Champions Format Rules (NOT standard VGC)

- 6 Pokemon, bring 4 per match, double battles
- Final evolutions only (Pikachu excepted)
- **Stat points: 0–32 per stat, 66 total** (not EVs/IVs)
- No duplicate species, no duplicate items
- Moves selected from a curated per-species pool — use `get_moves` to check legal moves
- Regulation H (2025): 0 restricted Legendaries allowed

## Swapping a Pokemon

To replace one Pokemon with another:
1. `copy_team` first to preserve the original
2. `remove_pokemon team_id=X pokemon_name="OldPoke"` — removes it
3. `add_pokemon team_id=X pokemon_name="NewPoke"` — adds replacement
4. Configure with `set_ability`, `set_item`, `set_moves`, `set_stats`

## Valid Stats for set_stats

`hp` `attack` `defense` `sp_attack` `sp_defense` `speed` — total must not exceed 66

## Search Tips

- `find_pokemon_by_name name="fire"` — finds by name substring
- `find_pokemon_by_filters type="ghost" owned=true` — filter by type/owned
- `search_moves query="spread"` — find spread moves
- `search_items query="berry"` — find berries
- `search_knowledge query="trick room setters"` — strategy docs
- `get_moves team_id=X pokemon_name="Garchomp"` — legal move pool for a species
