-- name: get-user-roles
WITH mainroles AS (
    SELECT ur.* FROM roles ur WHERE type = 'user' AND ur.parent_id IS NULL AND
    CASE WHEN $1 != 0 THEN ur.id = $1 ELSE TRUE END
),
listPerms AS (
    SELECT ur.parent_id, JSON_AGG(JSON_BUILD_OBJECT('id', ur.list_id, 'name', lists.name, 'permissions', ur.permissions)) AS listPerms
    FROM roles ur
    LEFT JOIN lists ON(lists.id = ur.list_id)
    WHERE ur.parent_id IS NOT NULL GROUP BY ur.parent_id
)
SELECT p.*, COALESCE(l.listPerms, '[]') AS "list_permissions" FROM mainroles p
    LEFT JOIN listPerms l ON p.id = l.parent_id ORDER BY p.created;

-- name: get-list-roles
WITH mainroles AS (
    SELECT ur.* FROM roles ur WHERE type = 'list' AND ur.parent_id IS NULL
),
listPerms AS (
    SELECT ur.parent_id, JSON_AGG(JSON_BUILD_OBJECT('id', ur.list_id, 'name', lists.name, 'permissions', ur.permissions)) AS listPerms
    FROM roles ur
    LEFT JOIN lists ON(lists.id = ur.list_id)
    WHERE ur.parent_id IS NOT NULL GROUP BY ur.parent_id
)
SELECT p.*, COALESCE(l.listPerms, '[]') AS "list_permissions" FROM mainroles p
    LEFT JOIN listPerms l ON p.id = l.parent_id ORDER BY p.created;


-- name: create-role
INSERT INTO roles (name, type, permissions, created, updated)
VALUES($1, $2, $3, strftime('%Y-%m-%d %H:%M:%fZ', 'now'), strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
RETURNING *;

-- name: upsert-list-permissions
WITH d AS (
    -- Delete lists that aren't included.
    DELETE FROM roles WHERE parent_id = $1 AND list_id NOT IN $2[]
),
p AS (
    -- Get (list_id, perms[]), (list_id, perms[])
    SELECT UNNEST($2) AS list_id, JSON_ARRAY_ELEMENTS(TO_JSON($3[][])) AS perms
)
INSERT INTO roles (parent_id, list_id, permissions, type)
    SELECT $1, list_id, ARRAY_REMOVE(ARRAY(SELECT JSON_ARRAY_ELEMENTS_TEXT(perms)), ''), 'list' FROM p
    ON CONFLICT (parent_id, list_id) DO UPDATE SET permissions = EXCLUDED.permissions;

-- name: delete-list-permission
DELETE FROM roles WHERE parent_id=$1 AND list_id=$2;

-- name: update-role
UPDATE roles SET name=$2, permissions=$3 WHERE id=$1 and parent_id IS NULL RETURNING *;

-- name: delete-role
DELETE FROM roles WHERE id=$1;
