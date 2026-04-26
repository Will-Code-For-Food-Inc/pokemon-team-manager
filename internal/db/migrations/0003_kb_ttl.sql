-- Add TTL tracking to knowledge base documents.
-- expires_at: when the doc is flagged for review (6 months from ingestion).
-- reviewed_at: last time the doc was reviewed/TTL reset.
ALTER TABLE kb_documents ADD COLUMN expires_at  DATETIME;
ALTER TABLE kb_documents ADD COLUMN reviewed_at DATETIME;

-- Backfill: set all existing rows to expire 6 months from now.
UPDATE kb_documents SET expires_at = datetime('now', '+6 months');
