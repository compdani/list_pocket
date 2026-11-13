# List Pocket MCP server

`listpocket-mcp` lets an MCP host use an existing List Pocket instance through
its permission-checked `/mailapi` routes. It is a separate Go executable using
the [official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk).
It communicates over **stdio**; it does not add an HTTP endpoint to List Pocket.
The host starts the executable and communicates through stdin/stdout. Diagnostics
go to stderr.

## Build and connect

From the repository root (Go version from `go.mod`):

```sh
go build -o listpocket-mcp ./cmd/listpocket-mcp
```

Start List Pocket normally. Obtain a **PocketBase users auth token**, such as
`data.token` from a successful `POST /mailapi/auth/login` with a JSON body
containing `username` and `password`. If the user has 2FA, complete the ordinary
2FA login flow first. Legacy BasicAuth/API secrets and PocketBase superuser
tokens are not substitutes for a users auth token.

Configure the MCP host with this stdio server definition, replacing the command
with the absolute path to the executable and the token with your user token:

```json
{
  "mcpServers": {
    "listpocket": {
      "command": "/absolute/path/to/listpocket-mcp",
      "args": [],
      "env": {
        "LISTPOCKET_MCP_URL": "http://127.0.0.1:9000",
        "LISTPOCKET_MCP_TOKEN": "YOUR_POCKETBASE_USER_TOKEN"
      }
    }
  }
}
```

The URL defaults to `http://127.0.0.1:9000`. Use the application base URL, without
`/mailapi`; a deployment subpath is supported. `-url` also sets the URL, with
`LISTPOCKET_MCP_URL` taking precedence. Use HTTPS for a hosted instance.

Keep the token in the host's private environment or secrets configuration.
Tokens expire; replace an expired token and restart the MCP server. The server
does not store passwords, log in automatically, or refresh tokens. All API
permissions and list access restrictions are enforced by List Pocket. Discovery
lists the supported tools; calling a tool still requires the relevant permission.

## Available tools

| Area | Read tools |
| --- | --- |
| Audience | `list_lists`, `get_list`, `list_subscribers`, `get_subscriber`, `get_subscriber_activity`, `get_subscriber_channels` |
| CRM | `list_contact_notes`, `list_contact_tasks`, `list_tasks`, `list_boards`, `get_board` |
| Campaigns and content | `list_campaigns`, `get_campaign`, `list_templates`, `get_template`, `list_media` |
| Inbox | `list_conversations`, `get_conversation_thread`, `list_inbound_emails`, `get_inbound_email` |
| Workflows | `list_workflows`, `get_workflow`, `get_workflow_catalog`, `list_workflow_runs`, `get_workflow_run` |
| Dashboard | `get_dashboard_counts`, `get_dashboard_charts` |

Single-record tools require `id`, the PocketBase record ID from another tool's
result. Read tools accept optional `query`, a URL-encoded query string using the
same filters as the HTTP API. Results preserve the API's JSON `data` envelope
and pagination metadata. Follow pagination when more results are needed.

Example tool arguments:

```json
{"query":"search=alice&page=1&per_page=20"}
```

```json
{"id":"CONTACT_RECORD_ID"}
```

For `get_conversation_thread`, pass `query` containing `key=` followed by the
URL-encoded conversation key returned by `list_conversations`.

## Optional writes

Add `"--allow-writes"` to the host definition's `args` to expose:

- `create_list`: `name`, `type` (`private` or `public`), `optin` (`single` or
  `double`); optional `description` and `tags`.
- `create_subscriber`: `status` (`enabled`, `disabled`, or `blocklisted`), email
  and/or phone; optional names, `list_record_ids`, and `attribs`. Does not
  preconfirm memberships or send opt-in messages.
- `create_template`: `name`, `type` (`campaign`, `campaign_sms`, or `tx`), `body`;
  optional `subject` and `channel`. Email campaign templates must contain
  `{{ template "content" . }}`.
- `create_contact_note`: contact `id` and note `body`.
- `update_subscriber`: contact `id` plus any of `email`, `phone`, `first_name`,
  `last_name`, and `attribs`. Omitted fields are preserved; an empty phone or
  name string clears that field. `attribs` replaces the entire attributes
  object (`{}` clears it). Uses `PATCH /mailapi/subscribers/{id}`, so it requires
  a List Pocket backend with this endpoint. Status, subscriptions, and consent
  are preserved, and no opt-in messages are sent.

These calls persist immediately, unlike the built-in assistant's proposals.
Use the MCP host's tool approval controls to review them. Failed requests are
never automatically retried: a timeout can occur after a write committed, so
read the affected records before retrying. Sending, scheduling, publishing,
deleting, and arbitrary HTTP requests are not exposed.

API failures become MCP tool errors. Requests time out after 30 seconds and
responses are limited to 8 MiB; narrow filters or paginate large results.
Redirects are rejected, so configure the final application URL.

## Verification

```sh
go test -race ./internal/mcpserver ./cmd/listpocket-mcp
```

Tests connect an official MCP client to the server and exercise discovery,
permission-error propagation, API requests, input validation, optional writes,
redirect handling, response limits, and token redaction against a mock HTTP API.
