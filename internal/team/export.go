package team

import (
	"fmt"
	"strings"

	"github.com/user/pokemon-team-manager/internal/pokemon"
)

// ExportMarkdown renders a team as a formatted markdown document.
func ExportMarkdown(t *Team) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", t.Name)
	fmt.Fprintf(&b, "**Regulation:** %s  \n", t.Regulation)
	if t.Notes != "" {
		fmt.Fprintf(&b, "**Notes:** %s  \n", t.Notes)
	}
	fmt.Fprintf(&b, "\n---\n\n")

	for _, m := range t.Members {
		if m.Species == nil {
			continue
		}
		name := m.Species.Name
		if m.Nickname != "" {
			name = fmt.Sprintf("%s (%s)", m.Nickname, m.Species.Name)
		}
		item := "No item"
		if m.Item != nil {
			item = m.Item.Name
		}
		ability := "Unknown"
		if m.Ability != nil {
			ability = m.Ability.Name
		}
		types := strings.Join(typesAsStrings(m.Species.Types()), " / ")

		fmt.Fprintf(&b, "## Slot %d — %s\n\n", m.Slot, name)
		fmt.Fprintf(&b, "| | |\n|---|---|\n")
		fmt.Fprintf(&b, "| **Type** | %s |\n", types)
		fmt.Fprintf(&b, "| **Ability** | %s |\n", ability)
		fmt.Fprintf(&b, "| **Item** | %s |\n", item)
		fmt.Fprintf(&b, "| **Nature** | %s |\n", m.Nature)
		if m.TeraType != "" {
			fmt.Fprintf(&b, "| **Tera Type** | %s |\n", m.TeraType)
		}
		if m.Role != "" {
			fmt.Fprintf(&b, "| **Role** | %s |\n", m.Role)
		}
		fmt.Fprintf(&b, "\n")

		// Base stats
		if m.Species != nil {
			fmt.Fprintf(&b, "**Base Stats:** HP %d / Atk %d / Def %d / SpA %d / SpD %d / Spe %d (BST %d)\n\n",
				m.Species.HP, m.Species.Attack, m.Species.Defense,
				m.Species.SpAttack, m.Species.SpDefense, m.Species.Speed, m.Species.BST())
		}

		// EVs / IVs
		fmt.Fprintf(&b, "**EVs:** HP %d / Atk %d / Def %d / SpA %d / SpD %d / Spe %d (%d total)\n\n",
			m.EVs.HP, m.EVs.Atk, m.EVs.Def, m.EVs.SpA, m.EVs.SpD, m.EVs.Spe, m.EVs.Total())


		// Moves
		if len(m.Moves) > 0 {
			fmt.Fprintf(&b, "**Moves:**\n\n")
			for _, mv := range m.Moves {
				if mv == nil {
					continue
				}
				power := "—"
				if mv.Power != nil {
					power = fmt.Sprintf("%d", *mv.Power)
				}
				acc := "—"
				if mv.Accuracy != nil {
					acc = fmt.Sprintf("%d%%", *mv.Accuracy)
				}
				fmt.Fprintf(&b, "- **%s** (%s %s, %s BP, %s acc)\n",
					mv.Name, mv.Type, mv.Category, power, acc)
			}
			fmt.Fprintf(&b, "\n")
		}

		if m.Notes != "" {
			fmt.Fprintf(&b, "> %s\n\n", m.Notes)
		}

		fmt.Fprintf(&b, "---\n\n")
	}
	return b.String()
}

func typesAsStrings(types []pokemon.Type) []string { //nolint:unused
	out := make([]string, len(types))
	for i, t := range types {
		out[i] = string(t)
	}
	return out
}
