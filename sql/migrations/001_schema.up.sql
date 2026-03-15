-- Enable the trigram extension required for strict_word_similarity() and ~* queries.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Plain CREATE TYPE so sqlc can infer the enum values for Go code generation.
-- golang-migrate tracks executed versions, so idempotency via DO $$ is not needed.
CREATE TYPE item_type_enum AS ENUM (
    'map',
    'gameskin',
    'hud',
    'skin',
    'entity',
    'theme',
    'template',
    'emoticon'
);

-- One row per searchable item.
-- item_id is the sole PK so that search_value can hold a simple FK reference to it.
CREATE TABLE search_item (
    item_id             UUID           PRIMARY KEY,
    item_type           item_type_enum NOT NULL,
    size                BIGINT         NOT NULL DEFAULT 0,
    checksum            VARCHAR(128)   NOT NULL,
    item_file_path      TEXT           NOT NULL
        CHECK (
            item_file_path <> ''
            AND item_file_path NOT LIKE '%..%'
            AND item_file_path LIKE '/%'
        ),
    item_thumbnail_path TEXT
        CHECK (
            item_thumbnail_path IS NULL
            OR (
                item_thumbnail_path <> ''
                AND item_thumbnail_path NOT LIKE '%..%'
                AND item_thumbnail_path LIKE '/%'
            )
        ),
    item_value          JSONB          NOT NULL,
    CONSTRAINT search_item_type_checksum_key UNIQUE (item_type, checksum)
);

CREATE INDEX search_item_id_item_type_idx ON search_item (item_id,item_type);

-- Per-field relevance multipliers used at query time to rank results.
CREATE TABLE search_value_weight (
    key_name VARCHAR(256) PRIMARY KEY,
    weight   REAL
);

-- Individual searchable key/value pairs for each item.
-- Cascades on delete so removing a search_item cleans up all its search values.
-- PK includes key_value to allow multiple values per key (e.g. multiple creators).
CREATE TABLE search_value (
    item_id   UUID         REFERENCES search_item (item_id) ON DELETE CASCADE,
    key_name  VARCHAR(256) NOT NULL,
    key_value TEXT         NOT NULL,
    PRIMARY KEY (item_id, key_name, key_value)
);

CREATE INDEX search_value_key_value_trgm_idx ON search_value USING GIN (key_value gin_trgm_ops);

-- Creation metadata for each search item (audit trail).
CREATE TABLE search_item_metadata (
    item_id          UUID        PRIMARY KEY REFERENCES search_item (item_id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    creator_ip       INET        NOT NULL,
    creator_agent    TEXT        NOT NULL DEFAULT '',
    accept_language  TEXT        NOT NULL DEFAULT '',
    referer          TEXT        NOT NULL DEFAULT '',
    content_type     TEXT        NOT NULL DEFAULT '',
    request_id       TEXT        NOT NULL DEFAULT ''
);
