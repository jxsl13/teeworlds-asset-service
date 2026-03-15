-- name: InsertItem :execrows
INSERT INTO search_item (item_id, item_type, size, checksum, item_file_path, item_thumbnail_path, item_value)
SELECT $1, $2, $3, $4, $5, sqlc.narg(item_thumbnail_path), $6
WHERE (SELECT total_size FROM storage_stats) + $3 <= sqlc.arg(max_total_size);

-- name: InsertItemMetadata :exec
INSERT INTO search_item_metadata (
    item_id,
    creator_ip,
    creator_agent,
    accept_language,
    referer,
    content_type,
    request_id
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: InsertSearchValue :exec
INSERT INTO search_value (item_id, key_name, key_value)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;
