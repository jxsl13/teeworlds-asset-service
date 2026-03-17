-- name: ListItems :many
SELECT
    ag.group_id,
    ag.asset_type,
    ag.group_name,
    ag.group_key,
    CAST(COALESCE(
        (SELECT string_agg(sv.key_value, ', ' ORDER BY sv.key_value)
         FROM search_value sv
         WHERE sv.group_id = ag.group_id AND sv.key_name = 'creators'),
        ''
    ) AS TEXT) AS creators,
    CAST(COALESCE(
        (SELECT string_agg(ai.item_id::text || ':' || ai.group_value, ',' ORDER BY ai.group_value)
         FROM asset_item ai
         WHERE ai.group_id = ag.group_id),
        ''
    ) AS TEXT) AS variants,
    COALESCE(
        (SELECT SUM(ai.size) FROM asset_item ai WHERE ai.group_id = ag.group_id),
        0
    )::BIGINT AS total_size,
    COALESCE(
        (SELECT MIN(sim.created_at) FROM asset_item ai JOIN asset_item_metadata sim ON ai.item_id = sim.item_id WHERE ai.group_id = ag.group_id),
        '1970-01-01'::timestamptz
    )::timestamptz AS created_at,
    COUNT(*) OVER () AS total_count
FROM asset_group ag
WHERE ag.asset_type = $1
  AND (
      sqlc.narg(filter_name)::text IS NULL
      OR EXISTS (
          SELECT 1 FROM search_value sv
          WHERE sv.group_id = ag.group_id
            AND sv.key_name = 'name'
            AND sv.key_value ILIKE '%' || sqlc.narg(filter_name)::text || '%'
      )
  )
  AND (
      sqlc.narg(filter_creator)::text IS NULL
      OR EXISTS (
          SELECT 1 FROM search_value sv
          WHERE sv.group_id = ag.group_id
            AND sv.key_name = 'creator'
            AND sv.key_value ILIKE '%' || sqlc.narg(filter_creator)::text || '%'
      )
  )
  AND (
      sqlc.narg(filter_license)::text IS NULL
      OR EXISTS (
          SELECT 1 FROM search_value sv
          WHERE sv.group_id = ag.group_id
            AND sv.key_name = 'license'
            AND sv.key_value = sqlc.narg(filter_license)::text
      )
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_field)::text
      WHEN 'name'       THEN ag.group_name
      WHEN 'creators'   THEN COALESCE(
          (SELECT string_agg(sv.key_value, ', ' ORDER BY sv.key_value)
           FROM search_value sv
           WHERE sv.group_id = ag.group_id AND sv.key_name = 'creators'),
          '')
      WHEN 'size'       THEN lpad(cast(COALESCE(
          (SELECT SUM(ai.size) FROM asset_item ai WHERE ai.group_id = ag.group_id),
          0) as text), 20, '0')
      WHEN 'created_at' THEN to_char(COALESCE(
          (SELECT MIN(sim.created_at) FROM asset_item ai JOIN asset_item_metadata sim ON ai.item_id = sim.item_id WHERE ai.group_id = ag.group_id),
          '1970-01-01'::timestamptz
      ), 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
      ELSE ag.group_name
    END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_field)::text
      WHEN 'name'       THEN ag.group_name
      WHEN 'creators'   THEN COALESCE(
          (SELECT string_agg(sv.key_value, ', ' ORDER BY sv.key_value)
           FROM search_value sv
           WHERE sv.group_id = ag.group_id AND sv.key_name = 'creators'),
          '')
      WHEN 'size'       THEN lpad(cast(COALESCE(
          (SELECT SUM(ai.size) FROM asset_item ai WHERE ai.group_id = ag.group_id),
          0) as text), 20, '0')
      WHEN 'created_at' THEN to_char(COALESCE(
          (SELECT MIN(sim.created_at) FROM asset_item ai JOIN asset_item_metadata sim ON ai.item_id = sim.item_id WHERE ai.group_id = ag.group_id),
          '1970-01-01'::timestamptz
      ), 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
      ELSE ag.group_name
    END
  END DESC,
  CASE WHEN sqlc.arg(sort_field_2)::text <> '' AND NOT sqlc.arg(sort_desc_2)::bool THEN
    CASE sqlc.arg(sort_field_2)::text
      WHEN 'name'       THEN ag.group_name
      WHEN 'creators'   THEN COALESCE(
          (SELECT string_agg(sv.key_value, ', ' ORDER BY sv.key_value)
           FROM search_value sv
           WHERE sv.group_id = ag.group_id AND sv.key_name = 'creators'),
          '')
      WHEN 'size'       THEN lpad(cast(COALESCE(
          (SELECT SUM(ai.size) FROM asset_item ai WHERE ai.group_id = ag.group_id),
          0) as text), 20, '0')
      WHEN 'created_at' THEN to_char(COALESCE(
          (SELECT MIN(sim.created_at) FROM asset_item ai JOIN asset_item_metadata sim ON ai.item_id = sim.item_id WHERE ai.group_id = ag.group_id),
          '1970-01-01'::timestamptz
      ), 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
      ELSE NULL
    END
  END ASC,
  CASE WHEN sqlc.arg(sort_field_2)::text <> '' AND sqlc.arg(sort_desc_2)::bool THEN
    CASE sqlc.arg(sort_field_2)::text
      WHEN 'name'       THEN ag.group_name
      WHEN 'creators'   THEN COALESCE(
          (SELECT string_agg(sv.key_value, ', ' ORDER BY sv.key_value)
           FROM search_value sv
           WHERE sv.group_id = ag.group_id AND sv.key_name = 'creators'),
          '')
      WHEN 'size'       THEN lpad(cast(COALESCE(
          (SELECT SUM(ai.size) FROM asset_item ai WHERE ai.group_id = ag.group_id),
          0) as text), 20, '0')
      WHEN 'created_at' THEN to_char(COALESCE(
          (SELECT MIN(sim.created_at) FROM asset_item ai JOIN asset_item_metadata sim ON ai.item_id = sim.item_id WHERE ai.group_id = ag.group_id),
          '1970-01-01'::timestamptz
      ), 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
      ELSE NULL
    END
  END DESC,
  ag.group_id ASC
LIMIT $2 OFFSET $3;
