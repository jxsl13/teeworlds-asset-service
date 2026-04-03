-- name: Search :many
SELECT
    ag.group_id,
    ag.asset_type,
    ag.group_name,
    ag.group_key,
    CAST(COALESCE(
        (SELECT string_agg(sv2.key_value, ', ' ORDER BY sv2.key_value)
         FROM search_value sv2
         WHERE sv2.group_id = ag.group_id AND sv2.key_name = 'creators'),
        ''
    ) AS TEXT) AS creators,
    CAST(COALESCE(
        (SELECT sv3.key_value
         FROM search_value sv3
         WHERE sv3.group_id = ag.group_id AND sv3.key_name = 'license'
         LIMIT 1),
        ''
    ) AS TEXT) AS license,
    CAST(COALESCE(
        (SELECT string_agg(ai.item_id::text || ':' || ai.group_value, ',' ORDER BY ai.group_value)
         FROM asset_item ai
         WHERE ai.group_id = ag.group_id),
        ''
    ) AS TEXT) AS variants,
    sm.sml,
    COUNT(*) OVER () AS total_count
FROM asset_group ag
JOIN (
    SELECT
        sv.group_id,
        CAST(
            SUM(strict_word_similarity(sv.key_value, $1) + COALESCE(sw.weight, 0))
            AS FLOAT8
        ) AS sml
    FROM  search_value sv
    LEFT JOIN search_value_weight sw ON sv.key_name = sw.key_name
    WHERE (length($1) >= 3 AND key_value ~* $1)
       OR (length($1) <  3 AND sv.key_name <> 'license' AND lower(key_value) LIKE '%' || lower($1) || '%')
    GROUP BY sv.group_id
) AS sm ON ag.group_id = sm.group_id
ORDER BY sm.sml DESC
LIMIT $2 OFFSET $3;

-- name: SearchByType :many
SELECT
    ag.group_id,
    ag.asset_type,
    ag.group_name,
    ag.group_key,
    CAST(COALESCE(
        (SELECT string_agg(sv2.key_value, ', ' ORDER BY sv2.key_value)
         FROM search_value sv2
         WHERE sv2.group_id = ag.group_id AND sv2.key_name = 'creators'),
        ''
    ) AS TEXT) AS creators,
    CAST(COALESCE(
        (SELECT sv3.key_value
         FROM search_value sv3
         WHERE sv3.group_id = ag.group_id AND sv3.key_name = 'license'
         LIMIT 1),
        ''
    ) AS TEXT) AS license,
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
    sm.sml,
    COUNT(*) OVER () AS total_count
FROM asset_group ag
JOIN (
    SELECT
        sv.group_id,
        CAST(
            SUM(strict_word_similarity(sv.key_value, $1) + COALESCE(sw.weight, 0))
            AS FLOAT8
        ) AS sml
    FROM  search_value sv
    LEFT JOIN search_value_weight sw ON sv.key_name = sw.key_name
    WHERE (length($1) >= 3 AND key_value ~* $1)
       OR (length($1) <  3 AND sv.key_name <> 'license' AND lower(key_value) LIKE '%' || lower($1) || '%')
    GROUP BY sv.group_id
) AS sm ON ag.group_id = sm.group_id
WHERE ag.asset_type = $4
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = '' THEN NULL END,
  CASE WHEN sqlc.arg(sort_field)::text <> '' AND NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_field)::text
      WHEN 'name'       THEN ag.group_name
      WHEN 'creators'   THEN COALESCE(
          (SELECT string_agg(sv3.key_value, ', ' ORDER BY sv3.key_value)
           FROM search_value sv3
           WHERE sv3.group_id = ag.group_id AND sv3.key_name = 'creators'),
          '')
      WHEN 'license'    THEN COALESCE(
          (SELECT sv3.key_value FROM search_value sv3
           WHERE sv3.group_id = ag.group_id AND sv3.key_name = 'license' LIMIT 1),
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
  CASE WHEN sqlc.arg(sort_field)::text <> '' AND sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_field)::text
      WHEN 'name'       THEN ag.group_name
      WHEN 'creators'   THEN COALESCE(
          (SELECT string_agg(sv3.key_value, ', ' ORDER BY sv3.key_value)
           FROM search_value sv3
           WHERE sv3.group_id = ag.group_id AND sv3.key_name = 'creators'),
          '')
      WHEN 'license'    THEN COALESCE(
          (SELECT sv3.key_value FROM search_value sv3
           WHERE sv3.group_id = ag.group_id AND sv3.key_name = 'license' LIMIT 1),
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
  sm.sml DESC
LIMIT $2 OFFSET $3;
