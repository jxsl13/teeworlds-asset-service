-- name: UpsertGroup :exec
INSERT INTO asset_group (group_id, asset_type, group_name, group_key)
VALUES ($1, $2, $3, $4)
ON CONFLICT (asset_type, group_name, group_key) DO NOTHING;

-- name: GetGroupID :one
SELECT group_id
FROM   asset_group
WHERE  asset_type = $1
AND    group_name = $2
AND    group_key  = $3;

-- name: InsertItem :execrows
INSERT INTO asset_item (item_id, group_id, group_value, size, checksum, item_file_path, item_thumbnail_path, thumbnail_checksum, original_filename)
SELECT $1, $2, $3, $4, $5, $6, sqlc.narg(item_thumbnail_path), $7, $8
WHERE (SELECT total_size FROM storage_stats) + $4 <= sqlc.arg(max_total_size);

-- name: InsertItemMetadata :exec
INSERT INTO asset_item_metadata (
    item_id,
    creator_ip,
    creator_agent,
    accept_language,
    referer,
    content_type,
    request_id
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: InsertSearchValue :exec
INSERT INTO search_value (group_id, key_name, key_value)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;
