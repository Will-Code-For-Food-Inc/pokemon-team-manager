package handlers

import (
	"fmt"
	"strings"

	"github.com/alpkeskin/gotoon"

	"github.com/user/pokemon-team-manager/internal/pokemon"
)

// toonMenuDefaultPage is the page size used by toonMenu when the caller does
// not supply a limit override.
const toonMenuDefaultPage = 10

// toonRows emits a TOON tabular block under a single label, without pagination
// or follow-up tail. Use this when the surrounding output is a narrative
// header (e.g. get_team renders "Team 4: ..." then a members[N]{...} block);
// for standalone list tools, use toonMenu instead.
func toonRows(label string, rows []map[string]any) (string, error) {
	if len(rows) == 0 {
		return "", nil
	}
	return gotoon.Encode(map[string]any{label: rows})
}

// toonMenu renders a paginated TOON tabular menu of choices for the model to
// pick from. Each row is a small projection of fields — just enough for the
// model to scan and decide which option to pull full details on. The followUp
// string tells the model exactly which tool to call next; the pagination tail
// tells it how to ask for more.
//
// label is the top-level key in the TOON output (e.g. "pokemon"). rows must
// share the same key set so gotoon emits tabular form. limitOverride > 0
// overrides the default page size.
func toonMenu(label string, rows []map[string]any, total, offset, limitOverride int, followUp string) string {
	size := toonMenuDefaultPage
	if limitOverride > 0 {
		size = limitOverride
	}
	if offset < 0 {
		offset = 0
	}
	if total == 0 {
		return ""
	}
	if offset >= total {
		return fmt.Sprintf("offset %d is past the end (%d total). Try offset=0 or refine the query.", offset, total)
	}
	hi := offset + size
	if hi > total {
		hi = total
	}
	page := rows[offset:hi]
	encoded, err := gotoon.Encode(map[string]any{label: page})
	if err != nil {
		return "error: TOON encode: " + err.Error()
	}
	var b strings.Builder
	b.WriteString(encoded)
	if !strings.HasSuffix(encoded, "\n") {
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "\nShowing %d–%d of %d.", offset+1, hi, total)
	if hi < total {
		fmt.Fprintf(&b, " Call again with offset=%d for the next page,", hi)
	}
	if followUp != "" {
		b.WriteString(" or ")
		b.WriteString(followUp)
	}
	b.WriteByte('\n')
	return b.String()
}

// speciesRows projects a Species slice into TOON-tabular row form. The column
// set is intentionally small; full details come from a follow-up get_pokemon
// or evaluate_pokemon call.
func speciesRows(results []pokemon.Species) []map[string]any {
	out := make([]map[string]any, 0, len(results))
	for _, s := range results {
		typ := string(s.Type1)
		if s.Type2 != "" {
			typ += "/" + string(s.Type2)
		}
		out = append(out, map[string]any{
			"name":  s.Name,
			"dex":   s.DexID,
			"type":  typ,
			"bst":   s.BST(),
			"owned": s.Owned,
		})
	}
	return out
}

// itemRows projects a held-item slice into TOON-tabular row form. descMax
// truncates each description (0 = no truncation).
func itemRows(items []pokemon.Item, descMax int) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		desc := it.Description
		if descMax > 0 && len(desc) > descMax {
			desc = desc[:descMax-1] + "…"
		}
		out = append(out, map[string]any{
			"name":   it.Name,
			"vp":     it.VPCost,
			"banned": it.IsBanned,
			"owned":  it.Owned,
			"desc":   desc,
		})
	}
	return out
}

// moveRows projects a move slice into TOON-tabular row form. nil Power and
// Accuracy are emitted as 0 so the column type stays uniform (TOON tabular
// requires consistent value shapes per column).
func moveRows(moves []pokemon.Move, descMax int) []map[string]any {
	out := make([]map[string]any, 0, len(moves))
	for _, mv := range moves {
		bp := 0
		if mv.Power != nil {
			bp = *mv.Power
		}
		acc := 0
		if mv.Accuracy != nil {
			acc = *mv.Accuracy
		}
		desc := mv.Description
		if descMax > 0 && len(desc) > descMax {
			desc = desc[:descMax-1] + "…"
		}
		out = append(out, map[string]any{
			"name": mv.Name,
			"type": string(mv.Type),
			"cat":  string(mv.Category),
			"bp":   bp,
			"acc":  acc,
			"pp":   mv.PP,
			"prio": mv.Priority,
			"desc": desc,
		})
	}
	return out
}
