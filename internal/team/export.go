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
	if t.Strategy != "" {
		fmt.Fprintf(&b, "**Strategy:** %s  \n", t.Strategy)
	}
	fmt.Fprintf(&b, "\n---\n\n")

	for _, m := range t.Members {
		if m.Config == nil || m.Config.Species == nil {
			continue
		}
		c := m.Config
		sp := c.Species

		name := sp.Name
		if c.Nickname != "" {
			name = fmt.Sprintf("%s (%s)", c.Nickname, sp.Name)
		}
		item := "No item"
		if c.Item != nil {
			item = c.Item.Name
		}
		ability := "Unknown"
		if c.Ability != nil {
			ability = c.Ability.Name
		}
		types := strings.Join(typesAsStrings(sp.Types()), " / ")

		fmt.Fprintf(&b, "## Slot %d — %s\n\n", m.Slot, name)
		fmt.Fprintf(&b, "| | |\n|---|---|\n")
		fmt.Fprintf(&b, "| **Type** | %s |\n", types)
		fmt.Fprintf(&b, "| **Ability** | %s |\n", ability)
		fmt.Fprintf(&b, "| **Item** | %s |\n", item)
		fmt.Fprintf(&b, "| **Nature** | %s |\n", c.Nature)
		if c.Role != "" {
			fmt.Fprintf(&b, "| **Role** | %s |\n", c.Role)
		}
		fmt.Fprintf(&b, "\n")

		fmt.Fprintf(&b, "**Base Stats:** HP %d / Atk %d / Def %d / SpA %d / SpD %d / Spe %d (BST %d)\n\n",
			sp.HP, sp.Attack, sp.Defense, sp.SpAttack, sp.SpDefense, sp.Speed, sp.BST())

		fmt.Fprintf(&b, "**EVs:** HP %d / Atk %d / Def %d / SpA %d / SpD %d / Spe %d (%d total)\n\n",
			c.EVs.HP, c.EVs.Atk, c.EVs.Def, c.EVs.SpA, c.EVs.SpD, c.EVs.Spe, c.EVs.Total())

		if len(c.Moves) > 0 {
			fmt.Fprintf(&b, "**Moves:**\n\n")
			for _, mv := range c.Moves {
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

		if c.Notes != "" {
			fmt.Fprintf(&b, "> %s\n\n", c.Notes)
		}

		fmt.Fprintf(&b, "---\n\n")
	}
	return b.String()
}

func typesAsStrings(types []pokemon.Type) []string {
	out := make([]string, len(types))
	for i, t := range types {
		out[i] = string(t)
	}
	return out
}
