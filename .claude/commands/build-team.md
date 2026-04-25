# Build a Pokemon Champions Team

Help the user design a VGC team from scratch based on a core, archetype, or playstyle. This skill is about *deciding what to run* — use `/onboard-team` once the roster is chosen.

## Flow

1. **Establish the brief** — ask one question: "What's your starting point?" Options to offer:
   - A core (1–2 Pokemon you want to build around)
   - An archetype (sand, rain, trick room, hyper offense, balance)
   - A playstyle ("bulky and hard to kill", "fast and aggressive", "setup and sweep")
   - A problem to solve ("I keep losing to Dragon types / fast attackers / status")

2. **Research before suggesting** — use `search_knowledge` to pull relevant strategy docs for the archetype or core. Use `get_pokemon` to verify base stats and typing before recommending a mon.

3. **Build the core first** — propose 2–3 Pokemon that anchor the team with a short reason for each. Get user buy-in before filling out the rest.

4. **Fill the remaining slots** addressing:
   - Speed control (fast lead, trick room setter, or tailwind?)
   - Offensive coverage (what types are you missing?)
   - Defensive answers (what threatens your core?)
   - Utility (Intimidate, redirection, weather control)

5. **Present the full 6** as a roster with a one-line role for each. Ask for approval or changes.

6. **Hand off to `/onboard-team`** — once the user confirms the roster, tell them to run `/onboard-team` to register it in ptm.

---

## Guidelines

- Regulation H (current): no restricted Legendaries. Keep suggestions legal.
- Pokemon Champions item availability may differ from mainline — if the user flags an item isn't in the game, note it and suggest an alternative.
- Prioritize synergy explanations over stat dumps — "Sylveon covers your Dragon weakness and Hyper Voice softens both targets" beats a wall of numbers.
- If the user already has some mons decided, work around them — don't re-suggest what they've already committed to.
- Keep each suggestion to 2–3 sentences max. This is a conversation, not a report.
