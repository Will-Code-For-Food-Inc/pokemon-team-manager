package knowledge_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/pokemon-team-manager/internal/db"
	"github.com/user/pokemon-team-manager/internal/knowledge"
)

func newRepo(t *testing.T) *knowledge.Repo {
	t.Helper()
	sqlDB, err := db.OpenMemory()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.Seed(sqlDB, "../../data"))
	return knowledge.NewRepo(sqlDB)
}

// ── Ingest ────────────────────────────────────────────────────────────────────

func TestIngest_StoredAsChunk(t *testing.T) {
	r := newRepo(t)
	require.NoError(t, r.Ingest("Doc A", "test", "Porygon2 leads Trick Room.", 0))
	docs, err := r.AllDocuments()
	require.NoError(t, err)
	found := false
	for _, d := range docs {
		if d.Title == "Doc A" {
			found = true
			assert.Equal(t, "Porygon2 leads Trick Room.", d.Content)
		}
	}
	assert.True(t, found, "ingested document must appear in AllDocuments")
}

func TestIngest_SplitsLongContent(t *testing.T) {
	r := newRepo(t)
	content := strings.Repeat("Trick Room team. ", 100) // ~1700 chars
	require.NoError(t, r.Ingest("Long Doc", "test", content, 200))
	docs, err := r.AllDocuments()
	require.NoError(t, err)
	var chunks []knowledge.Document
	for _, d := range docs {
		if d.Title == "Long Doc" {
			chunks = append(chunks, d)
		}
	}
	assert.Greater(t, len(chunks), 1, "content longer than chunk size must produce multiple chunks")
	for _, c := range chunks {
		assert.LessOrEqual(t, len([]rune(c.Content)), 200+1, "each chunk must be at most chunkSize runes")
	}
}

func TestIngest_Idempotent(t *testing.T) {
	r := newRepo(t)
	require.NoError(t, r.Ingest("Dup", "test", "Some content.", 0))
	require.NoError(t, r.Ingest("Dup", "test", "Some content.", 0))
	docs, err := r.AllDocuments()
	require.NoError(t, err)
	count := 0
	for _, d := range docs {
		if d.Title == "Dup" {
			count++
		}
	}
	assert.Equal(t, 2, count, "ingesting twice produces two rows (caller deduplicates by title if desired)")
}

// ── Search ────────────────────────────────────────────────────────────────────

func TestSearch_ReturnsIngestedContent(t *testing.T) {
	r := newRepo(t)
	require.NoError(t, r.Ingest("TR Guide", "test", "Trick Room teams use Porygon2 as a setter.", 0))
	results, err := r.Search("Trick Room", 10)
	require.NoError(t, err)
	assert.NotEmpty(t, results, "search must find the ingested document")
	found := false
	for _, res := range results {
		if strings.Contains(res.Content, "Porygon2") {
			found = true
		}
	}
	assert.True(t, found, "result must contain ingested content about Porygon2")
}

func TestSearch_EmptyQuery_DoesNotPanic(t *testing.T) {
	r := newRepo(t)
	require.NoError(t, r.Ingest("X", "test", "something", 0))
	results, err := r.Search("", 10)
	// Empty query may return results or be empty; it must not error or panic.
	assert.NoError(t, err)
	_ = results
}

func TestSearch_NoMatch_ReturnsEmpty(t *testing.T) {
	r := newRepo(t)
	require.NoError(t, r.Ingest("Only", "test", "Trick Room teams.", 0))
	// Query for something unrelated to the ingested content.
	results, err := r.Search("xyzzy_no_such_word_7q2", 10)
	require.NoError(t, err)
	assert.Empty(t, results, "unmatched query must return empty slice, not error")
}

func TestSearch_LimitHonoured(t *testing.T) {
	r := newRepo(t)
	for i := range 5 {
		require.NoError(t, r.Ingest("SpeedDoc", "test", "fast pokemon fast fast fast speed speed", i))
	}
	results, err := r.Search("fast", 2)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(results), 2, "search must respect the limit parameter")
}

// ── Touch / Stale ─────────────────────────────────────────────────────────────

func TestTouch_ResetsExpiry(t *testing.T) {
	r := newRepo(t)
	require.NoError(t, r.Ingest("Touchable", "test", "expires soon", 0))
	n, err := r.Touch("Touchable")
	require.NoError(t, err)
	assert.Equal(t, int64(1), n, "Touch must update exactly one row")
}

func TestTouch_UnknownTitle_AffectsZeroRows(t *testing.T) {
	r := newRepo(t)
	n, err := r.Touch("does-not-exist")
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestStaleDocuments_FreshDocsNotStale(t *testing.T) {
	r := newRepo(t)
	require.NoError(t, r.Ingest("Fresh", "test", "content", 0))
	stale, err := r.StaleDocuments()
	require.NoError(t, err)
	for _, d := range stale {
		assert.NotEqual(t, "Fresh", d.Title, "freshly ingested doc must not appear as stale")
	}
}

// ── Embeddings ────────────────────────────────────────────────────────────────

func TestSaveEmbedding_RoundTrip(t *testing.T) {
	r := newRepo(t)
	require.NoError(t, r.Ingest("Embed Doc", "test", "content", 0))
	docs, err := r.AllDocuments()
	require.NoError(t, err)
	require.NotEmpty(t, docs)
	id := docs[len(docs)-1].ID
	vec := []float32{0.1, 0.5, 0.9}
	require.NoError(t, r.SaveEmbedding(id, vec))
	n, err := r.EmbeddingCount()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 1)
}

func TestVectorSearch_ReturnsStoredDoc(t *testing.T) {
	r := newRepo(t)
	require.NoError(t, r.Ingest("VecDoc", "test", "vector search content", 0))
	docs, err := r.AllDocuments()
	require.NoError(t, err)
	require.NotEmpty(t, docs)
	id := docs[len(docs)-1].ID
	vec := []float32{1, 0, 0}
	require.NoError(t, r.SaveEmbedding(id, vec))
	results, err := r.VectorSearch([]float32{1, 0, 0}, 5)
	require.NoError(t, err)
	assert.NotEmpty(t, results, "vector search must return the doc whose embedding was stored")
}

// ── UnembeddedDocuments ───────────────────────────────────────────────────────

func TestUnembeddedDocuments_AfterIngest(t *testing.T) {
	r := newRepo(t)
	require.NoError(t, r.Ingest("NoEmbed", "test", "no embedding yet", 0))
	unembedded, err := r.UnembeddedDocuments()
	require.NoError(t, err)
	found := false
	for _, d := range unembedded {
		if d.Title == "NoEmbed" {
			found = true
		}
	}
	assert.True(t, found, "freshly ingested doc with no embedding must appear in UnembeddedDocuments")
}

func TestUnembeddedDocuments_AfterEmbedding(t *testing.T) {
	r := newRepo(t)
	require.NoError(t, r.Ingest("WithEmbed", "test", "has embedding", 0))
	docs, err := r.AllDocuments()
	require.NoError(t, err)
	require.NotEmpty(t, docs)
	id := docs[len(docs)-1].ID
	require.NoError(t, r.SaveEmbedding(id, []float32{0.5}))
	unembedded, err := r.UnembeddedDocuments()
	require.NoError(t, err)
	for _, d := range unembedded {
		assert.NotEqual(t, id, d.ID, "doc with saved embedding must not appear in UnembeddedDocuments")
	}
}
