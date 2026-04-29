-- Chat archive: instead of deleting chat history we move it here.
-- Each archive batch shares an archive_id (UnixMicro of the archive op) so
-- the original conversation flow can be reconstructed later for analysis,
-- training data, or undo.

CREATE TABLE IF NOT EXISTS chat_messages_archive (
    archive_id   INTEGER NOT NULL,
    archived_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    id           INTEGER NOT NULL,
    role         TEXT    NOT NULL,
    content      TEXT    NOT NULL,
    in_context   INTEGER NOT NULL DEFAULT 0,
    embedding    BLOB,
    created_at   TEXT    NOT NULL,
    PRIMARY KEY (archive_id, id)
);

CREATE INDEX IF NOT EXISTS idx_chat_messages_archive_archived_at
    ON chat_messages_archive(archived_at);

CREATE TABLE IF NOT EXISTS chat_context_archive (
    archive_id  INTEGER PRIMARY KEY,
    archived_at TEXT    NOT NULL DEFAULT (datetime('now')),
    messages    TEXT    NOT NULL
);
