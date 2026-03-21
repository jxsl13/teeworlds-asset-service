-- name: GetKV :one
SELECT value FROM kv_store WHERE key = $1;

-- name: UpsertKV :exec
INSERT INTO kv_store (key, value)
VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;
