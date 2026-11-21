# Context-aware assistant

Open the sparkle button in the application header for the side panel, or visit
`/admin/agent` for the full Agent page. The sidebar stays open while navigating.
Each side-panel turn captures the current route, filters, open editor source,
selected text or component, media-library pictures, and a revision of the
unsaved draft. The Agent page adds `@` context references (contacts,
conversations, campaigns, lists, templates, workflows and runs) and pasted data.

Configure `LISTPOCKET_AI_API_KEY` (and optionally `LISTPOCKET_AI_BASE_URL`) or
sign in with ChatGPT under **Settings → AI**. The provider must support
streaming responses and tool calls; reference PDFs and images additionally
require a model supporting those inputs. No application data is sent to a
provider before a user submits a message.

## Tools and review

Read tools call existing application handlers with the authenticated user's
permissions and list restrictions. They cover campaigns, analytics, lists,
subscribers, templates, media, inbox messages, CRM boards/tasks/notes, workflow
graphs and recent runs. Queries return bounded results; the assistant cannot
issue arbitrary SQL or HTTP requests. Analytics tools require a campaign ID,
metric and date range. Plugin development tools are visible and callable only to
super admins.

Campaign, template and workflow edits appear as proposals. Applying an editor
proposal changes the unsaved editor only. Text replacements must match exactly
once; visual block replacements use stable block IDs. A changed document
invalidates its previous proposal (`stale`). Approving while a different or no
editor is open leaves the proposal pending until you return to the original
editor. The assistant offers undo until the next edit, turn, or conversation
switch.

Creating campaigns saves drafts after approval. Sending, scheduling, workflow
execution, plugin installation and publication all require approval first.
Workflow graph writes validate and commit atomically, and plugin proposals
snapshot the draft's files at proposal time.

## Storage and API

Both the side panel and the Agent page are backed by the `pocketbase-mcp` chat
module mounted at `/api/mcp/chat/v1` (alongside `/mcp` — see [mcp.md](mcp.md)).

- Conversations, messages, proposals and uploaded files live on the server,
  scoped to the signed-in user.
- Turns and pending proposals survive server restarts. SSE reconnects resume
  from the last sequence number without re-running tools.
- There are four concurrent model workers (global and per user), and at most one
  active turn per conversation (`409`).
- A handler returning `400` refuses the proposal before writing (`422 refused`)
  so it stays pending for edits; a failure after a write may have committed is
  recorded as `unknown` or `failed` and never retried automatically.

### Environment variables

- `LISTPOCKET_MCP_ALLOW_WRITES` — `true` or `1` offers the record-writing tools
  on `/mcp` as well, without a review step there. See [mcp.md](mcp.md).
- `LISTPOCKET_AI_MAX_OUTPUT_TOKENS` — output-token ceiling for chat turns
  (default `128000`); `0` leaves the parameter out.

## Validation

Run `go test ./cmd ./internal/migrations`, `npm --prefix frontend run test:unit`,
and `npm --prefix frontend run build`. Browser tests are in
`frontend/cypress/e2e/assistant-chat.cy.js` and
`frontend/cypress/e2e/agent-page-chat.cy.js`; they use standalone harnesses and
stub `/api/mcp/chat/v1/**`, without resetting the application database.
