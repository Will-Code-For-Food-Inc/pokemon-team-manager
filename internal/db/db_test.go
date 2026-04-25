package db_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/pokemon-team-manager/internal/db"
)

func TestOpenMemory(t *testing.T) {
	sqlDB, err := db.OpenMemory()
	require.NoError(t, err)
	defer sqlDB.Close()

	// Verify migrations ran: schema_migrations table must exist.
	var count int
	err = sqlDB.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, 3) // 3 migration files

	// Verify core tables exist.
	for _, table := range []string{"species", "moves", "abilities", "items", "teams", "team_members", "kb_documents"} {
		var n int
		err := sqlDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n)
		assert.NoError(t, err, "checking table %s", table)
		assert.Equal(t, 1, n, "table %s should exist", table)
	}
}
