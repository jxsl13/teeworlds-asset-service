-- name: ListItems :many
SELECT
    si.item_id,
    si.item_type,
    si.item_value,
    COUNT(*) OVER () AS total_count
FROM search_item si
LEFT JOIN search_item_metadata sim ON si.item_id = sim.item_id
WHERE si.item_type = $1
  AND (
      sqlc.narg(filter_name)::text IS NULL
      OR EXISTS (
          SELECT 1 FROM search_value sv
          WHERE sv.item_id = si.item_id
            AND sv.key_name = 'name'
            AND sv.key_value ILIKE '%' || sqlc.narg(filter_name)::text || '%'
      )
  )
  AND (
      sqlc.narg(filter_creator)::text IS NULL
      OR EXISTS (
          SELECT 1 FROM search_value sv
          WHERE sv.item_id = si.item_id
            AND sv.key_name = 'creator'
            AND sv.key_value ILIKE '%' || sqlc.narg(filter_creator)::text || '%'
      )
  )
  AND (
      sqlc.narg(filter_license)::text IS NULL
      OR EXISTS (
          SELECT 1 FROM search_value sv
          WHERE sv.item_id = si.item_id
            AND sv.key_name = 'license'
            AND sv.key_value = sqlc.narg(filter_license)::text
      )
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_field)::text
      WHEN 'name'       THEN si.item_value->>'name'
      WHEN 'created_at' THEN to_char(COALESCE(sim.created_at, '1970-01-01'::timestamptz), 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
      ELSE si.item_value->>'name'
    END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_field)::text
      WHEN 'name'       THEN si.item_value->>'name'
      WHEN 'created_at' THEN to_char(COALESCE(sim.created_at, '1970-01-01'::timestamptz), 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
      ELSE si.item_value->>'name'
    END
  END DESC,
  si.item_id ASC
LIMIT $2 OFFSET $3;
