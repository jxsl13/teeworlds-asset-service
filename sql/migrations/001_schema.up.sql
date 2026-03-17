-- Enable the trigram extension required for strict_word_similarity() and ~* queries.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Plain CREATE TYPE so sqlc can infer the enum values for Go code generation.
-- golang-migrate tracks executed versions, so idempotency via DO $$ is not needed.
CREATE TYPE asset_type_enum AS ENUM (
    'map',
    'gameskin',
    'hud',
    'skin',
    'entity',
    'theme',
    'template',
    'emoticon'
);

-- Logical grouping of group_key (e.g. 'resolution') variants.
-- Each group has a single asset_type and a unique name within that type.
-- Metadata (name, creators, license) lives here because it is shared across resolutions.
CREATE TABLE asset_group (
    group_id    UUID            PRIMARY KEY,
    asset_type  asset_type_enum NOT NULL,
    group_name  TEXT            NOT NULL, -- user defined name, e.g. skin name
    group_key   TEXT            NOT NULL, -- e.g. resolution
    CONSTRAINT asset_group_type_name_key UNIQUE (asset_type, group_key, group_name)
);

CREATE INDEX asset_group_asset_type_group_key_group_name_idx ON asset_group (asset_type, group_key, group_name);


-- One row per uploaded file (one variant within a group, e.g. a specific resolution).
CREATE TABLE asset_item (
    group_id            UUID            NOT NULL REFERENCES asset_group (group_id) ON DELETE CASCADE,
    item_id             UUID            PRIMARY KEY,
    group_value         TEXT            NOT NULL, -- e.g. 512x256
    size                BIGINT          NOT NULL,
    checksum            VARCHAR(128)    NOT NULL,
    item_file_path      TEXT            NOT NULL
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
    original_filename   TEXT NOT NULL DEFAULT '',
    -- we assume that there is not supposed to be a checksum collision across asset types,
    -- otherwise we'd have to also keep track of the asset_type in this table
    CONSTRAINT asset_asset_type_checksum_key UNIQUE (checksum),
    -- there can only be a single e.g. resolution 1920x1080 for a given group
    CONSTRAINT asset_item_group_variant_key UNIQUE (group_id, group_value)
);

CREATE INDEX asset_item_group_id_idx ON asset_item (group_id, group_value);
CREATE INDEX asset_item_checksum_idx ON asset_item (checksum);

-- Creation metadata for each search item (audit trail).
CREATE TABLE asset_item_metadata (
    item_id          UUID        PRIMARY KEY REFERENCES asset_item (item_id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    creator_ip       INET        NOT NULL,
    creator_agent    TEXT        NOT NULL DEFAULT '',
    accept_language  TEXT        NOT NULL DEFAULT '',
    referer          TEXT        NOT NULL DEFAULT '',
    content_type     TEXT        NOT NULL DEFAULT '',
    request_id       TEXT        NOT NULL DEFAULT ''
);

-- Per-field relevance multipliers used at query time to rank results.
CREATE TABLE search_value_weight (
    key_name VARCHAR(256) PRIMARY KEY,
    weight   REAL
);

-- Individual searchable key/value pairs for each group.
-- Cascades on delete so removing an asset_group cleans up all its search values.
-- PK includes key_value to allow multiple values per key (e.g. multiple creators).
CREATE TABLE search_value (
    group_id  UUID         REFERENCES asset_group (group_id) ON DELETE CASCADE,
    key_name  VARCHAR(256) NOT NULL,
    key_value TEXT         NOT NULL,
    PRIMARY KEY (group_id, key_name, key_value)
);

CREATE INDEX search_value_key_value_trgm_idx ON search_value USING GIN (key_value gin_trgm_ops);


