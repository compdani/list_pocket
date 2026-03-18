-- subscribers
-- name: get-subscriber
-- Get a single subscriber by id or UUID or email.
SELECT * FROM subscribers WHERE
    CASE
        WHEN $1 > 0 THEN id = $1
        WHEN $2 != '' THEN uuid = $2
        WHEN $3 != '' THEN email = $3
    END;

-- name: has-subscriber-list
-- Used for checking access permission by list.
SELECT s.id AS subscriber_id,
    CASE
        WHEN EXISTS (SELECT 1 FROM subscriber_lists sl WHERE sl.subscriber_id = s.id AND sl.list_id IN $2)
        THEN TRUE
        ELSE FALSE
    END AS has
FROM subscribers s WHERE s.id IN $1;

-- name: get-subscribers-by-emails
-- Get subscribers by emails.
SELECT * FROM subscribers WHERE email=ANY($1);

-- name: get-subscriber-lists
WITH sub AS (
    SELECT id FROM subscribers WHERE CASE WHEN $1 > 0 THEN id = $1 ELSE uuid = $2 END
)
SELECT * FROM lists
    LEFT JOIN subscriber_lists ON (lists.id = subscriber_lists.list_id)
    WHERE subscriber_id = (SELECT id FROM sub)
    -- Optional list IDs or UUIDs to filter.
    AND (CASE WHEN CARDINALITY($3[]) > 0 THEN id IN $3[]
          WHEN CARDINALITY($4[]) > 0 THEN uuid IN $4[]
          ELSE TRUE
    END)
    AND (CASE WHEN $5 != '' THEN subscriber_lists.status = $5 ELSE TRUE END)
    AND (CASE WHEN $6 != '' THEN lists.optin = $6 ELSE TRUE END)
    ORDER BY id;

-- name: get-subscriber-lists-lazy
-- Get lists associations of subscribers given a list of subscriber IDs.
-- This query is used to lazy load given a list of subscriber IDs.
-- The query returns results in the same order as the given subscriber IDs, and for non-existent subscriber IDs,
-- the query still returns a row with 0 values. Thus, for lazy loading, the application simply iterate on the results in
-- the same order as the list of campaigns it would've queried and attach the results.
WITH subs AS (
    SELECT subscriber_id, JSON_AGG(
        ROW_TO_JSON(
            (SELECT l FROM (
                SELECT
                    subscriber_lists.status AS subscription_status,
                    subscriber_lists.created AS subscription_created,
                    subscriber_lists.updated AS subscription_updated,
                    subscriber_lists.meta AS subscription_meta,
                    lists.*
            ) l)
        )
    ) AS lists FROM lists
    LEFT JOIN subscriber_lists ON (subscriber_lists.list_id = lists.id)
    WHERE subscriber_lists.subscriber_id IN $1
    GROUP BY subscriber_id
)
SELECT id as subscriber_id,
    COALESCE(s.lists, '[]') AS lists
    FROM (SELECT id FROM UNNEST($1) AS id) x
    LEFT JOIN subs AS s ON (s.subscriber_id = id)
    ORDER BY ARRAY_POSITION($1, id);

-- name: get-subscriptions
-- Retrieves all lists a subscriber is attached to.
-- if $3 is set to true, all lists are fetched including the subscriber's subscriptions.
-- subscription_status, and subscription_created are null in that case.
WITH sub AS (
    SELECT id FROM subscribers WHERE CASE WHEN $1 > 0 THEN id = $1 ELSE uuid = $2 END
)
SELECT lists.*,
    subscriber_lists.status as subscription_status,
    subscriber_lists.created as subscription_created,
    subscriber_lists.meta as subscription_meta
    FROM lists LEFT JOIN subscriber_lists
    ON (subscriber_lists.list_id = lists.id AND subscriber_lists.subscriber_id = (SELECT id FROM sub))
    WHERE CASE WHEN $3 = TRUE THEN TRUE ELSE subscriber_lists.status IS NOT NULL END
    ORDER BY subscriber_lists.status;

-- name: insert-subscriber
WITH sub AS (
    INSERT INTO subscribers (uuid, email, first_name, last_name, name, status, attribs)
    VALUES($1, $2, $3, $4, $5, $6, $7)
    RETURNING id, status
),
listIDs AS (
    SELECT id FROM lists WHERE
        (CASE WHEN CARDINALITY($8[]) > 0 THEN id=ANY($8)
              ELSE uuid=ANY($9[]) END)
),
subs AS (
    INSERT INTO subscriber_lists (subscriber_id, list_id, status)
    VALUES(
        (SELECT id FROM sub),
        UNNEST(ARRAY(SELECT id FROM listIDs)),
        (CASE WHEN $6='blocklisted' THEN 'unsubscribed' ELSE $10 END)
    )
    ON CONFLICT (subscriber_id, list_id) DO UPDATE
        SET updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now'),
            status=(
                CASE WHEN $6='blocklisted' OR (SELECT status FROM sub)='blocklisted'
                THEN 'unsubscribed'
                ELSE $10 END
            )
)
SELECT id from sub;

