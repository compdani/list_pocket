-- lists
-- name: get-lists
SELECT * FROM lists WHERE (CASE WHEN $1 = '' THEN 1=1 ELSE type=$1 END)
    AND (CASE WHEN $2 = '' THEN 1=1 ELSE status=$2 END)
    AND CASE
        -- Optional list IDs based on user permission.
        WHEN $4 = TRUE THEN TRUE ELSE id IN $5[]
    END
    ORDER BY CASE WHEN $3 = 'id' THEN id END, CASE WHEN $3 = 'name' THEN name END;

-- name: query-lists
WITH ls AS (
    SELECT COUNT(*) OVER () AS total, lists.* FROM lists WHERE
    CASE
        WHEN $1 > 0 THEN id = $1
        WHEN $2 != '' THEN uuid = $2
        WHEN $3 != '' THEN (TO_TSVECTOR(name) @@ TO_TSQUERY ($3) OR name ILIKE $3)
        ELSE TRUE
    END
    AND ($4 = '' OR type = $4)
    AND ($5 = '' OR optin = $5)
    AND ($6 = '' OR status = $6)
    AND (CARDINALITY($7(100)[]) = 0 OR $7 <@ tags)
    AND CASE
        -- Optional list IDs based on user permission.
        WHEN $8 = TRUE THEN TRUE ELSE id IN $9[]
    END
    OFFSET $10 LIMIT (CASE WHEN $11 < 1 THEN NULL ELSE $11 END)
),
statuses AS (
    SELECT
        list_id,
        COALESCE(JSON_OBJECT_AGG(status, subscriber_count) FILTER (WHERE status IS NOT NULL), '{}') AS subscriber_statuses,
        SUM(subscriber_count) AS subscriber_count
    FROM mat_list_subscriber_stats
    GROUP BY list_id
)
SELECT ls.*, COALESCE(ss.subscriber_statuses, '{}') AS subscriber_statuses, COALESCE(ss.subscriber_count, 0) AS subscriber_count
    FROM ls LEFT JOIN statuses ss ON (ls.id = ss.list_id) ORDER BY %order%;

-- name: get-lists-by-optin
-- Can have a list of IDs or a list of UUIDs.
SELECT * FROM lists WHERE (CASE WHEN $1 != '' THEN optin=$1 ELSE TRUE END) AND
    (CASE WHEN $2[] IS NOT NULL THEN id IN $2[]
          WHEN $3[] IS NOT NULL THEN uuid IN $3[]
    END) ORDER BY name;

-- name: get-list-types
-- Retrieves the private|public type of lists by ID or uuid. Used for filtering.
SELECT id, uuid, type FROM lists WHERE
    (CASE WHEN $1[] IS NOT NULL THEN id IN $1[]
          WHEN $2[] IS NOT NULL THEN uuid IN $2[]
    END);

-- name: create-list
INSERT INTO lists (uuid, name, type, optin, status, tags, description) VALUES($1, $2, $3, $4, $5, $6, $7) RETURNING id;

-- name: update-list
WITH l AS (
    UPDATE lists SET
        name=(CASE WHEN $2 != '' THEN $2 ELSE name END),
        type=(CASE WHEN $3 != '' THEN $3 ELSE type END),
        optin=(CASE WHEN $4 != '' THEN $4 ELSE optin END),
        status=(CASE WHEN $5 != '' THEN $5 ELSE status END),
        tags=$6(100)[],
        description=(CASE WHEN $7 != '' THEN $7 ELSE description END),
        updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now')
    WHERE id = $1
    RETURNING id, name
),
c AS (
    UPDATE campaign_lists SET list_name = l.name FROM l WHERE campaign_lists.list_id = l.id RETURNING 1
)
SELECT COUNT(*) FROM l, c;

-- name: update-lists-date
UPDATE lists SET updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now') WHERE id IN $1;

-- name: delete-lists
DELETE FROM lists
WHERE CASE
    WHEN CARDINALITY($1[]) > 0 THEN id IN $1
    ELSE ($2 = '' OR to_tsvector(name) @@ to_tsquery($2))
END
AND CASE
    -- Optional list IDs based on user permission.
    WHEN $3 = TRUE THEN TRUE ELSE id IN $4[]
END;

