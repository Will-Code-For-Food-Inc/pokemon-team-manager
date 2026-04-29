package handlers

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/pokemon-team-manager/internal/pokemon"
)

// fakeRows builds n trivially-distinct rows sharing the schema {id,name}.
// Used to exercise toonMenu pagination math without depending on entity
// projection helpers.
func fakeRows(n int) []map[string]any {
	out := make([]map[string]any, 0, n)
	for i := range n {
		out = append(out, map[string]any{"id": i, "name": "row" + intToStr(i)})
	}
	return out
}

func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	var b strings.Builder
	if i < 0 {
		b.WriteByte('-')
		i = -i
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	b.Write(digits)
	return b.String()
}

func TestToonMenu_EmptyTotal(t *testing.T) {
	got := toonMenu("things", nil, 0, 0, 0, "")
	assert.Empty(t, got, "empty input must return empty string, not a header with no rows")
}

func TestToonMenu_SinglePageSuppressesNextHint(t *testing.T) {
	out := toonMenu("things", fakeRows(3), 3, 0, 0, "call get_thing for details.")

	require.Contains(t, out, "things[3]{id,name}:", "TOON header must include length marker and projected schema")
	assert.Contains(t, out, "Showing 1–3 of 3.")
	assert.NotContains(t, out, "offset=", "single page must not advertise a next-page offset")
	assert.Contains(t, out, "call get_thing for details.")
}

func TestToonMenu_DefaultPageIsTen(t *testing.T) {
	rows := fakeRows(25)
	out := toonMenu("things", rows, 25, 0, 0, "follow up")

	// Header should reflect the projected schema regardless of rows shown,
	// and the pagination tail should advertise offset=10.
	require.Contains(t, out, "things[10]{id,name}:")
	assert.Contains(t, out, "Showing 1–10 of 25.")
	assert.Contains(t, out, "offset=10")

	// Spot-check that the page contains the first row but not the eleventh —
	// this is the pagination's reason for existing.
	assert.Contains(t, out, "0,row0")
	assert.NotContains(t, out, "10,row10")
}

func TestToonMenu_LimitOverride(t *testing.T) {
	out := toonMenu("things", fakeRows(20), 20, 0, 5, "follow up")
	require.Contains(t, out, "things[5]{id,name}:")
	assert.Contains(t, out, "Showing 1–5 of 20.")
	assert.Contains(t, out, "offset=5")
}

func TestToonMenu_MidPage(t *testing.T) {
	out := toonMenu("things", fakeRows(25), 25, 10, 0, "follow up")
	require.Contains(t, out, "things[10]{id,name}:")
	assert.Contains(t, out, "Showing 11–20 of 25.")
	assert.Contains(t, out, "offset=20")
	// Mid-page must not include row 0 or row 24.
	assert.NotContains(t, out, "0,row0")
	assert.Contains(t, out, "10,row10")
}

func TestToonMenu_LastPage(t *testing.T) {
	out := toonMenu("things", fakeRows(25), 25, 20, 0, "follow up")
	require.Contains(t, out, "things[5]{id,name}:")
	assert.Contains(t, out, "Showing 21–25 of 25.")
	// On the last page we must NOT advertise a next page; the model has no
	// further offset to call. The follow-up hint still appears.
	assert.NotContains(t, out, "offset=25")
	assert.NotContains(t, out, "for the next page")
	assert.Contains(t, out, "follow up")
}

func TestToonMenu_OffsetPastEndIsActionable(t *testing.T) {
	out := toonMenu("things", fakeRows(5), 5, 99, 0, "follow up")
	// Past-end is a model-recoverable error, not a panic. Tell the model the
	// offset is bad and what the total is so it can self-correct.
	assert.Contains(t, out, "offset 99 is past the end")
	assert.Contains(t, out, "5 total")
	assert.Contains(t, out, "offset=0")
}

func TestToonMenu_NegativeOffsetClampsToZero(t *testing.T) {
	out := toonMenu("things", fakeRows(3), 3, -10, 0, "")
	assert.Contains(t, out, "Showing 1–3 of 3.")
}

func TestToonMenu_NoFollowUp(t *testing.T) {
	out := toonMenu("things", fakeRows(3), 3, 0, 0, "")
	assert.Contains(t, out, "Showing 1–3 of 3.")
	// No follow-up means no trailing " or ..." clause.
	assert.NotContains(t, out, " or ")
}

// ── projection helpers ────────────────────────────────────────────────────────

func TestSpeciesRows_DualType(t *testing.T) {
	hp, atk, def := 108, 130, 95
	spa, spd, spe := 80, 85, 102
	sp := pokemon.Species{
		Name: "Garchomp", DexID: 445,
		Type1: "dragon", Type2: "ground",
		HP: hp, Attack: atk, Defense: def,
		SpAttack: spa, SpDefense: spd, Speed: spe,
		Owned: true,
	}
	rows := speciesRows([]pokemon.Species{sp})
	require.Len(t, rows, 1)
	r := rows[0]
	assert.Equal(t, "Garchomp", r["name"])
	assert.Equal(t, 445, r["dex"])
	assert.Equal(t, "dragon/ground", r["type"], "dual-type must be concatenated with /")
	assert.Equal(t, hp+atk+def+spa+spd+spe, r["bst"])
	assert.Equal(t, true, r["owned"])
}

