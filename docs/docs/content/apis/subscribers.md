# API / Subscribers

Subscriber **identifiers** in URLs and JSON are **PocketBase record ids** (the `subscribers.id` string returned as `id` on subscriber objects), not numeric SQLite rowids. Bulk actions use the repeated query parameter `subscriber_record_id` or the JSON field `subscriber_record_ids`.

| Method | Endpoint                                                                                | Description                                    |
| ------ | --------------------------------------------------------------------------------------- | ---------------------------------------------- |
| GET    | [/api/subscribers](#get-apisubscribers)                                                 | Query and retrieve subscribers.                |
| GET    | [/api/subscribers/{id}](#get-apisubscribersid)                    | Retrieve a specific subscriber.                |
| GET    | [/api/subscribers/{id}/export](#get-apisubscribersidexport)       | Export a specific subscriber.                  |
| GET    | [/api/subscribers/{id}/bounces](#get-apisubscribersidbounces)     | Retrieve a  subscriber bounce records.         |
| GET    | [/api/inbound-email-replies/{replyId}/attachments](#get-apiinbound-email-repliesreplyidattachments) | List inbound email attachments for a timeline email reply event. |
| GET    | [/api/inbound-email-attachments/{id}/download](#get-apiinbound-email-attachmentsiddownload) | Download an inbound email attachment file. |
| POST   | [/api/subscribers](#post-apisubscribers)                                                | Create a new subscriber.                       |
| POST   | [/api/subscribers/bulk-add](#post-apisubscribersbulk-add)                               | Bulk upsert subscribers from JSON contacts.    |
| POST   | [/api/subscribers/{id}/optin](#post-apisubscribersidoptin)        | Sends optin confirmation email to subscribers. |
| POST   | [/api/public/subscription](#post-apipublicsubscription)                                 | Create a public subscription.                  |
| PUT    | [/api/subscribers/lists](#put-apisubscriberslists)                                      | Modify subscriber list memberships.            |
| PUT    | [/api/subscribers/bulk-update](#put-apisubscribersbulk-update)                          | Bulk tag/list updates for existing contacts.   |
| PUT    | [/api/subscribers/{id}](#put-apisubscribersid)                    | Update a specific subscriber.                  |
| PUT    | [/api/subscribers/{id}/blocklist](#put-apisubscribersidblocklist) | Blocklist a specific subscriber.               |
| PUT    | [/api/subscribers/blocklist](#put-apisubscribersblocklist)                              | Blocklist one or many subscribers.             |
| PUT    | [/api/subscribers/query/blocklist](#put-apisubscribersqueryblocklist)                   | Blocklist subscribers based on SQL expression. |
| DELETE | [/api/subscribers/{id}](#delete-apisubscribersid)                 | Delete a specific subscriber.                  |
| DELETE | [/api/subscribers/{id}/bounces](#delete-apisubscribersidbounces)  | Delete a specific subscriber's bounce records. |
| DELETE | [/api/subscribers](#delete-apisubscribers)                                              | Delete one or more subscribers.                |
| POST   | [/api/subscribers/query/delete](#post-apisubscribersquerydelete)                        | Delete subscribers based on SQL expression.    |

______________________________________________________________________

#### GET /api/subscribers

Retrieve all subscribers.

##### Query parameters

| Name                | Type   | Required | Description                                                           |
| :------------------ | :----- | :------- | :-------------------------------------------------------------------- |
| query               | string |          | Subscriber search by SQL expression.                                  |
| list_record_id      | string |          | PocketBase list record id to filter by. Repeat the query key for multiple lists. |
| subscription_status | string |          | Subscription status to filter by if there are one or more `list_id`s. |
| order_by            | string |          | Result sorting field. Options: name, status, created_at, updated_at, id (PocketBase record id).  |
| order               | string |          | Sorting order: ASC for ascending, DESC for descending.                |
| page                | number |          | Page number for paginated results.                                    |
| per_page            | number |          | Results per page. Set as 'all' for all results.                       |

##### Example Request

```shell
curl -u 'api_username:access_token' 'http://localhost:9000/api/subscribers?page=1&per_page=100' 
```

```shell
curl -u 'api_username:access_token' 'http://localhost:9000/api/subscribers?list_record_id=pbc_list_default_001&list_record_id=pbc_list_second_002&page=1&per_page=100'
```

```shell
curl -u 'api_username:access_token' -X GET 'http://localhost:9000/api/subscribers' \
    --url-query 'page=1' \
    --url-query 'per_page=100' \
    --url-query "query=subscribers.name LIKE 'Test%' AND subscribers.attribs->>'city' = 'Bengaluru'"
```

##### Example Response

```json
{
    "data": {
        "results": [
            {
                "id": "pbc_subscriber_john_001",
                "created_at": "2020-02-10T23:07:16.199433+01:00",
                "updated_at": "2020-02-10T23:07:16.199433+01:00",
                "uuid": "ea06b2e7-4b08-4697-bcfc-2a5c6dde8f1c",
                "email": "john@example.com",
                "name": "John Doe",
                "attribs": {
                    "city": "Bengaluru",
                    "good": true,
                    "type": "known"
                },
                "status": "enabled",
                "lists": [
                    {
                        "subscription_status": "unconfirmed",
                        "id": "pbc_list_default_001",
                        "uuid": "ce13e971-c2ed-4069-bd0c-240e9a9f56f9",
                        "name": "Default list",
                        "type": "public",
                        "tags": [
                            "test"
                        ],
                        "created_at": "2020-02-10T23:07:16.194843+01:00",
                        "updated_at": "2020-02-10T23:07:16.194843+01:00"
                    }
                ]
            },
            {
                "id": "pbc_subscriber_quadri_002",
                "created_at": "2020-02-18T21:10:17.218979+01:00",
                "updated_at": "2020-02-18T21:10:17.218979+01:00",
                "uuid": "ccf66172-f87f-4509-b7af-e8716f739860",
                "email": "quadri@example.com",
                "name": "quadri",
                "attribs": {},
                "status": "enabled",
                "lists": [
                    {
                        "subscription_status": "unconfirmed",
                        "id": "pbc_list_default_001",
                        "uuid": "ce13e971-c2ed-4069-bd0c-240e9a9f56f9",
                        "name": "Default list",
                        "type": "public",
                        "tags": [
                            "test"
                        ],
                        "created_at": "2020-02-10T23:07:16.194843+01:00",
                        "updated_at": "2020-02-10T23:07:16.194843+01:00"
                    }
                ]
            },
            {
                "id": "pbc_subscriber_sugar_003",
                "created_at": "2020-02-19T19:10:49.36636+01:00",
                "updated_at": "2020-02-19T19:10:49.36636+01:00",
                "uuid": "5d940585-3cc8-4add-b9c5-76efba3c6edd",
                "email": "sugar@example.com",
                "name": "sugar",
                "attribs": {},
                "status": "enabled",
                "lists": []
            }
        ],
        "query": "",
        "total": 3,
        "per_page": 20,
        "page": 1
    }
}
```

______________________________________________________________________

#### GET /api/subscribers/{id}

Retrieve a specific subscriber.

##### Parameters

| Name          | Type   | Required | Description      |
| :------------ | :----- | :------- | :--------------- |
| id | string | Yes      | Subscriber's PocketBase record id (same value as `id` in JSON responses). |

##### Example Request

```shell
curl -u 'api_username:access_token' 'http://localhost:9000/api/subscribers/pbc_subscriber_john_001'
```

##### Example Response

```json
{
    "data": {
        "id": "pbc_subscriber_john_001",
        "created_at": "2020-02-10T23:07:16.199433+01:00",
        "updated_at": "2020-02-10T23:07:16.199433+01:00",
        "uuid": "ea06b2e7-4b08-4697-bcfc-2a5c6dde8f1c",
        "email": "john@example.com",
        "name": "John Doe",
        "attribs": {
            "city": "Bengaluru",
            "good": true,
            "type": "known"
        },
        "status": "enabled",
        "lists": [
            {
                "subscription_status": "unconfirmed",
                "id": "pbc_list_default_001",
                "uuid": "ce13e971-c2ed-4069-bd0c-240e9a9f56f9",
                "name": "Default list",
                "type": "public",
                "tags": [
                    "test"
                ],
                "created_at": "2020-02-10T23:07:16.194843+01:00",
                "updated_at": "2020-02-10T23:07:16.194843+01:00"
            }
        ]
    }
}
```
______________________________________________________________________

#### GET /api/subscribers/{id}/export

Export a specific subscriber data that gives profile, list subscriptions, campaign views and link clicks information. Names of private lists are replaced with "Private list". 

##### Parameters

| Name          | Type   | Required | Description      |
| :------------ | :----- | :------- | :--------------- |
| id | string | Yes      | Subscriber's PocketBase record id. |

##### Example Request

```shell
curl -u 'api_username:access_token' 'http://localhost:9000/api/subscribers/pbc_subscriber_john_001/export'
```

##### Example Response

```json
{
  "profile": [
    {
      "id": "pbc_subscriber_john_001",
      "uuid": "c2cc0b31-b485-4d72-8ce8-b47081beadec",
      "email": "john@example.com",
      "name": "John Doe",
      "attribs": {
        "city": "Bengaluru",
        "good": true,
        "type": "known"
      },
      "status": "enabled",
      "created_at": "2024-07-29T11:01:31.478677+05:30",
      "updated_at": "2024-07-29T11:01:31.478677+05:30"
    }
  ],
  "subscriptions": [
    {
      "subscription_status": "unconfirmed",
      "name": "Private list",
      "type": "private",
      "created_at": "2024-07-29T11:01:31.478677+05:30"
    }
  ],
  "campaign_views": [],
  "link_clicks": []
}
```
______________________________________________________________________

#### GET /api/subscribers/{id}/bounces

Get a specific subscriber bounce records.
##### Parameters

| Name          | Type   | Required | Description      |
| :------------ | :----- | :------- | :--------------- |
| id | string | Yes      | Subscriber's PocketBase record id. |

##### Example Request

```shell
curl -u 'api_username:access_token' 'http://localhost:9000/api/subscribers/pbc_subscriber_john_001/bounces'
```

##### Example Response

```json
{
  "data": [
    {
      "id": 841706,
      "type": "hard",
      "source": "demo",
      "meta": {
        "some": "parameter"
      },
      "created_at": "2024-08-22T09:05:12.862877Z",
      "email": "thomas.hobbes@example.com",
      "subscriber_uuid": "137c0d83-8de6-44e2-a55f-d4238ab21969",
      "subscriber_id": "pbc_subscriber_thomas_099",
      "campaign": {
        "id": "pbc_campaign_welcome_002",
        "name": "Welcome to List Pocket"
      }
    },
    {
      "id": 841680,
      "type": "hard",
      "source": "demo",
      "meta": {
        "some": "parameter"
      },
      "created_at": "2024-08-19T14:07:53.141917Z",
      "email": "thomas.hobbes@example.com",
      "subscriber_uuid": "137c0d83-8de6-44e2-a55f-d4238ab21969",
      "subscriber_id": "pbc_subscriber_thomas_099",
      "campaign": {
        "id": "pbc_campaign_test_001",
        "name": "Test campaign"
      }
    }
  ]
}
```

______________________________________________________________________

#### GET /api/inbound-email-replies/{replyId}/attachments

List attachment records associated with an inbound email reply timeline entry.

##### Parameters

| Name    | Type   | Required | Description |
| :------ | :----- | :------- | :---------- |
| replyId | string | Yes      | Inbound email reply PocketBase record id (available as `metadata.inbound_email_reply_id` on timeline `inbound_email_reply` events). |

##### Example Request

```shell
curl -u 'api_username:access_token' 'http://localhost:9000/api/inbound-email-replies/pbc_inreply_001/attachments'
```

##### Example Response

```json
{
  "data": [
    {
      "id": "pbc_inattach_001",
      "reply_id": "pbc_inreply_001",
      "original_name": "invoice.pdf",
      "content_type": "application/pdf",
      "size_bytes": 184322,
      "file_name": "invoice_abcd1234.pdf",
      "download_url": "/api/inbound-email-attachments/pbc_inattach_001/download",
      "created": "2026-04-20 14:18:39.138Z"
    }
  ]
}
```

______________________________________________________________________

#### GET /api/inbound-email-attachments/{id}/download

Download a single inbound email attachment file.

##### Parameters

| Name | Type   | Required | Description |
| :--- | :----- | :------- | :---------- |
| id   | string | Yes      | Attachment PocketBase record id from `/api/inbound-email-replies/{replyId}/attachments`. |

##### Example Request

```shell
curl -u 'api_username:access_token' -L 'http://localhost:9000/api/inbound-email-attachments/pbc_inattach_001/download' -o invoice.pdf
```

##### Example Response

`307 Temporary Redirect` to PocketBase file API, followed by the binary file response.

______________________________________________________________________

#### POST /api/subscribers

Create a new subscriber.

##### Parameters

| Name                     | Type       | Required | Description                                                                                                                   |
| :----------------------- | :--------- | :------- | :---------------------------------------------------------------------------------------------------------------------------- |
| email                    | string     | Yes      | Subscriber's email address.                                                                                                   |
| name                     | string     | Yes      | Subscriber's name.                                                                                                            |
| status                   | string     | Yes      | Subscriber's status: `enabled`, `blocklisted`.                                                                                |
| lists                    | number\[\] |          | Legacy list rowids to subscribe to (prefer `list_record_ids`).                                                                 |
| list_record_ids          | string\[\] |          | PocketBase list record ids to subscribe to.                                                                                    |
| attribs                  | JSON       |          | Optional JSON object attributes for the subscriber that can be used in message templates. Example `{"location": "Somewhere"}` |
| preconfirm_subscriptions | bool       |          | If true, subscriptions are marked as confirmed and no-optin emails are sent for double opt-in lists.                          |

##### Example Request

```shell
curl -u 'api_username:access_token' 'http://localhost:9000/api/subscribers' -H 'Content-Type: application/json' \
    --data '{"email":"subscriber@domain.com","name":"The Subscriber","status":"enabled","list_record_ids":["pbc_list_default_001"],"attribs":{"city":"Bengaluru","projects":3,"stack":{"languages":["go","python"]}}}'
```

##### Example Response

```json
{
  "data": {
    "id": "pbc_subscriber_new_003",
    "created_at": "2019-07-03T12:17:29.735507+05:30",
    "updated_at": "2019-07-03T12:17:29.735507+05:30",
    "uuid": "eb420c55-4cfb-4972-92ba-c93c34ba475d",
    "email": "subscriber@domain.com",
    "name": "The Subscriber",
    "attribs": {
      "city": "Bengaluru",
      "projects": 3,
      "stack": { "languages": ["go", "python"] }
    },
    "status": "enabled",
    "lists": []
  }
}
```

______________________________________________________________________

#### POST /api/subscribers/{id}/optin

Sends optin confirmation email to subscribers.

##### Example Request

```shell
curl -u 'api_username:access_token' 'http://localhost:9000/api/subscribers/pbc_subscriber_john_001/optin' -H 'Content-Type: application/json' \
--data {}
```

##### Example Response

```json
{
    "data": true
} 
```
______________________________________________________________________

#### POST /api/public/subscription

Create a public subscription, accepts both form encoded or JSON encoded body.

##### Parameters

| Name       | Type       | Required | Description                 |
| :--------- | :--------- | :------- | :-------------------------- |
| email      | string     | Yes      | Subscriber's email address. |
| name       | string     |          | Subscriber's name.          |
| list_uuids | string\[\] | Yes      | List of list UUIDs.         |

##### Example JSON Request

```shell
curl 'http://localhost:9000/api/public/subscription' -H 'Content-Type: application/json' \
    --data '{"email":"subscriber@domain.com","name":"The Subscriber","list_uuids": ["eb420c55-4cfb-4972-92ba-c93c34ba475d", "0c554cfb-eb42-4972-92ba-c93c34ba475d"]}'
```

##### Example Form Request

```shell
curl 'http://localhost:9000/api/public/subscription' \
    -d 'email=subscriber@domain.com' -d 'name=The Subscriber' -d 'l=eb420c55-4cfb-4972-92ba-c93c34ba475d' -d 'l=0c554cfb-eb42-4972-92ba-c93c34ba475d'
```

Note: For form request, use `l` for multiple lists instead of `lists`.

##### Example Response

```json
{
  "data": true
}
```

______________________________________________________________________

#### PUT /api/subscribers/lists

Modify subscriber list memberships.

##### Parameters

| Name                    | Type       | Required           | Description                                                       |
| :---------------------- | :--------- | :----------------- | :---------------------------------------------------------------- |
| subscriber_record_ids   | string\[\] | Yes                | PocketBase subscriber record ids to modify.                      |
| action                  | string     | Yes                | Action to be applied: `add`, `remove`, or `unsubscribe`.          |
| target_list_record_ids  | string\[\] | Yes                | PocketBase list record ids to add/remove/unsubscribe from.       |
| status                  | string     | Required for `add` | Subscription status: `confirmed`, `unconfirmed`, or `unsubscribed`. |

##### Example Request

```shell
curl -u 'api_username:access_token' -X PUT 'http://localhost:9000/api/subscribers/lists' \
-H 'Content-Type: application/json' \
--data-raw '{"subscriber_record_ids": ["pbc_subscriber_john_001", "pbc_subscriber_quadri_002"], "action": "add", "target_list_record_ids": ["pbc_list_a_004", "pbc_list_b_005"], "status": "confirmed"}'
```

##### Example Response

```json
{
    "data": true
} 
```

______________________________________________________________________

#### PUT /api/subscribers/bulk-update

Apply tag and list operations to **existing** subscribers identified by email. Unknown emails are skipped and counted in the response; the request does not fail when some emails are missing.

Requires permission: `subscribers:manage`.

Maximum batch size: **5,000** email addresses per request.

List record ids in `list_remove` and `list_update` are filtered against the authenticated user's list manage permissions.

##### Parameters

| Name                 | Type       | Required | Description                                                                 |
| :------------------- | :--------- | :------- | :-------------------------------------------------------------------------- |
| contacts             | string\[\] | Yes      | Subscriber email addresses.                                                   |
| tags_add             | string\[\] |          | Tags to merge into each matched subscriber's `attribs.tags`.                |
| tags_remove          | string\[\] |          | Tags to remove from each matched subscriber (case-insensitive).            |
| list_remove          | string\[\] |          | PocketBase list record ids to remove matched subscribers from.              |
| list_update          | string\[\] |          | PocketBase list record ids to add or update membership on.                  |
| subscription_status  | string     |          | Status for `list_update` (`confirmed`, `unconfirmed`, `unsubscribed`). Default: `unconfirmed`. |

At least one of `tags_add`, `tags_remove`, `list_remove`, or `list_update` must be provided.

##### Response fields

| Name            | Type   | Description                                                       |
| :-------------- | :----- | :---------------------------------------------------------------- |
| ok              | bool   | Always `true` on success.                                         |
| matched         | number | Subscribers found and processed.                                  |
| skipped         | number | Email addresses with no matching subscriber.                      |
| tags_updated    | number | Subscribers whose tags were updated.                              |
| lists_removed   | number | List membership rows removed.                                     |
| lists_updated   | number | List membership rows inserted or updated.                           |

##### Example Request

```shell
curl -u 'api_username:access_token' -X PUT 'http://localhost:9000/api/subscribers/bulk-update' \
-H 'Content-Type: application/json' \
--data-raw '{"contacts":["john@example.com","missing@example.com"],"tags_add":["vip"],"tags_remove":["old-tag"],"list_update":["pbc_list_a_004"],"list_remove":["pbc_list_b_005"],"subscription_status":"confirmed"}'
```

##### Example Response

```json
{
  "data": {
    "ok": true,
    "matched": 1,
    "skipped": 1,
    "tags_updated": 1,
    "lists_removed": 1,
    "lists_updated": 1
  }
}
```

______________________________________________________________________

#### POST /api/subscribers/bulk-add

Bulk upsert subscribers from JSON contact objects. Behavior is similar to [CSV import](import.md), but runs synchronously and accepts structured JSON instead of a file upload. Creates new subscribers or updates existing ones matched by email.

Requires permission: `subscribers:import`.

Maximum batch size: **5,000** contacts per request.

List record ids in `list_remove` and `list_update` are filtered against the authenticated user's list manage permissions. Updating an existing subscriber requires access to at least one of that subscriber's lists (same as other subscriber manage APIs).

##### Parameters

| Name                 | Type       | Required | Description                                                                 |
| :------------------- | :--------- | :------- | :-------------------------------------------------------------------------- |
| contacts             | object\[\] | Yes      | Contact objects (see table below).                                          |
| tags_add             | string\[\] |          | Tags merged into each contact after upsert.                                 |
| tags_remove          | string\[\] |          | Tags removed from each contact after upsert.                                |
| list_remove          | string\[\] |          | PocketBase list record ids to remove each contact from.                     |
| list_update          | string\[\] |          | PocketBase list record ids to add or update membership on.                  |
| subscription_status  | string     |          | Status for `list_update`. Default: `unconfirmed`.                           |
| override_details     | bool       |          | When `true`, overwrite profile fields on email conflict. Default: `false`.  |

##### Contact object fields

Each item in `contacts` supports the following fields:

| Name        | Type   | Required | Description                                                                 |
| :---------- | :----- | :------- | :-------------------------------------------------------------------------- |
| email       | string | Yes      | Subscriber email address.                                                   |
| phone       | string |          | Phone number.                                                               |
| name        | string |          | Full name. Derived from email local-part if omitted on create.              |
| first_name  | string |          | First name.                                                                 |
| last_name   | string |          | Last name.                                                                  |
| attribs     | JSON   |          | Custom attributes stored on the subscriber.                                 |
| attributes  | JSON   |          | Alias for `attribs` (same as CSV import column name).                       |

When `override_details` is `false` and the email already exists, existing `phone`, name fields, and `attribs` are preserved. When `true`, incoming contact fields replace those values on conflict.

Global `tags_add`, `tags_remove`, `list_update`, and `list_remove` are applied to every contact in the batch after each upsert.

##### Response fields

| Name            | Type   | Description                                                       |
| :-------------- | :----- | :---------------------------------------------------------------- |
| ok              | bool   | Always `true` on success.                                         |
| matched         | number | Contacts successfully upserted (`created` + `updated`).           |
| skipped         | number | Invalid or empty contact entries skipped during processing.       |
| created         | number | New subscribers inserted.                                         |
| updated         | number | Existing subscribers matched by email.                            |
| tags_updated    | number | Contacts whose tags were updated.                                 |
| lists_removed   | number | List membership rows removed.                                     |
| lists_updated   | number | List membership rows inserted or updated.                           |

##### Example Request

```shell
curl -u 'api_username:access_token' -X POST 'http://localhost:9000/api/subscribers/bulk-add' \
-H 'Content-Type: application/json' \
--data-raw '{"contacts":[{"email":"john@example.com","first_name":"John","last_name":"Doe","attribs":{"city":"Berlin"}}],"tags_add":["vip"],"list_update":["pbc_list_a_004"],"subscription_status":"confirmed","override_details":false}'
```

##### Example Response

```json
{
  "data": {
    "ok": true,
    "matched": 1,
    "skipped": 0,
    "created": 1,
    "updated": 0,
    "tags_updated": 1,
    "lists_removed": 0,
    "lists_updated": 1
  }
}
```

______________________________________________________________________

#### PUT /api/subscribers/{id}

Update a specific subscriber. The `{id}` path segment is the subscriber's PocketBase record id (same as JSON `id`).

> Refer to parameters from [POST /api/subscribers](#post-apisubscribers). Note: All parameters must be set, if not, the subscriber will be removed from all previously assigned lists.

______________________________________________________________________

#### PUT /api/subscribers/{id}/blocklist

Blocklist a specific subscriber.

##### Parameters

| Name          | Type   | Required | Description      |
| :------------ | :----- | :------- | :--------------- |
| id | string | Yes      | Subscriber's PocketBase record id. |

##### Example Request

```shell
curl -u 'api_username:access_token' -X PUT 'http://localhost:9000/api/subscribers/pbc_subscriber_john_001/blocklist'
```

##### Example Response

```json
{
    "data": true
} 
```

______________________________________________________________________

#### PUT /api/subscribers/blocklist

Blocklist multiple subscriber.

##### Parameters

| Name                    | Type       | Required | Description                          |
| :---------------------- | :--------- | :------- | :----------------------------------- |
| subscriber_record_ids   | string\[\] | Yes      | PocketBase subscriber record ids.    |

##### Example Request

```shell
curl -u 'api_username:access_token' -X PUT 'http://localhost:9000/api/subscribers/blocklist' -H 'Content-Type: application/json' --data-raw '{"subscriber_record_ids":["pbc_subscriber_quadri_002","pbc_subscriber_john_001"]}'
```

##### Example Response

```json
{
    "data": true
} 
```

______________________________________________________________________

#### PUT /api/subscribers/query/blocklist

Blocklist subscribers based on SQL expression.

> Refer to the [querying and segmentation](../querying-and-segmentation.md#querying-and-segmenting-subscribers) section for more information on how to query subscribers with SQL expressions.

##### Parameters

| Name     | Type     | Required | Description                                  |
| :------- | :------- | :------- | :------------------------------------------- |
| query    | string   | Yes      | SQL expression to filter subscribers with.   |
| list_record_ids | string[] | No       | Optional PocketBase list record ids to limit the filtering to. |

##### Example Request

```shell
curl -u 'api_username:access_token' -X PUT 'http://localhost:9000/api/subscribers/query/blocklist' \
-H 'Content-Type: application/json' \
--data-raw '{"query":"subscribers.name LIKE \'John Doe\' AND subscribers.attribs->>'\''city'\'' = '\''Bengaluru'\''"}'
```

##### Example Response

```json
{
    "data": true
}
```

______________________________________________________________________

#### DELETE /api/subscribers/{id}

Delete a specific subscriber.

##### Parameters

| Name          | Type   | Required | Description      |
| :------------ | :----- | :------- | :--------------- |
| id | string | Yes      | Subscriber's PocketBase record id. |

##### Example Request

```shell
curl -u 'api_username:access_token' -X DELETE 'http://localhost:9000/api/subscribers/pbc_subscriber_john_001'
```

##### Example Response

```json
{
    "data": true
}
```

______________________________________________________________________

#### DELETE /api/subscribers/{id}/bounces

Delete a subscriber's bounce records

##### Parameters

| Name | Type   | Required | Description      |
| :--- | :----- | :------- | :--------------- |
| id   | string | Yes      | Subscriber's PocketBase record id. |

##### Example Request

```shell
curl -u 'api_username:access_token' -X DELETE 'http://localhost:9000/api/subscribers/pbc_subscriber_john_001/bounces'
```

##### Example Response

```json
{
    "data": true
}
```

______________________________________________________________________

#### DELETE /api/subscribers

Delete one or more subscribers.

##### Parameters

| Name                    | Type       | Required | Description                                                |
| :---------------------- | :--------- | :------- | :--------------------------------------------------------- |
| subscriber_record_id    | string     | Yes      | Repeat this query key once per subscriber to delete.     |

##### Example Request

```shell
curl -u 'api_username:access_token' -X DELETE 'http://localhost:9000/api/subscribers?subscriber_record_id=pbc_subscriber_john_001&subscriber_record_id=pbc_subscriber_quadri_002'
```

##### Example Response

```json
{
    "data": true
}
```

______________________________________________________________________

#### POST /api/subscribers/query/delete

Delete subscribers based on SQL expression.

##### Parameters

| Name     | Type     | Required | Description                                                        |
| :------- | :------- | :------- | :----------------------------------------------------------------- |
| query    | string   | No       | SQL expression to filter subscribers with.                         |
| list_record_ids | string[] | No       | Optional PocketBase list record ids to limit the filtering to.     |
| all      | bool     | No       | When set to `true`, ignores any query and deletes all subscribers. |


##### Example Request

```shell
curl -u 'api_username:access_token' -X POST 'http://localhost:9000/api/subscribers/query/delete' \
-H 'Content-Type: application/json' \
--data-raw '{"query":"subscribers.name LIKE \'John Doe\' AND subscribers.attribs->>'\''city'\'' = '\''Bengaluru'\''"}'
```

##### Example Response

```json
{
    "data": true
}
```
