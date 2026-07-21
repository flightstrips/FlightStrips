-- AIRAC.NET HTTP cache validators and response bodies are adapter-private
-- checkpoints. They are intentionally separate from canonical AMAN nav cache
-- fragments, manifests and route digests.
CREATE TABLE IF NOT EXISTS airacnet_http_checkpoints (
    request_key TEXT PRIMARY KEY,
    etag TEXT NOT NULL DEFAULT '',
    last_modified TEXT NOT NULL DEFAULT '',
    next_page INTEGER NOT NULL DEFAULT 0 CHECK (next_page >= 0),
    response_body BYTEA NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
