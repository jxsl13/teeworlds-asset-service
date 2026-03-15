-- name: Search :many
SELECT
    sm.item_id,
    si.item_type,
    sm.sml,
    si.item_value,
    COUNT(*) OVER () AS total_count
FROM search_item si
JOIN (
    SELECT
        sv.item_id,
        CAST(
            SUM(strict_word_similarity(sv.key_value, $1) + COALESCE(sw.weight, 0))
            AS FLOAT8
        ) AS sml
    FROM  search_value sv
    LEFT JOIN search_value_weight sw ON sv.key_name = sw.key_name
    WHERE key_value ~* $1 -- search keywords
    GROUP BY sv.item_id
) AS sm ON si.item_id = sm.item_id
ORDER BY sm.sml DESC
LIMIT $2 OFFSET $3;

-- name: SearchByType :many
SELECT
    sm.item_id,
    si.item_type,
    sm.sml,
    si.item_value,
    COUNT(*) OVER () AS total_count
FROM search_item si
JOIN (
    SELECT
        sv.item_id,
        CAST(
            SUM(strict_word_similarity(sv.key_value, $1) + COALESCE(sw.weight, 0))
            AS FLOAT8
        ) AS sml
    FROM  search_value sv
    LEFT JOIN search_value_weight sw ON sv.key_name = sw.key_name
    WHERE key_value ~* $1 -- search keywords
    GROUP BY sv.item_id
) AS sm ON si.item_id = sm.item_id
WHERE si.item_type = $4
ORDER BY sm.sml DESC
LIMIT $2 OFFSET $3;
