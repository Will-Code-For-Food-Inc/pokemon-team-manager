package web

import (
	"database/sql"

	"github.com/user/pokemon-team-manager/internal/handlers"
)

// agentConfig is an alias for handlers.AgentConfig.
type agentConfig = handlers.AgentConfig

func loadConfig(db *sql.DB) agentConfig  { return handlers.LoadConfig(db) }
func saveConfig(db *sql.DB, cfg agentConfig) error { return handlers.SaveConfig(db, cfg) }
