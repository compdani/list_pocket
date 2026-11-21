# MCP endpoint

List Pocket serves MCP at `<root URL>/mcp` over streamable HTTP. The endpoint is
part of the List Pocket binary; there is no separate executable to build or run.
It exposes the same tools the in-app assistant uses, through the same
permission-checked handlers, so a call sees what the signed-in user sees in the
app: their role permissions and list scope apply.

## Sign-in

The endpoint accepts a List Pocket **users** token in the `Authorization`
header:

```
Authorization: Bearer <token>
```

Get one by signing in through the normal API:

```sh
curl -s https://<host>/mailapi/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"alex","password":"..."}'
```

The users token is `data.token` in the answer. If the account has TOTP enabled,
login answers `{"data":{"status":"twofa_required","token":"...","next":"/admin"}}`
instead; `next` is the page to open after sign-in. Post the code to finish the
sign-in, then read `data.token` from that answer:

```sh
curl -s https://<host>/mailapi/auth/twofa \
  -H 'Content-Type: application/json' \
  -d '{"token":"<token from login>","totp_code":"123456"}'
```

A missing or invalid token gets 401, and so does a PocketBase `_superusers`
token: only `users` tokens are accepted. Every request re-checks the token, and
each tool call runs as that user. Tokens expire; sign in again and update the
client's header.

**OAuth is off on purpose.** The module's OAuth sign-in checks only the account's
password. Turning it on now would let a user with List Pocket's own TOTP second
factor sign in with just their password. It will be enabled once the module can
run a second step.

## Client setup

Replace `<host>` with the application root URL (no `/mailapi`, no `/admin`) and
`<token>` with the users token. Use HTTPS for anything reachable from outside
the machine. These are examples; any MCP client that speaks streamable HTTP with
custom headers works.

### Claude Code

```sh
claude mcp add --transport http listpocket https://<host>/mcp \
  --header "Authorization: Bearer <token>"
```

### Cursor

In `~/.cursor/mcp.json`, or a project `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "listpocket": {
      "url": "https://<host>/mcp",
      "headers": {
        "Authorization": "Bearer <token>"
      }
    }
  }
}
```

### Clients that require OAuth for remote servers

