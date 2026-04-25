-- Knowledge base tables for semantic search over strategy docs

CREATE TABLE IF NOT EXISTS kb_documents (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    title       TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT '',
    content     TEXT NOT NULL,
    chunk_index INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- FTS5 index for full-text search over documents
CREATE VIRTUAL TABLE IF NOT EXISTS kb_fts USING fts5(
    title,
    content,
    content='kb_documents',
    content_rowid='id'
);

-- Triggers to keep FTS index in sync
CREATE TRIGGER IF NOT EXISTS kb_fts_insert AFTER INSERT ON kb_documents BEGIN
    INSERT INTO kb_fts(rowid, title, content) VALUES (new.id, new.title, new.content);
END;

CREATE TRIGGER IF NOT EXISTS kb_fts_delete AFTER DELETE ON kb_documents BEGIN
    INSERT INTO kb_fts(kb_fts, rowid, title, content) VALUES ('delete', old.id, old.title, old.content);
END;

CREATE TRIGGER IF NOT EXISTS kb_fts_update AFTER UPDATE ON kb_documents BEGIN
    INSERT INTO kb_fts(kb_fts, rowid, title, content) VALUES ('delete', old.id, old.title, old.content);
    INSERT INTO kb_fts(rowid, title, content) VALUES (new.id, new.title, new.content);
END;

-- Vector embeddings stored as BLOBs (float32 array, 384-dim)
-- Cosine similarity computed in-process.
CREATE TABLE IF NOT EXISTS kb_embeddings (
    document_id INTEGER NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
    embedding   BLOB    NOT NULL,
    PRIMARY KEY (document_id)
);
