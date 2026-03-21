-- Simple key-value store for encrypted service settings (e.g. OIDC credentials).
-- The TEXT value column stores base64-encoded AES-GCM ciphertexts.
CREATE TABLE kv_store (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
