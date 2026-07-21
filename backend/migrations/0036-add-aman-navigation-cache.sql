-- Canonical AMAN navigation cache. These tables contain canonical navigation
-- fragments and no provider transport state.
CREATE TABLE IF NOT EXISTS aman_nav_airport_fragments (
    digest VARCHAR PRIMARY KEY,
    schema_version VARCHAR NOT NULL,
    cycle VARCHAR NOT NULL,
    source_revision VARCHAR NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_until TIMESTAMPTZ NOT NULL,
    airport VARCHAR NOT NULL,
    provenance JSONB NOT NULL,
    validation_state VARCHAR NOT NULL,
    imported_at TIMESTAMPTZ NOT NULL,
    validated_at TIMESTAMPTZ,
    payload JSONB NOT NULL,
    CHECK (validation_state IN ('candidate', 'validated'))
);

CREATE TABLE IF NOT EXISTS aman_nav_procedure_fragments (
    digest VARCHAR PRIMARY KEY,
    schema_version VARCHAR NOT NULL,
    cycle VARCHAR NOT NULL,
    source_revision VARCHAR NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_until TIMESTAMPTZ NOT NULL,
    airport VARCHAR NOT NULL,
    procedure_kind VARCHAR NOT NULL,
    provenance JSONB NOT NULL,
    validation_state VARCHAR NOT NULL,
    imported_at TIMESTAMPTZ NOT NULL,
    validated_at TIMESTAMPTZ,
    payload JSONB NOT NULL,
    CHECK (validation_state IN ('candidate', 'validated'))
);

CREATE TABLE IF NOT EXISTS aman_nav_fix_fragments (
    digest VARCHAR PRIMARY KEY,
    schema_version VARCHAR NOT NULL,
    cycle VARCHAR NOT NULL,
    source_revision VARCHAR NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_until TIMESTAMPTZ NOT NULL,
    provenance JSONB NOT NULL,
    validation_state VARCHAR NOT NULL,
    imported_at TIMESTAMPTZ NOT NULL,
    validated_at TIMESTAMPTZ,
    payload JSONB NOT NULL,
    CHECK (validation_state IN ('candidate', 'validated'))
);

CREATE TABLE IF NOT EXISTS aman_nav_terminal_fragments (
    digest VARCHAR PRIMARY KEY,
    schema_version VARCHAR NOT NULL,
    cycle VARCHAR NOT NULL,
    source_revision VARCHAR NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_until TIMESTAMPTZ NOT NULL,
    airport VARCHAR NOT NULL,
    config_version VARCHAR NOT NULL,
    provenance JSONB NOT NULL,
    validation_state VARCHAR NOT NULL,
    imported_at TIMESTAMPTZ NOT NULL,
    validated_at TIMESTAMPTZ,
    payload JSONB NOT NULL,
    CHECK (validation_state IN ('candidate', 'validated'))
);

CREATE TABLE IF NOT EXISTS aman_nav_manifests (
    manifest_id BIGSERIAL PRIMARY KEY,
    airport VARCHAR NOT NULL,
    revision BIGINT NOT NULL,
    cycle VARCHAR NOT NULL,
    source_revision VARCHAR NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_until TIMESTAMPTZ NOT NULL,
    airport_digest VARCHAR NOT NULL REFERENCES aman_nav_airport_fragments(digest) ON DELETE RESTRICT,
    procedure_digests JSONB NOT NULL,
    fix_digest VARCHAR NOT NULL REFERENCES aman_nav_fix_fragments(digest) ON DELETE RESTRICT,
    terminal_digest VARCHAR REFERENCES aman_nav_terminal_fragments(digest) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (airport, revision)
);

CREATE TABLE IF NOT EXISTS aman_nav_active_manifests (
    airport VARCHAR PRIMARY KEY,
    manifest_id BIGINT NOT NULL REFERENCES aman_nav_manifests(manifest_id) ON DELETE RESTRICT,
    revision BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS aman_nav_route_cache (
    cache_key VARCHAR PRIMARY KEY,
    semantic_key VARCHAR NOT NULL,
    resolver_version VARCHAR NOT NULL,
    schema_version VARCHAR NOT NULL,
    route_digest VARCHAR NOT NULL,
    provenance JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    query JSONB NOT NULL,
    payload JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_aman_nav_manifests_airport_revision ON aman_nav_manifests (airport, revision DESC);
CREATE INDEX IF NOT EXISTS idx_aman_nav_route_cache_semantic_key ON aman_nav_route_cache (semantic_key);
