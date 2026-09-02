# Performance

List Pocket is built to be highly performant and can handle millions of subscribers with minimal system resources.

As the **SQLite** database (managed by embedded PocketBase) grows—with a large number of subscribers, campaign views, and click records—counting records and aggregating statistics can slow down. For instance, loading admin pages that do these aggregations can take tens of seconds if the database has millions of subscribers.

- Aggregate counts and statistics on the landing dashboard.
- Subscriber count beside every list on the Lists page.

On installations with millions of subscribers, where those pages do not load instantly, turn on **Settings → Performance → Cache slow database queries**.

## Slow query caching

When this option is enabled, dashboard counts and list subscriber counts are no longer computed on every request. Instead they are stored as JSON blobs in `listpocket_settings` and refreshed on a crontab (default: `0 3 * * *`, 3 AM daily). Use [crontab.guru](https://crontab.guru) to generate an expression.

Dashboard **charts** stay live (they are timezone-dependent) and are aggregated in SQL.

Bulk subscriber import also refreshes the cache when it finishes.

## Maintenance (SQLite)

Running [`VACUUM`](https://www.sqlite.org/lang_vacuum.html) on large databases at regular intervals can reclaim space and keep the file compact. SQLite also benefits from [`ANALYZE`](https://www.sqlite.org/lang_analyze.html) to update query planner statistics. Schedule these during low-traffic windows; `VACUUM` can take an exclusive lock while it runs.
