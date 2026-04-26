# Pokemon Forms and Regional Variants

## How Forms Are Stored

Every species row has two fields: `name` and `form`. The base/default variant always has `form = ""` (empty string). Regional variants and alternate forms carry a non-empty `form` value.

Examples in this database:

| name      | form             | type          |
|-----------|------------------|---------------|
| Ninetales | *(base)*         | fire          |
| Ninetales | Alolan           | ice/fairy     |
| Tauros    | *(base)*         | normal        |
| Tauros    | Paldean-Combat   | fighting      |
| Tauros    | Paldean-Blaze    | fighting/fire |
| Tauros    | Paldean-Aqua     | fighting/water|

## How to Find a Specific Form

Use `search_pokemon` with the species name to see all forms at once:

```
search_pokemon query="Ninetales"   → returns base + Alolan
search_pokemon query="Tauros"      → returns base + all Paldean forms
```

The result includes `form` in every row. To add a specific form to a team, pass the exact species name — the system uses the database `name` field, and all forms share the same `name`. The agent must then use `get_pokemon` to pick the right row by form.

## Adding a Regional Form to a Team

`add_pokemon` matches by species name. If multiple forms exist, the **base form** is matched by default unless the user specifies a form name (e.g. "Alolan Ninetales", "Paldean-Combat Tauros"). Translate these to a `search_pokemon` call first, find the correct `id`, and pass the exact name from the result.

## Abilities Per Form

Regional variants have their own ability pools — they do not inherit the base form's abilities:

- **Ninetales (base)**: Flash Fire / Drought (hidden)
- **Ninetales Alolan**: Snow Cloak / Snow Warning (hidden)
- **Tauros (base)**: Intimidate / Anger Point / Sheer Force (hidden)
- **Tauros Paldean (all three)**: Intimidate / Inner Focus / Cud Chew (hidden)

Use `get_pokemon pokemon_name="Ninetales"` and inspect the `abilities` field to see what is available for each form before setting one.

## Form Naming Conventions

Forms use title-case with hyphens for multi-word variants: `Alolan`, `Paldean-Combat`, `Paldean-Blaze`, `Paldean-Aqua`, `Hisuian`. When a user says "Alolan Ninetales" or "Paldean Tauros", search for the base name and filter by form field.
