-- name: GetItemFilePath :one
SELECT item_file_path
FROM   search_item
WHERE  item_id   = $1
AND    item_type = $2;

-- name: GetItemThumbnailPath :one
SELECT item_thumbnail_path
FROM   search_item
WHERE  item_id   = $1
AND    item_type = $2
AND    item_thumbnail_path IS NOT NULL;
