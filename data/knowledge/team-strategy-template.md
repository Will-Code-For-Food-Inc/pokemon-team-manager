# Team Strategy Notes Template

This is the canonical structure for team strategy notes in ptm. When writing
or updating notes via `set_team_notes`, follow this template and fill each
section with concrete values for the specific team. Keep each section short
(1-3 lines max). The full document should fit in ~600 words — strategy notes
are operational reminders for the player, not exhaustive analysis.

If a section doesn't apply, write `n/a` and one sentence explaining why,
rather than deleting the heading. Consistent structure across teams makes
notes easier to compare.

---

## Win Condition

One sentence on how this team wins. Be specific about the mechanism, not just
the outcome.

> Examples:
> - "Snowball games with Trick Room + max-Atk Iron Fist Hitmonchan double-targeting."
> - "Stall the weather war, then crash through with Choice Scarf Noivern under Tailwind."
> - "Suspend Speed control with Whimsicott Tailwind, sweep with Mega Garchomp."

## Standard Lead

The two Pokemon brought to slots 1+2 in 80% of games, and *why* that pair.

> Example: "Whimsicott + Incineroar — Tailwind setup off Prankster while Intimidate
> defangs Garchomp leads. Whimsicott Encore locks setup attempts."

## Alt Leads (matchup-specific)

When the standard lead is bad. Format: `vs <archetype> → <pair> — <reason>`

> Examples:
> - vs Sun (Char Y / Venusaur) → Pelipper + Goodra — flip the weather, eat Heat Waves
> - vs TR (Hatterene / Ursaluna-BM) → Whimsicott + Gholdengo — Taunt the setter, threaten KO turn 1
> - vs hyper-offense Mega Salamence → Aggron + Sylveon — Sturdy + Pixilate Hyper Voice

## Bring Decisions

Which 4 of 6 to bring vs each major archetype. Include leave-behinds explicitly
so the rationale is clear.

| Opposing archetype | Bring | Leave | Reason |
|---|---|---|---|
| Sun | Pelipper, Noivern, Corviknight, Ceruledge | Goodra, Alakazam | Flip weather, hit Sun mons hard |
| Rain | Goodra, Alakazam, Ceruledge, Incineroar | Pelipper, Corviknight | n/a |
| TR | ... | ... | ... |
| Balance | ... | ... | ... |

## Speed Benchmarks

Critical thresholds. Format: `<base/boost> = <stat> — <key threats> [out|under]`.

Cross-reference the speed-tier sheet in `champions-meta-2026-04.md` rather than
restating it. Just call out the team-specific decisions:

> Examples:
> - Noivern (123 base, +Scarf, +Tailwind) = 348 — out-speeds every non-Booster threat under Tailwind.
> - Ceruledge (85 base, +Jolly, no item) = 154 — under everything fast; intentionally a Trick Room target.
> - Aggron (50 base, -Spe nature) = 76 — TR target by design.

## Stat Allocation Rationale (66 SP per Pokemon, max 32/stat)

For each member, document the SP spread AND why. Format:
`<Mon>: <hp>/<atk>/<def>/<spa>/<spd>/<spe> — <one-line rationale>`

> Examples:
> - Noivern: 16/0/0/32/0/18 — max SpA for Boomburst; +18 Spe hits 348 under Tailwind+Scarf (out-paces Mega Salamence); 16 HP for residual chip survival.
> - Ceruledge: 16/32/0/0/0/18 — max Atk for Bitter Blade snowball; 18 Spe creeps into Tailwind range; 16 HP hits Sitrus Berry break threshold.
> - Corviknight: 32/0/32/0/0/0 — pure phys wall (HP + Def maxed); Roost compresses defensive calcs.
> - Aggron: 32/0/0/0/32/2 — Sturdy guarantee + max SpD; 2 Spe creeps mirror match.

If `total < 66` on any member, the player left points on the table — flag it
in the notes ("Goodra has 4 unused SP — TODO").

### Stat priority quick-reference by role

| Role | Primary | Secondary | Tertiary |
|---|---|---|---|
| physical attacker | Atk 32 | Spe 16-32 | HP 16 |
| special attacker | SpA 32 | Spe 16-32 | HP 16 |
| mixed attacker | Atk/SpA 16-24 each | Spe 16-32 | HP rest |
| support | HP 32 | Spe 0-16 | Def or SpD rest |
| physical wall | HP 32 | Def 32 | SpD 0-2 |
| special wall | HP 32 | SpD 32 | Def 0-2 |
| mixed wall | HP 32 | Def 16 | SpD 16-18 |
| Trick Room sweeper | Atk or SpA 32 | HP 16-32 | Spe 0 (or invest in -Spe nature) |
| pivot (Fake Out / Parting Shot) | HP 32 | Def or SpD 16-32 | Spe 0-16 |

### Damage / bulk benchmarks worth recording

When the player has a target damage calc, write it down. The pattern is
`<defending mon> at <SP> survives <attacker's move> from <attacker config>`.

> Examples:
> - Goodra (32 HP / 0 Def, no item) survives Bullet Punch from +0 Mega Scizor (Adamant 32 Atk) at full HP — confirms Goodra can pivot in safely.
> - Corviknight (32 HP / 32 Def + Leftovers) survives Wood Hammer from Choice Band Rillaboom in Grassy Terrain after a 6%-chip turn.

Don't fabricate calcs. Only record ones the player verified in the damage
calculator or saw in actual matches.

## Threats and Counters

Pokemon or sets that meaningfully threaten this team, and the planned answer.

| Threat | Why scary | Counter / play |
|---|---|---|
| Choice Scarf Gholdengo | Outspeeds Noivern | Lead Incineroar (Intimidate + Knock Off threat); Goodra eats Make It Rain |
| Mega Lopunny | Fake Out into setup | Aggron Sturdy + Heavy Slam |
| Sun + Mega Charizard Y | Solar Beam OHKOs Goodra | Pelipper turn 1, Roost-tank with Corviknight |

Aim for 4-7 entries. The team can't have an answer to everything — call out the
genuine "lose if this happens" matchups too.

## Mega Plan

Pokemon Champions has Mega Evolutions (no Tera). The held Mega Stone IS the
plan — whichever member holds the stone is your designated mega. Note here:
the trigger turn (early for offense, late for clutch), and what the
mega-evolution gains (typing change, ability swap, stat shift).

> Example: "Charizard holds Charizardite Y — mega turn 1 to set sun via
> Drought before opponent can flip weather. Loses Solar Power for Drought,
> gains permanent sun until KO."

## Player Notes

Things YOU tend to do wrong with this team. Concrete operational reminders.

> Examples:
> - Don't tera turn 1 just because the lead matchup is bad.
> - Boomburst Noivern hits your own Ceruledge — protect Ceruledge or position carefully.
> - Don't double-target into Protect on Whimsicott — Prankster makes it 100% safe for them.

## Open Questions / TODO

Stuff the player wants to test or revisit. Not gameplay strategy per se — more
like a personal followup list.

> Examples:
> - Test Brave Bird vs Drill Peck on Corviknight — recoil might matter more than expected.
> - Consider Sitrus Berry on Ceruledge instead of Black Belt — survivability over damage.