func TestSpeciesRows_MonoType(t *testing.T) {
	rows := speciesRows([]pokemon.Species{
		{Name: "Snorlax", DexID: 143, Type1: "normal", HP: 160, Attack: 110},
	})
	require.Len(t, rows, 1)
	assert.Equal(t, "normal", rows[0]["type"], "single-type must NOT have a trailing /")
}

func TestItemRows_TruncatesDescription(t *testing.T) {
	long := strings.Repeat("a", 200)
	rows := itemRows([]pokemon.Item{{
		Name: "Leftovers", VPCost: 700,
		Description: long, IsBanned: false, Owned: true,
	}}, 50)
	require.Len(t, rows, 1)
	desc, _ := rows[0]["desc"].(string)
	// descMax is in runes (display characters), not bytes — the ellipsis is
	// 3 bytes but counts as one character to the model and the user.
	assert.Equal(t, 50, utf8.RuneCountInString(desc), "description must be clipped to descMax runes including the ellipsis")
	assert.True(t, strings.HasSuffix(desc, "…"), "truncation must end in an ellipsis")
}

func TestItemRows_NoTruncationWhenDescMaxZero(t *testing.T) {
	long := strings.Repeat("a", 200)
	rows := itemRows([]pokemon.Item{{Name: "X", Description: long}}, 0)
	require.Len(t, rows, 1)
	assert.Equal(t, long, rows[0]["desc"], "descMax=0 must disable truncation entirely")
}

func TestItemRows_FlagsPropagate(t *testing.T) {
	rows := itemRows([]pokemon.Item{
		{Name: "BannedItem", IsBanned: true, Owned: false, VPCost: 1000},
		{Name: "OwnedItem", IsBanned: false, Owned: true, VPCost: 200},
	}, 0)
	require.Len(t, rows, 2)
	assert.Equal(t, true, rows[0]["banned"])
	assert.Equal(t, false, rows[0]["owned"])
	assert.Equal(t, 1000, rows[0]["vp"])
	assert.Equal(t, false, rows[1]["banned"])
	assert.Equal(t, true, rows[1]["owned"])
}

func TestMoveRows_NilPowerAccuracyBecomeZero(t *testing.T) {
	// Status moves have nil Power; some have nil Accuracy. The TOON tabular
	// form requires consistent value shapes per column, so we coerce nil to 0.
	rows := moveRows([]pokemon.Move{
		{Name: "Trick Room", Type: "psychic", Category: "status", Power: nil, Accuracy: nil, PP: 5, Priority: -7},
	}, 0)
	require.Len(t, rows, 1)
	assert.Equal(t, 0, rows[0]["bp"], "nil Power must coerce to 0 to keep the column homogeneous")
	assert.Equal(t, 0, rows[0]["acc"], "nil Accuracy must coerce to 0 to keep the column homogeneous")
	assert.Equal(t, -7, rows[0]["prio"])
	assert.Equal(t, "status", rows[0]["cat"])
}

func TestMoveRows_PopulatedPower(t *testing.T) {
	power := 90
	acc := 100
	rows := moveRows([]pokemon.Move{
		{Name: "Psychic", Type: "psychic", Category: "special", Power: &power, Accuracy: &acc, PP: 12},
	}, 0)
	require.Len(t, rows, 1)
	assert.Equal(t, 90, rows[0]["bp"])
	assert.Equal(t, 100, rows[0]["acc"])
}

// ── end-to-end: projection + menu produces parseable TOON ─────────────────────

// TestToonMenu_PokemonShape locks in the wire format so a future gotoon upgrade
// or projection refactor doesn't silently change what the LLM sees.
func TestToonMenu_PokemonShape(t *testing.T) {
	species := []pokemon.Species{
		{Name: "Garchomp", DexID: 445, Type1: "dragon", Type2: "ground", HP: 108, Attack: 130, Defense: 95, SpAttack: 80, SpDefense: 85, Speed: 102, Owned: false},
		{Name: "Excadrill", DexID: 530, Type1: "ground", Type2: "steel", HP: 110, Attack: 135, Defense: 60, SpAttack: 50, SpDefense: 65, Speed: 88, Owned: true},
	}
	out := toonMenu("pokemon", speciesRows(species), 2, 0, 0, "call get_pokemon name='X' for full stats.")

	// Header must declare all five projected columns. gotoon currently
	// alphabetizes column names; assert the set, not the order, so a future
	// upgrade that preserves insertion order doesn't break this test.
	require.Contains(t, out, "pokemon[2]{")
	for _, col := range []string{"name", "dex", "type", "bst", "owned"} {
		assert.Contains(t, out, col, "header must include projected column %q", col)
	}
	// Both rows present with the type column concatenated.
	assert.Contains(t, out, "Garchomp")
	assert.Contains(t, out, "dragon/ground")
	assert.Contains(t, out, "Excadrill")
	assert.Contains(t, out, "ground/steel")
	// Tail line + follow-up hint.
	assert.Contains(t, out, "Showing 1–2 of 2.")
	assert.Contains(t, out, "call get_pokemon name='X' for full stats.")
}
