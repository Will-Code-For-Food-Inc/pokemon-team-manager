# Onboard a Pokemon Champions Team

Walk the user through building a complete ptm team from scratch, one Pokemon at a time. Use the MCP tools to register each mon as it's confirmed.

## Flow

1. **Create the team** — ask for a team name and regulation (default H). Call `create_team`.

2. **For each Pokemon (up to 6)**, run through the questionnaire below. You can accept a list of 6 names upfront and then go mon by mon, or add them one at a time — follow the user's preference.

3. **After all 6**, call `validate_team` and `analyse_team`, then show a summary.

---

## Per-Pokemon Questionnaire

For each mon, ask or infer the following. Where you can make a strong guess based on the species or role, offer it as a default and let the user confirm or override.

**Required:**
- Species name → `add_pokemon`
- Ability (offer legal options from `get_pokemon`) → `set_ability`
- Nature (offer role-appropriate suggestion with reasoning) → `set_nature`
- Item (offer 2–3 options appropriate to the role; note item clause — no duplicates) → `set_item`
- Moves (offer a recommended set of 4; always include Protect unless user declines) → `set_moves`
- Stat focus (Pokemon Champions: 66 points, max 32/stat — ask which 2–3 stats to invest in, offer a recommendation) → `set_stats`
- Role label (offer options: sand_setter, physical_attacker, special_attacker, physical_wall, special_wall, setup_sweeper, speed_control, support, revenge_killer) → `set_role`

**Stat guidance to share with user:**
- 2-stat focus: 32/32 = 64 points (near cap, strong specialization)
- 3-stat focus: 22/22/22 = 66 points (balanced)
- Max 32 per stat — going over is invalid

---

## Adding a Single Pokemon

This skill can also be used just to add one mon to an existing team — skip the create_team step and ask for the team ID. Run the same per-Pokemon questionnaire.

---

## Tips

- Always call `search_pokemon` or `get_pokemon` before `add_pokemon` to confirm the exact species name is in the database.
- After setting moves, note any that failed (not in learnset) and suggest alternatives using `get_moves`.
- If the user says "standard build" or "meta build", search the knowledge base with `search_knowledge` for that species before asking.
- Keep suggestions concise — one recommended option with a short reason, not a wall of text.