Claude Desktop and ChatGPT custom connectors only connect to remote MCP servers
through OAuth, which this endpoint doesn't offer yet. Until it does, run a local
bridge such as [`mcp-remote`](https://www.npmjs.com/package/mcp-remote) and give
the bridge the header:

```json
{
  "mcpServers": {
    "listpocket": {
      "command": "npx",
      "args": [
        "mcp-remote",
        "https://<host>/mcp",
        "--header",
        "Authorization: Bearer ${LISTPOCKET_TOKEN}"
      ],
      "env": {
        "LISTPOCKET_TOKEN": "<token>"
      }
    }
  }
}
```

Keep the token in the client's private environment or secrets store. `mcp-remote`
expands `${LISTPOCKET_TOKEN}` from its environment.

## Tools

With writes off (the default), the endpoint offers these read tools:

| Area | Tool | Reads |
|---|---|---|
| Audience | `lists` | Accessible mailing lists and audience counts. |
| | `subscribers` | Permitted contacts. `search` matches name or email; `page` selects the page. |
| | `subscriber_activity` | One contact's campaign sends, views and link clicks. `id` is the contact. |
| | `subscriber_channels` | One contact's addresses and consent on plugin channels. `id` is the contact. |
| CRM | `contact_profile` | One contact's owner and company. `id` is the contact. |
| | `contact_notes` | One contact's notes. `id` is the contact. |
| | `contact_tasks` | One contact's tasks. `id` is the contact. |
| | `contact_boards` | Which boards a contact is on, and their stage on each. `id` is the contact. |
| | `crm_tasks` | Follow-up tasks across contacts. `status` is open or done, `assignee` is me or a user ID. |
| | `crm_assignees` | Who contacts and tasks can be assigned to. |
| | `boards_list` | Boards and their stages, with how many contacts are in each. |
| | `board_view` | One board's contacts by stage. `id` is the board key. |
| Campaigns and content | `campaigns` | Campaigns, or one with `id`, including audience exclusions and content. |
| | `analytics` | One campaign metric. `id` is the metric: `views`, `views_unique`, `views_raw`, `views_suspected`, `clicks`, `bounces`, `unsubscribes` or `links`. |
| | `templates` | Reusable campaign templates, or one with `id` including its body. |
| | `media` | Hosted media and image URLs. |
| Inbox | `inbox` | Permitted inbound email, or one with `id`. |
| | `conversations` | Inbox conversations. `view` is open, unread, closed or all. |
| | `conversation_thread` | One conversation's messages. `query` carries `key=<contact record ID, or channel:address>`. |
| Workflows | `workflow_schema` | How workflows are written: node types, expressions and triggers. |
| | `workflow_catalog` | Every tool and event a workflow can use, with input and output schemas. |
| | `workflows` | Workflows, or one with `id` (or `name`) including its draft, validation and published version. |
| | `workflow_runs` | Recent runs, or one with `id` including its steps. `status` and `workflow` filter. |
| | `workflow_approvals` | Approvals waiting for a person. `status` filters. |
| Dashboard | `dashboard_counts` | Dashboard totals: contacts, campaigns, messages and recent activity. |
| | `dashboard_charts` | Dashboard chart data for the last 30 days. `query` takes `tz`, an IANA zone, default UTC. |

Single-record tools take `id`, the PocketBase record ID from another tool's
result. List tools take `query`, a URL-encoded query string using the same
filters as the `/mailapi` routes:

```json
{"query": "search=alice&page=1&per_page=20"}
```

```json
{"id": "CONTACT_RECORD_ID"}
```

There is no SQL. `subscribers` refuses a SQL `query` field and takes `search`
instead. `analytics` also needs `id`, `from`, `to` and `tz=UTC` in `query`, with
the metric in the `id` argument. Each page returns at most 20 records; follow
`page` for more. A result over 256 KB is refused with a message to narrow the
query.

Results carry the handler's response under `data` and a `source` object:
`source.href` is the page to open in List Pocket, with `source.tool` and, for
tools that run a handler, `source.id` and `source.query`.

Plugin tools, `propose_editor_patch` and every proposal-style write are chat-only.
They never appear in `tools/list`, and calling one by name is refused.

## Optional writes

Set `LISTPOCKET_MCP_ALLOW_WRITES=true` (or `1`) on the List Pocket server to add
exactly five tools:

| Tool | Arguments |
|---|---|
| `propose_create_list` | `body`: `name`, `type` (`public` or `private`), `optin` (`single` or `double`); optional `description`. |
| `propose_create_contact` | `body`: email and/or phone; optional `name`, `status` (`enabled`, `disabled` or `blocklisted`), `lists` (list IDs) and `attribs`. Does not preconfirm memberships or send opt-in messages. |
| `propose_create_template` | `body`: `name`, `type` (`campaign` or `tx`), `subject`, `body`. Email campaign templates must contain `{{ template "content" . }}`. |
| `propose_contact_note` | `id` (contact) and `body`: `{"body": "note text"}`. |
| `propose_subscriber_update` | `id` (contact) and `body`: any of `email`, `phone`, `first_name`, `last_name`, `attribs`. Omitted fields are preserved; an empty phone or name string clears that field. `attribs` replaces the whole attributes object. |

Despite the names, these run immediately on MCP: there is no proposal or review
step in List Pocket. They call the same handlers as the chat proposal tools,
under the caller's permissions and list scope. Use the MCP client's own
tool-approval controls.

A failed write is never retried automatically, because it may have committed
before the failure. Read the record before trying again.

Never exposed on MCP, whatever the setting: sending or scheduling messages,
publishing or running workflows, installing or enabling plugins, and deleting
records.

## Migrating from the stdio server

`listpocket-mcp`, the separate stdio executable, has been removed. Move the
client to `https://<host>/mcp` with the `Authorization` header. Tool names
changed too:

| Old (stdio) | New (`/mcp`) |
|---|---|
| `list_lists` | `lists` |
| `get_list` | `lists` with `id` |
| `list_subscribers` | `subscribers` |
| `get_subscriber` | `subscribers` with `id` |
| `get_subscriber_activity` | `subscriber_activity` |
| `get_subscriber_channels` | `subscriber_channels` |
| `list_contact_notes` | `contact_notes` |
| `list_contact_tasks` | `contact_tasks` |
| `list_tasks` | `crm_tasks` |
| `list_boards` | `boards_list` |
| `get_board` | `board_view` |
| `list_campaigns` | `campaigns` |
| `get_campaign` | `campaigns` with `id` |
| `list_templates` | `templates` |
| `get_template` | `templates` with `id` |
| `list_media` | `media` |
| `list_conversations` | `conversations` |
| `get_conversation_thread` | `conversation_thread` |
| `list_inbound_emails` | `inbox` |
| `get_inbound_email` | `inbox` with `id` |
| `list_workflows` | `workflows` |
| `get_workflow` | `workflows` with `id` |
| `get_workflow_catalog` | `workflow_catalog` |
| `list_workflow_runs` | `workflow_runs` |
| `get_workflow_run` | `workflow_runs` with `id` |
| `get_dashboard_counts` | `dashboard_counts` |
| `get_dashboard_charts` | `dashboard_charts` |
| `create_list` (writes on) | `propose_create_list` |
| `create_subscriber` (writes on) | `propose_create_contact` |
| `create_template` (writes on) | `propose_create_template` |
| `create_contact_note` (writes on) | `propose_contact_note` |
| `update_subscriber` (writes on) | `propose_subscriber_update` |

Records are read with `id` and lists with `query`, so prompts, allowlists and
saved arguments that used the old per-tool filters need updating. Write tools
take their payload under `body`. Results now carry a `source` link back into the
app.

## Limits and operations

- Each tool call times out after 5 minutes. A timeout cancels the call and
  returns an error; a write is never retried after one.
- The endpoint has no built-in rate limit yet. Put per-IP limits at the reverse
  proxy.
- When the app's root URL is set, only that origin is accepted for calls made
  from a browser. Requests without an `Origin` header, which is normal for CLI
  clients, are unaffected. Without a root URL, the module's default (any origin)
  applies; the bearer token is still required.
- Each tool call writes one `mcp:` log line with the tool, channel, outcome,
  duration and user ID. Arguments and tokens are never logged.

## Verification

```sh
GOWORK=off go test -count=1 ./cmd/ -run MCP
GOWORK=off go test -count=1 ./cmd
```

The MCP tests start a List Pocket app in process and speak to `/mcp` with a real
client: discovery, read calls as an admin, a list-scoped user and a user without
the permission, the absence of write, plugin and proposal tools, the opt-in write
list, 401 for a missing token and for a `_superusers` token, and the tool error
mapping.
