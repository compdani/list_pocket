# Context-aware assistant

Open the sparkle button in the application header. The sidebar stays open while
navigating. Each turn captures the current route, filters, editor source, selected
text or component, and a revision of the unsaved draft.

Configure `LISTPOCKET_AI_API_KEY` and optionally `LISTPOCKET_AI_BASE_URL` using the
existing AI builder setup. The assistant uses the model and timeout in AI builder
settings. The provider must support streaming Chat Completions and function calls;
reference PDFs and images additionally require a model supporting those inputs.
No application data is sent to a provider before a user submits a message.

## Tools and review

Read tools call existing application handlers with the authenticated user's
permissions and list restrictions. They cover campaigns, analytics, lists,
subscribers, templates, media, inbox messages, workflow graphs and recent runs.
Queries return bounded results; the assistant cannot issue arbitrary SQL or HTTP
requests. Analytics tools require a campaign ID, metric and date range.

Campaign and workflow edits appear as proposals. Applying an editor proposal
changes the unsaved editor only. Text replacements must match exactly once;
visual block replacements use stable block IDs. A changed document invalidates
its previous proposal. The assistant offers undo until the next edit or turn.

Creating campaigns saves drafts after approval. Sending, scheduling, workflow
execution and publication remain in their existing UI flows. Workflow graph
writes validate and commit atomically. Editing a published workflow creates a
separate draft copy, preserving its current execution behavior.

## Storage and connections

Conversations are stored in this browser, scoped to the authenticated account.
Use **Import previous campaign chats** to claim the legacy browser history for
one account; original records are retained, including their picture references.
Reference files remain staged for follow-up turns during the current page session;
reattach files after reloading. File contents are not stored in localStorage.

Turn events and pending approvals are held in server memory for up to an hour
after completion. A server restart invalidates pending proposals; request a fresh
turn. SSE reconnects resume from the last event ID without rerunning tools. There
are four concurrent model workers, and only one active turn per conversation.
A failed server mutation is never retried through the same approval, because a
handler can fail after committing a write; inspect saved data before retrying.

## Validation

Run `go test ./cmd ./internal/workflow`, `npm --prefix frontend run test:unit`, and
`npm --prefix frontend run build`. The focused Cypress scenarios are in
`frontend/cypress/e2e/assistant.cy.js`; they use a standalone fixture and mocked
provider events, without resetting the application database.
