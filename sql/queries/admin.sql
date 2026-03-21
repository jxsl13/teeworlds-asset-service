-- name: DeleteGroup :exec
DELETE FROM asset_group
WHERE group_id = $1
AND   asset_type = $2;

-- name: DeleteItem :exec
DELETE FROM asset_item
WHERE item_id   = $1
AND   group_id  = $2;

-- name: UpdateGroupName :exec
UPDATE asset_group
SET    group_name = $1
WHERE  group_id   = $2
AND    asset_type = $3;

-- name: DeleteSearchValues :exec
DELETE FROM search_value
WHERE group_id = $1
AND   key_name = $2;

-- name: GetGroupInfo :one
SELECT ag.group_id, ag.asset_type, ag.group_name, ag.group_key
FROM   asset_group ag
WHERE  ag.group_id  = $1
AND    ag.asset_type = $2;

-- name: GetItemInfo :one
SELECT ai.item_id, ai.group_id, ai.group_value, ai.size, ai.item_file_path, ai.item_thumbnail_path
FROM   asset_item ai
WHERE  ai.item_id  = $1
AND    ai.group_id = $2;

-- name: GetGroupItemPaths :many
SELECT ai.item_file_path, ai.item_thumbnail_path
FROM   asset_item ai
WHERE  ai.group_id = $1;

-- name: UpdateItem :exec
UPDATE asset_item
SET    size              = $1,
       checksum          = $2,
       item_file_path    = $3,
       item_thumbnail_path = sqlc.narg(item_thumbnail_path),
       original_filename = $4
WHERE  item_id  = $5
AND    group_id = $6;

-- name: CountGroupItems :one
SELECT COUNT(*) FROM asset_item WHERE group_id = $1;

-- name: CountGroupsCreatedByIP :one
-- Returns the number of distinct asset groups first created by the given IP
-- within the given time window. A group is "created by" an IP if that IP
-- uploaded the earliest item in the group.
SELECT COUNT(*)::BIGINT
FROM (
    SELECT DISTINCT ON (ai.group_id)
        aim.creator_ip,
        aim.created_at
    FROM asset_item ai
    JOIN asset_item_metadata aim ON aim.item_id = ai.item_id
    ORDER BY ai.group_id, aim.created_at ASC
) AS first_uploads
WHERE creator_ip = $1
  AND created_at >= sqlc.arg(since)::TIMESTAMPTZ;

-- name: GetGroupItems :many
SELECT ai.item_id,
       ai.group_value,
       ai.size,
       ai.original_filename
FROM   asset_item ai
WHERE  ai.group_id = $1
ORDER  BY ai.group_value;

-- name: GetGroupItemsWithMetadata :many
SELECT ai.item_id,
       ai.group_value,
       ai.size,
       ai.original_filename,
       aim.created_at,
       host(aim.creator_ip)::TEXT AS creator_ip,
       aim.creator_agent,
       aim.accept_language,
       aim.referer,
       aim.content_type,
       aim.request_id
FROM   asset_item ai
JOIN   asset_item_metadata aim ON aim.item_id = ai.item_id
WHERE  ai.group_id = $1
ORDER  BY ai.group_value;
