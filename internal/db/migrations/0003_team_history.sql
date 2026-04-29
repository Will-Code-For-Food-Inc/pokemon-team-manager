-- Team history: every successful repo mutation writes a snapshot of the team
-- as a JSON blob. Snapshots are schema-resilient (the JSON is whatever the
-- team looked like at that moment) and serve as the substrate for both
-- "view a past version" UI and a pairwise-diff view.
--
-- kind = 'mutation' — fine-grained, auto-pruned to last N per team.
-- kind = 'checkpoint' — coarser, user/agent-marked, never auto-pruned.
--
-- Pruning is intentionally user-only — no agent-callable tool drops rows from
-- this table. Same boundary as team deletion.

CREATE TABLE IF NOT EXISTS team_snapshots (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    team_id    INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    kind       TEXT    NOT NULL CHECK (kind IN ('mutation', 'checkpoint')),
    label      TEXT    NOT NULL DEFAULT '',
    payload    TEXT    NOT NULL,
    created_at TEXT    NOT NULL DEFAULT (datetime('now')),
    created_by TEXT    NOT NULL DEFAULT 'system'
);

CREATE INDEX IF NOT EXISTS idx_team_snapshots_team_created
    ON team_snapshots(team_id, created_at DESC);