-- name: upsert-subscriber
-- Upserts a subscriber where existing subscribers get their names and attributes overwritten.
-- If $7 = true, update name/attribs. If $8 = true, update subscription status.
WITH sub AS (
    INSERT INTO subscribers as s (uuid, email, first_name, last_name, name, attribs, status)
    VALUES($1, $2, $3, $4, $5, $6, 'enabled')
    ON CONFLICT (email)
    DO UPDATE SET
        first_name=(CASE WHEN $8 THEN $3 ELSE s.first_name END),
        last_name=(CASE WHEN $8 THEN $4 ELSE s.last_name END),
        name=(CASE WHEN $8 THEN $5 ELSE s.name END),
        attribs=(CASE WHEN $8 THEN $6 ELSE s.attribs END),
        updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now')
    RETURNING uuid, id, status
),
subs AS (
    INSERT INTO subscriber_lists (subscriber_id, list_id, status)
    SELECT sub.id, listID, CASE WHEN sub.status = 'blocklisted' THEN 'unsubscribed' ELSE $7 END
    FROM sub, UNNEST($6[]) AS listID
    ON CONFLICT (subscriber_id, list_id) DO UPDATE
    SET updated = strftime('%Y-%m-%d %H:%M:%fZ', 'now'),
        status = CASE WHEN $9 THEN EXCLUDED.status ELSE subscriber_lists.status END
)
SELECT uuid, id from sub;

-- name: upsert-blocklist-subscriber
-- Upserts a subscriber where the update will only set the status to blocklisted
-- unlike upsert-subscribers where name and attributes are updated. In addition, all
-- existing subscriptions are marked as 'unsubscribed'.
-- This is used in the bulk importer.
WITH sub AS (
    INSERT INTO subscribers (uuid, email, first_name, last_name, name, attribs, status)
    VALUES($1, $2, $3, $4, $5, $6, 'blocklisted')
    ON CONFLICT (email) DO UPDATE SET status='blocklisted', updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now')
    RETURNING id
)
UPDATE subscriber_lists SET status='unsubscribed', updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now')
    WHERE subscriber_id = (SELECT id FROM sub);

-- name: update-subscriber
UPDATE subscribers SET
    email=(CASE WHEN $2 != '' THEN $2 ELSE email END),
    first_name=(CASE WHEN $3 != '' THEN $3 ELSE first_name END),
    last_name=(CASE WHEN $4 != '' THEN $4 ELSE last_name END),
    name=(CASE WHEN $5 != '' THEN $5 ELSE name END),
    status=(CASE WHEN $6 != '' THEN $6 ELSE status END),
    attribs=(CASE WHEN $7 != '' THEN $7 ELSE attribs END),
    updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now')
WHERE id = $1;

-- name: update-subscriber-with-lists
-- Updates a subscriber's data, and given a list of list_ids, inserts subscriptions
-- for them while deleting existing subscriptions not in the list.
WITH s AS (
    UPDATE subscribers SET
        email=(CASE WHEN $2 != '' THEN $2 ELSE email END),
        first_name=(CASE WHEN $3 != '' THEN $3 ELSE first_name END),
        last_name=(CASE WHEN $4 != '' THEN $4 ELSE last_name END),
        name=(CASE WHEN $5 != '' THEN $5 ELSE name END),
        status=(CASE WHEN $6 != '' THEN $6 ELSE status END),
        attribs=(CASE WHEN $7 != '' THEN $7 ELSE attribs END),
        updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now')
    WHERE id = $1 RETURNING id
),
listIDs AS (
    SELECT id FROM lists WHERE
        (CASE WHEN CARDINALITY($8[]) > 0 THEN id=ANY($8)
              ELSE uuid=ANY($9[]) END)
),
d AS (
    DELETE FROM subscriber_lists WHERE $11 = TRUE AND subscriber_id = $1 AND list_id NOT IN SELECT id FROM listIDs
)
INSERT INTO subscriber_lists (subscriber_id, list_id, status)
    VALUES(
        (SELECT id FROM s),
        UNNEST(ARRAY(SELECT id FROM listIDs)),
        (CASE WHEN $6='blocklisted' THEN 'unsubscribed' ELSE $10 END)
    )
    ON CONFLICT (subscriber_id, list_id) DO UPDATE
    SET status = (
        CASE
            WHEN $6='blocklisted' THEN 'unsubscribed'
            -- When subscriber is edited from the admin form, retain the status. Otherwise, a blocklisted
            -- subscriber when being re-enabled, their subscription statuses change.
            WHEN subscriber_lists.status = 'confirmed' THEN 'confirmed'
            WHEN subscriber_lists.status = 'unsubscribed' THEN 'unsubscribed'
            ELSE $10
        END
    );

