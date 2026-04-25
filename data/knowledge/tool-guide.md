# ptm Tool Guide

Quick reference for the MCP tools available in the Pokemon Team Manager.

## Workflow: Building a Team

1. `list_regulations` — pick a regulation (use "H" for current 2025)
2. `create_team name="My Team" regulation="H"` — creates empty team, returns team_id
3. `search_pokemon query="Garchomp"` — find species names
4. `add_pokemon team_id=1 pokemon_name="Garchomp"` — adds to next open slot
5. `set_ability team_id=1 pokemon_name="Garchomp" ability="Rough Skin"`
6. `set_nature team_id=1 pokemon_name="Garchomp" nature="Jolly"`
7. `set_item team_id=1 pokemon_name="Garchomp" item="Choice Scarf"`
8. `set_moves team_id=1 pokemon_name="Garchomp" move1="Earthquake" move2="Rock Slide" move3="Dragon Claw" move4="Protect"`
9. `set_stats team_id=1 pokemon_name="Garchomp" stat1="attack" stat2="speed"` — 252/252/4 split
10. `set_role team_id=1 pokemon_name="Garchomp" role="physical_attacker"`
11. Repeat steps 3–10 for all 6 slots
12. `validate_team team_id=1` — check for violations
13. `analyse_team team_id=1` — type coverage, speed tiers, archetypes
14. `export_team team_id=1` — markdown team sheet

## VGC Rules (quick ref)

- 6 Pokemon, bring 4 per match
- No duplicate species, no duplicate items
- Final evolutions only
- EVs: max 252 per stat, 508 total
- IVs: 0–31 per stat
- Regulation H (2025): 0 restricted Legendaries allowed

## Valid Stats for set_stats

`hp` `attack` `defense` `sp_attack` `sp_defense` `speed`

## Common Natures

| Goal | Nature |
|---|---|
| Max speed | Timid (SpA) or Jolly (Atk) |
| Max SpA | Modest (Atk-) |
| Max Atk | Adamant (SpA-) |
| Bulky | Bold (Atk-) or Calm (Atk-) |
| Trick Room | Quiet (Spe-) or Brave (Spe-) |

## Search Tips

- `search_pokemon query="fire"` — finds fire-types by name substring
- `search_moves query="spread" category="special"` — find spread moves
- `search_items query="berry"` — find berries
- `search_knowledge query="trick room setters"` — strategy docs
