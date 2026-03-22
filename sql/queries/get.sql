-- name: GetItemFilePath :one
SELECT ai.item_file_path, ai.original_filename
FROM   asset_item ai
JOIN   asset_group ag ON ai.group_id = ag.group_id
WHERE  ai.item_id    = $1
AND    ag.asset_type  = $2;

-- name: GetItemThumbnailPath :one
SELECT ai.item_thumbnail_path, ai.thumbnail_checksum
FROM   asset_item ai
JOIN   asset_group ag ON ai.group_id = ag.group_id
WHERE  ai.item_id    = $1
AND    ag.asset_type  = $2
AND    ai.item_thumbnail_path IS NOT NULL;

-- name: GetGroupThumbnailPath :one
SELECT ai.item_thumbnail_path, ai.thumbnail_checksum
FROM   asset_item ai
WHERE  ai.group_id = $1
AND    ai.item_thumbnail_path IS NOT NULL
ORDER BY ai.size ASC
LIMIT 1;

-- name: GetGroupFilePath :one
SELECT ai.item_file_path, ai.original_filename
FROM   asset_item ai
JOIN   asset_group ag ON ai.group_id = ag.group_id
WHERE  ai.group_id   = $1
AND    ag.asset_type  = $2
ORDER BY ai.size ASC
LIMIT 1;

-- name: GetItemByChecksum :one
SELECT ai.item_id, ag.group_id, ag.asset_type, ag.group_name, ai.group_value
FROM   asset_item ai
JOIN   asset_group ag ON ai.group_id = ag.group_id
WHERE  ai.checksum = $1;

-- name: GetGroupFiles :many
SELECT ag.group_name, ai.group_value, ai.item_file_path, ai.original_filename
FROM   asset_item ai
JOIN   asset_group ag ON ai.group_id = ag.group_id
WHERE  ai.group_id   = $1
AND    ag.asset_type  = $2
ORDER BY ai.size ASC;

-- name: GetMultiGroupFiles :many
SELECT ag.asset_type,
       ag.group_name,
       ai.group_value,
       ai.item_file_path,
       ai.original_filename
FROM   asset_item ai
JOIN   asset_group ag ON ai.group_id = ag.group_id
WHERE  ai.group_id = ANY(sqlc.slice(group_ids)::uuid[])
ORDER BY ag.asset_type, ag.group_name, ai.size ASC;