-- name: delete-subscribers
-- Delete one or more subscribers by ID or UUID.
DELETE FROM subscribers WHERE CASE WHEN ARRAY_LENGTH($1[], 1) > 0 THEN id IN $1 ELSE uuid IN $2[] END;

-- name: delete-blocklisted-subscribers
DELETE FROM subscribers WHERE status = 'blocklisted';

-- name: delete-orphan-subscribers
DELETE FROM subscribers a WHERE NOT EXISTS
    (SELECT 1 FROM subscriber_lists b WHERE b.subscriber_id = a.id);

-- name: blocklist-subscribers
WITH b AS (
    UPDATE subscribers SET status='blocklisted', updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now')
    WHERE id IN $1[]
)
UPDATE subscriber_lists SET status='unsubscribed', updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now')
    WHERE subscriber_id IN $1[];

-- name: add-subscribers-to-lists
INSERT INTO subscriber_lists (subscriber_id, list_id, status)
    (SELECT a, b, (CASE WHEN $3 != '' THEN $3 ELSE 'unconfirmed' END) FROM UNNEST($1[]) a, UNNEST($2[]) b)
    ON CONFLICT (subscriber_id, list_id) DO UPDATE SET status=(CASE WHEN $3 != '' THEN $3 ELSE subscriber_lists.status END);

-- name: delete-subscriptions
DELETE FROM subscriber_lists
    WHERE (subscriber_id, list_id) IN SELECT a, b FROM UNNEST($1[] a, UNNEST($2[]) b);

-- name: confirm-subscription-optin
WITH subID AS (
    SELECT id FROM subscribers WHERE uuid = $1
),
listIDs AS (
    SELECT id FROM lists WHERE uuid IN $2[]
)
UPDATE subscriber_lists SET status='confirmed', meta=meta || $3, updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now')
    WHERE subscriber_id = (SELECT id FROM subID) AND list_id IN SELECT id FROM listIDs;

-- name: unsubscribe-subscribers-from-lists
WITH listIDs AS (
    SELECT ARRAY(
        SELECT id FROM lists WHERE
        (CASE WHEN CARDINALITY($2[]) > 0 THEN id=ANY($2) ELSE uuid=ANY($3[]) END)
    ) id
)
UPDATE subscriber_lists SET status='unsubscribed', updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now')
    WHERE (subscriber_id, list_id) IN SELECT a, b FROM UNNEST($1[] a, UNNEST((SELECT id FROM listIDs)) b);

-- name: unsubscribe-by-campaign
-- Unsubscribes a subscriber given a campaign UUID (from all the lists in the campaign) and the subscriber UUID.
-- If $3 is TRUE, then all subscriptions of the subscriber is blocklisted
-- and all existing subscriptions, irrespective of lists, unsubscribed.
WITH lists AS (
    SELECT list_id FROM campaign_lists
    LEFT JOIN campaigns ON (campaign_lists.campaign_id = campaigns.id)
    WHERE campaigns.uuid = $1
),
sub AS (
    UPDATE subscribers SET status = (CASE WHEN $3 IS TRUE THEN 'blocklisted' ELSE status END)
    WHERE uuid = $2 RETURNING id
)
UPDATE subscriber_lists SET status = 'unsubscribed', updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now') WHERE
    subscriber_id = (SELECT id FROM sub) AND status != 'unsubscribed' AND
    -- If $3 is false, unsubscribe from the campaign's lists, otherwise all lists.
    CASE WHEN $3 IS FALSE THEN list_id IN SELECT list_id FROM lists ELSE list_id != 0 END;

-- name: delete-unconfirmed-subscriptions
WITH optins AS (
    SELECT id FROM lists WHERE optin = 'double'
)
DELETE FROM subscriber_lists
    WHERE status = 'unconfirmed' AND list_id IN (SELECT id FROM optins) AND created < $1;


-- Partial and RAW queries used to construct arbitrary subscriber
-- queries for segmentation follow.

-- name: query-subscribers
-- raw: true
-- Unprepared statement for issuring arbitrary WHERE conditions for
-- searching subscribers. While the results are sliced using offset+limit,
-- there's a COUNT() OVER() that still returns the total result count
-- for pagination in the frontend, albeit being a field that'll repeat
-- with every resultant row.
SELECT subscribers.* FROM subscribers
    LEFT JOIN subscriber_lists
    ON (
        -- Optional list filtering.
        (CASE WHEN CARDINALITY($1[]) > 0 THEN true ELSE false END)
        AND subscriber_lists.subscriber_id = subscribers.id
        AND ($2 = '' OR subscriber_lists.status = $2)
    )
    WHERE (CARDINALITY($1) = 0 OR subscriber_lists.list_id IN $1[])
    AND (CASE WHEN $3 != '' THEN name ~* $3 OR email ~* $3 ELSE TRUE END)
    AND %query%
    ORDER BY %order% OFFSET $4 LIMIT (CASE WHEN $5 < 1 THEN NULL ELSE $5 END);

-- name: query-subscribers-count
-- Replica of query-subscribers for obtaining the results count.
SELECT COUNT(*) AS total FROM subscribers
    LEFT JOIN subscriber_lists
    ON (
        -- Optional list filtering.
        (CASE WHEN CARDINALITY($1[]) > 0 THEN true ELSE false END)
        AND subscriber_lists.subscriber_id = subscribers.id
        AND ($2 = '' OR subscriber_lists.status = $2)
    )
    WHERE (CARDINALITY($1) = 0 OR subscriber_lists.list_id IN $1[])
    AND (CASE WHEN $3 != '' THEN name ~* $3 OR email ~* $3 ELSE TRUE END)
    AND %query%;

-- name: query-subscribers-count-all
-- Cached query for getting the "all" subscriber count without arbitrary conditions.
SELECT COALESCE(SUM(subscriber_count), 0) AS total FROM mat_list_subscriber_stats
    WHERE list_id IN CASE WHEN CARDINALITY($1[] > 0 THEN $1 ELSE '{0}' END)
    AND ($2 = '' OR status = $2);

-- name: query-subscribers-for-export
-- raw: true
-- Unprepared statement for issuring arbitrary WHERE conditions for
-- searching subscribers to do bulk CSV export.
SELECT subscribers.id,
       subscribers.uuid,
       subscribers.email,
       subscribers.name,
       subscribers.status,
       subscribers.attribs,
       subscribers.created,
       subscribers.updated
       FROM subscribers
    LEFT JOIN subscriber_lists
    ON (
        -- Optional list filtering.
        (CASE WHEN CARDINALITY($1[]) > 0 THEN true ELSE false END)
        AND subscriber_lists.subscriber_id = subscribers.id
        AND ($4 = '' OR subscriber_lists.status = $4)
    )
    WHERE subscriber_lists.list_id = ALL($1[]) AND id > $2
    AND (CASE WHEN CARDINALITY($3[]) > 0 THEN id=ANY($3) ELSE true END)
    AND (CASE WHEN $5 != '' THEN name ~* $5 OR email ~* $5 ELSE TRUE END)
    AND %query%
    ORDER BY subscribers.id ASC LIMIT (CASE WHEN $6 < 1 THEN NULL ELSE $6 END);

-- name: query-subscribers-template
-- raw: true
-- This raw query is reused in multiple queries (blocklist, add to list, delete)
-- etc., so it's kept has a raw template to be injected into other raw queries,
-- and for the same reason, it is not terminated with a semicolon.
--
-- All queries that embed this query should expect
-- $1=true/false (dry-run or not) and $2=[]INT (option list IDs).
-- That is, their positional arguments should start from $4.
SELECT subscribers.id FROM subscribers
LEFT JOIN subscriber_lists
ON (
    -- Optional list filtering.
    (CASE WHEN CARDINALITY($2[]) > 0 THEN true ELSE false END)
    AND subscriber_lists.subscriber_id = subscribers.id
    AND ($3 = '' OR subscriber_lists.status = $3)
)
WHERE subscriber_lists.list_id = ALL($2[])
    AND (CASE WHEN $4 != '' THEN name ~* $4 OR email ~* $4 ELSE TRUE END)
    AND %query%
LIMIT (CASE WHEN $1 THEN 1 END)

-- name: delete-subscribers-by-query
-- raw: true
WITH subs AS (%query%)
DELETE FROM subscribers WHERE id=ANY(SELECT id FROM subs);

-- name: blocklist-subscribers-by-query
-- raw: true
WITH subs AS (%query%),
b AS (
    UPDATE subscribers SET status='blocklisted', updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now')
    WHERE id IN SELECT id FROM subs
)
UPDATE subscriber_lists SET status='unsubscribed', updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now')
    WHERE subscriber_id IN SELECT id FROM subs;

-- name: add-subscribers-to-lists-by-query
-- raw: true
WITH subs AS (%query%)
INSERT INTO subscriber_lists (subscriber_id, list_id, status)
    (SELECT a, b, (CASE WHEN $6 != '' THEN $6 ELSE 'unconfirmed' END) FROM UNNEST(ARRAY(SELECT id FROM subs)) a, UNNEST($5[]) b)
    ON CONFLICT (subscriber_id, list_id) DO NOTHING;

-- name: delete-subscriptions-by-query
-- raw: true
WITH subs AS (%query%)
DELETE FROM subscriber_lists
    WHERE (subscriber_id, list_id) IN SELECT a, b FROM UNNEST(ARRAY(SELECT id FROM subs) a, UNNEST($5[]) b);

-- name: unsubscribe-subscribers-from-lists-by-query
-- raw: true
WITH subs AS (%query%)
UPDATE subscriber_lists SET status='unsubscribed', updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now')
    WHERE (subscriber_id, list_id) IN SELECT a, b FROM UNNEST(ARRAY(SELECT id FROM subs) a, UNNEST($5[]) b);


-- privacy
-- name: export-subscriber-data
WITH prof AS (
    SELECT id, uuid, email, name, attribs, status, created, updated FROM subscribers WHERE
    CASE WHEN $1 > 0 THEN id = $1 ELSE uuid = $2 END
),
subs AS (
    SELECT subscriber_lists.status AS subscription_status,
            (CASE WHEN lists.type = 'private' THEN 'Private list' ELSE lists.name END) as name,
            lists.type, subscriber_lists.created
    FROM lists
    LEFT JOIN subscriber_lists ON (subscriber_lists.list_id = lists.id)
    WHERE subscriber_lists.subscriber_id = (SELECT id FROM prof)
),
views AS (
    SELECT subject as campaign, COUNT(subscriber_id) as views FROM campaign_views
        LEFT JOIN campaigns ON (campaigns.id = campaign_views.campaign_id)
        WHERE subscriber_id = (SELECT id FROM prof)
        GROUP BY campaigns.id ORDER BY campaigns.id
),
clicks AS (
    SELECT url, COUNT(subscriber_id) as clicks FROM link_clicks
        LEFT JOIN links ON (links.id = link_clicks.link_id)
        WHERE subscriber_id = (SELECT id FROM prof)
        GROUP BY links.id ORDER BY links.id
)
SELECT (SELECT email FROM prof) as email,
        COALESCE((SELECT JSON_AGG(t) FROM prof t), '{}') AS profile,
        COALESCE((SELECT JSON_AGG(t) FROM subs t), '[]') AS subscriptions,
        COALESCE((SELECT JSON_AGG(t) FROM views t), '[]') AS campaign_views,
        COALESCE((SELECT JSON_AGG(t) FROM clicks t), '[]') AS link_clicks;

-- name: get-subscriber-activity
-- Gets the subscriber's campaign views and link clicks with detailed information
-- for display in the Activity tab
WITH views AS (
    SELECT
        c.id,
        c.uuid,
        c.name,
        c.subject,
        COUNT(*) as view_count,
        MAX(cv.created) as last_viewed_at
    FROM campaign_views cv
    LEFT JOIN campaigns c ON c.id = cv.campaign_id
    WHERE cv.subscriber_id = $1
    GROUP BY c.id, c.uuid, c.name, c.subject
    ORDER BY last_viewed_at DESC
),
clicks AS (
    SELECT
        l.id as link_id,
        l.url,
        c.id as campaign_id,
        c.uuid as campaign_uuid,
        c.name as campaign_name,
        c.subject as campaign_subject,
        COUNT(*) as click_count,
        MAX(lc.created) as last_clicked_at
    FROM link_clicks lc
    LEFT JOIN links l ON l.id = lc.link_id
    LEFT JOIN campaigns c ON c.id = lc.campaign_id
    WHERE lc.subscriber_id = $1
    GROUP BY l.id, l.url, c.id, c.uuid, c.name, c.subject
    ORDER BY last_clicked_at DESC
)
SELECT
    COALESCE((SELECT JSON_AGG(v) FROM views v), '[]') as campaign_views,
    COALESCE((SELECT JSON_AGG(c) FROM clicks c), '[]') as link_clicks;
