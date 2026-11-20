# Migrating the assistant to Pocketbase-MCP chat

Status: draft, 2026-11-19. Covers moving the side panel (`AIAssistant.vue`) and the
Agent page (`Agent.vue`) from List Pocket's own chat code onto the chat module in
[Pocketbase-MCP](https://github.com/compdani/Pocketbase-MCP) (local:
`/Volumes/mini512/Components/Pocketbase-MCP`, at `3280275`, tag v0.1.5).

Requirement: feature parity, no regressions. Any behaviour change needs an explicit
decision (see [Decisions](#decisions)).

## Summary

**Update (v0.1.6, `origin/master` a31b29b).** The module now resolves every blocker
below (B1–B12) and the verify items V1–V5. The checks behind that:

- An outside module with `go get …/adapters/pocketbase@v0.1.6` and PocketBase v0.41.1
  builds with `GOWORK=off`.
- Root, adapter and browser-client tests pass (179 client tests).
- I read each hook in the code, not only the docs.

Three small gaps remain. They're listed under
[Remaining after v0.1.6](#remaining-after-v016). Phase 1 (List Pocket backend behind
the flag) can start.

The original assessment follows for the record.

Migration isn't possible yet. The module covers the core: durable turns, SSE replay,
proposals that run once, editor proposals, uploads, a provider catalog with ChatGPT
sign-in, reasoning effort and context-window management. But List Pocket depends on
about a dozen things the module can't do yet. Most are small, additive hooks in the
module (per-turn tool filtering, caller identity in `Setup`, proposal warnings,
proposal-time argument expansion, "refused, stays pending", stored references, history
import). Two are packaging and provider issues that block even a first build: the
submodules can't be consumed from another Go module, and the provider admin API is
restricted to `_superusers`.

Order: fix the module blockers and release, then port the backend behind a flag, then
the Agent page, then the side panel, then remove the legacy code.

## Remaining after v0.1.6

How each blocker was resolved, so List Pocket uses the right API:

| # | Resolved by |
|---|---|
| B1 | Submodule tags `adapters/pocketbase/v0.1.6` and `auth/stockoauth/v0.1.6`, with real requires. Checked: an outside build works. |
| B2 | `ChatConfig.ProviderStore` without `ManageProviders` (no admin routes), `srv.ChatRouter().Reload(ctx)`, and `catalog.CodexTokenStore` (the host owns sign-in and refresh; the router only asks for tokens). Also `catalog.Provider.MaxTokensField`. |
| B3 | `TurnSetup.Tools func(name) bool`. It can only hide tools, and hidden tools are refused on call too. |
| B4 | `TurnInput.Identity` and `pb.ResolveTurnCaller(ctx, app, in)`, which re-validates the token |
| B5 | `ChatOptions.StoreContext` (a `context` part on the user message, returned in the transcript and never sent as history) and `TurnInput.EarlierContexts` (at most 30, oldest first) |
| B6 | `ToolSpec.Warn` (stored on the proposal, shown and sent to the model) |
| B7 | `ToolSpec.Prepare` (runs once, and its result is the stored arguments) |
| B8 | `pbmcp.Refused(msg)` makes the decision answer 422, and the proposal goes back to `pending` at a new revision |
| B9 | `max_completion_tokens` plus one retry without it. `output_limit`, `content_filtered` and `empty_reply` fail the turn, on the Responses wire too |
| B10 | `chat.ImportConversation` (server) and `POST /conversations/actions/import` (browser, `Imports: true`). Idempotent per `SourceID`. |
| B11 | New failure codes `provider_auth`, `usage_limit` and `rate_limited`, plus `errorText` in the binding |
| B12 | `ParallelToolCalls` on `RunnerConfig` and `TurnSetup` |
| V1 | Fine: argument edits on custom server tools aren't restricted by target (`chat_bridge.go` `Revise`) |
| V2 | `pbmcp.Config.DisableMCPEndpoint` |
| V3 | `TitleModel: "first-message"` |
| V4 | `AcceptOtherImages` (BMP, TIFF, ICO, HEIC, AVIF; never SVG) |
| V5 | Still `DecisionTimeout`. Measure the slow approvals in phase 1. |

**Decided 2026-11-19: old chats aren't migrated.** Existing Agent page conversations
(`agent_*`) and side-panel threads in `localStorage` are deleted at cutover, with no
import. B10 is therefore not used, and the two import gaps found in the v0.1.6 review
(no `context` part on imported messages; browser import is text only) no longer
matter.

**Decided 2026-11-19: the host decides which references stay in view.**
`EarlierContexts` keeps the 30 most recent contexts within the history window the
runner reads (60 messages for us, via `MaxHistoryMessages`). `Setup` adds the
current message's references first, then fills the remaining slots from those
earlier contexts, newest first. List Pocket used to keep the first 30 distinct
references across up to 500 messages, so early references now drop out instead of
late ones.

No module work or decisions remain open.

Also for List Pocket's side, not the module's: `errorText` in the binding uses generic
wording ("…in the provider settings"). Our own binding should map `provider_auth` and
`usage_limit` to today's text ("sign in again under Settings → AI").

## What exists today

Two surfaces share one turn engine (`cmd/ai_assistant.go`):

| | Side panel | Agent page |
|---|---|---|
| Host | `components/AIAssistant.vue` | `views/Agent.vue` |
| Conversations | Browser `localStorage`, per account (`listpocket_assistant_v1:<id>`) | Server: `agent_conversations`, `agent_messages`, `agent_attachments` |
| History sent to the model | Client resends the last 60 messages each turn (text only) | Server rebuilds from stored messages (60), adding proposal outcomes |
| Turn state, events, proposals | In memory, 1 h retention, lost on restart | Same engine. Reply saved to `agent_messages.parts` when done |
| Proposal decisions | `POST /ai/assistant/turns/{id}/decision` (in memory) | `POST /agent/messages/{id}/parts/{i}/apply\|reject` (stored) |
| Attachments | Base64 inline every turn ("sticky" composer chips), images, PDF, text | Uploaded to `agent_attachments` (text 2 MB, binary 6 MB), referenced as `file` context |
| Context | Route, params, filters, editor snapshot + revision + selection, media pictures | `@` references (contact, conversation, campaign, list, template, workflow, run, file), stored per message, re-read fresh every turn into the system prompt |
| Editor tool | `propose_editor_patch` (text, block, graph ops), applied in browser, undo | Disabled (`NoEditor`) |
| Limits | AI builder timeout (≥180 s; 3600 s for super admins with plugins), 16 rounds | 3600 s, 16 rounds |
| Shared | 4 global workers, 1 active turn per conversation, 128k output-token ceiling with retry-without-limit, `parallel_tool_calls=false`, truncated replies count as failures, actionable provider errors | |

Tools (`assistantTools`, `assistantCRMTools`, `assistantFlowTools`,
`assistantPluginTools`): about 40. Reads run existing `/mailapi` handlers in process
through `a.auth.PermRE` with the caller's `auth.User`. Writes are proposals. Plugin
tools are super-admin only and hidden from everyone else's tool list. Results carry
`source.href` for "Open" links.

Providers (`cmd/ai_settings.go`, `internal/flow/llm`, `internal/flow/chatgpt`): an
OpenAI-compatible "api" provider from `LISTPOCKET_AI_API_KEY` /
`LISTPOCKET_AI_BASE_URL`, plus a ChatGPT plan sign-in. The default is in
`ai_settings`. Live model lists are cached for 10 minutes. Settings are gated by the
`settings:manage` role permission. Workflow AI steps and the campaign builder use the
same resolver.

Supporting endpoints that stay: `/agent/mentions`, `/agent/preview`,
`/agent/preview-proposal`, `/agent/models`.

## Blockers in Pocketbase-MCP

These must be added or changed in the module, and released, before List Pocket can
switch without a regression.

| # | Blocker | Evidence | Proposed module change |
|---|---|---|---|
| B1 | **Submodules can't be consumed.** `adapters/pocketbase` and `auth/stockoauth` require `github.com/compdani/pocketbase-mcp v0.0.0` and resolve it with relative `replace` paths. Go ignores a dependency's replaces, and only root tags exist (`v0.1.x`). A local `replace` in List Pocket would break the from-source Docker build. | `adapters/pocketbase/go.mod:45-47` | Tag `adapters/pocketbase/vX.Y.Z` and `auth/stockoauth/vX.Y.Z`. Require real versions, and keep the replaces only in a `go.work` used for development. |
| B2 | **Provider settings are superuser-only and separate.** The managed provider API needs a PocketBase `_superusers` token, but List Pocket admins are `users` with roles (`settings:manage`). The module also has its own Codex credential store (`codex.Credentials`), separate from List Pocket's `chatgpt.Store`, which would mean two keys and two ChatGPT sign-ins (already listed in the roadmap). | `docs/chat.md` §8, `chat/provider/catalog/config.go:264` | A documented `catalog.Store` path that works without `ManageProviders` and without the admin routes, plus an exported `Reload` (or change hook) so List Pocket can refresh the catalog when Settings → AI changes. Also a credentials source interface so the Codex provider can read tokens from the host (List Pocket's `chatgpt.Store.Token`) instead of its own table. |
| B3 | **No per-turn tool filtering.** The Agent page hides `propose_editor_patch`. `TurnSetup` can change the prompt and limits, not the tool set. | `chat/setup.go:37` | `TurnSetup.Tools func(name string) bool` (narrow-only, never widens). |
| B4 | **`Setup` doesn't know who the caller is.** The Agent page reads every attached reference as the user (through permission-checked handlers) and puts the results in the system prompt. `TurnInput` has `OwnerKey` but no token or principal, so `Setup` can't run handlers as the caller. | `chat/setup.go:13`, `cmd/agent.go:436` | Add the principal to `TurnInput`, with the same in-memory-only token rule, or a `ResolveCaller` equivalent usable from `Setup`. |
| B5 | **References aren't stored.** List Pocket stores each user message's references, shows them as chips (`contextInUse`), accumulates up to 30 across the conversation and re-reads them every turn. The module's turn `context` is per-turn and never stored. | `docs/chat.md` §3 "Page context" | Opt-in stored message metadata (for example `ChatOptions.StoreContext`), returned in the transcript, with the conversation's earlier contexts passed to `Setup`. |
| B6 | **Proposal warnings.** `validateFlowProposal` returns warnings that are shown on the card and sent back to the model. `ToolSpec.Validate` returns only an error. | `cmd/ai_assistant.go` runAssistant, `config.go:137` | A `Validate` variant (or `Warn`) that returns warnings, stored on the proposal and included in the tool result. |
| B7 | **Proposal-time argument expansion.** Plugin proposals snapshot the draft's files into the arguments (`expandAssistantProposal`), so the reviewer approves exactly those files. `Validate` must not have side effects or change arguments. | `cmd/ai_assistant_plugins.go:141` | `ToolSpec.Prepare(ctx, call, args) (args, error)`, run before validation and storage. |
| B8 | **"Refused before writing" must stay pending.** List Pocket puts a proposal back to `pending` when the handler answers 400, so the user can fix it in the review modal and apply again. The module maps any `ToolError` to terminal `failed`. | `cmd/ai_assistant.go` `applyRefused`, `chat/decide.go` `recordOutcome` | A distinct error (for example `chat.Refused`) that returns 422 with the message and leaves the proposal pending at a new revision. |
| B9 | **Output-token parameter.** The OpenAI adapter sends `max_tokens`. List Pocket sends `max_completion_tokens` (128k ceiling, `LISTPOCKET_AI_MAX_OUTPUT_TOKENS`) and retries once without it when the model refuses it. OpenAI reasoning models reject `max_tokens`. | `chat/provider/openai/openai.go:213`, `cmd/ai_assistant.go:578` | Send `max_completion_tokens` on Chat Completions, with a retry without a limit on a 400 that names it. Also confirm that `finish_reason: length` / `content_filter` / empty fails the turn rather than storing a truncated reply. |
| B10 | **No history import.** Both stores need importing: `agent_*` records with tool and proposal parts, and `localStorage` threads (including the older `listpocket_ai_builder_threads_v1`). The roadmap lists this as not built. | `docs/roadmap.md` "App migrations" | A store-level `ImportConversation` that keeps roles, timestamps, tool parts and historical proposal states (terminal, not actionable), and is idempotent by source ID. |
| B11 | **Actionable provider errors.** List Pocket tells the user "ChatGPT usage limit reached, wait or switch models" and "credentials rejected, check the key or sign in again under Settings → AI". The module only shows generic codes. | `cmd/ai_assistant.go:596`, `docs/chat.md` §3 "Failures" | More failure codes (`usage_limit`, `provider_auth`), or a host error mapper that returns safe user text. |
| B12 | **`parallel_tool_calls=false`.** List Pocket turns parallel calls off. The module doesn't send it. | `cmd/ai_assistant.go` runAssistant | A per-provider or per-turn option. |

To verify in the module (likely fine, but unconfirmed):

- **V1 Argument edits.** `AllowArgumentEdits` refuses edits that change "which
  collection, record or editor" a call targets. Confirm what counts as the target for
  custom tools. List Pocket's review modal edits whole workflow graphs, campaign drafts
  and plugin files.
- **V2 `/mcp` exposure.** `RegisterRoutes` mounts `/mcp`. List Pocket ships MCP as the
  stdio `listpocket-mcp` and adds no HTTP MCP endpoint today. Either confirm that chat
  can mount without `/mcp`, or decide to accept an `/mcp` that has no tools (D2).
- **V3 Titles.** Titles are model-generated or off. List Pocket uses the first message,
  cut to 50 or 60 characters, with no model call. Either add a `TitleModel:
  "first-message"` mode or have the client `PATCH` after the first send.
- **V4 Images.** The Agent page accepts any `image/*`, but the module accepts only PNG,
  JPEG, GIF and WebP. Other text types (HTML, YAML, TSV) fall back to `text/plain`,
  which is fine. Narrowing images is a small regression and needs D-approval or a
  module change.
- **V5 Long-running approvals.** `DecisionTimeout` defaults to 30 s. Measure plugin
  install, workflow publish and `propose_reply` (send) under load and set it to match.

## Work in List Pocket

These need no module changes once the blockers are fixed.

1. **Dependency and mount.** Add `pocketbase-mcp` and `adapters/pocketbase`. Use
   `pb.New(se.App, Config{Manifest: empty, AllowWrites: true, CustomTools: ...})` with
   `/mcp` on (see [The `/mcp` endpoint](#the-mcp-endpoint)), then `EnableChat` with
   `Namespace: "listpocket"`, `Proposals`, `Uploads`, `AllowArgumentEdits`,
   `TurnContext`, `StoreContext` and `AutoApprove`. Auth collection: `users`.
2. **Tool port (about 40 tools).** Make each `assistantTool` a `ToolSpec` with a real
   JSON Schema. Read tools get `Channels: "both"`, and write tools `"chat"` (see the
   `/mcp` section). Each `Handler` calls
   `pb.ResolveCaller`, then `caller.RequestEvent`, sets `auth.UserHTTPCtxKey` from the
   validated token (reusing `validatePBToken`), and runs the same handler through
   `PermRE`. Keep: the query-key allowlist, the SQL refusal on subscribers, `per_page=20`,
   the 256 KB "narrow the query" cap and the `source.href` mapping. Factor the current
   `executeAssistantTool` into one function used by both engines during the overlap, and
   by `/agent/mentions` and `/agent/preview` afterwards.
3. **Visibility.** Register policies such as `lp_super_admin` and
   `lp_perm:<permission>` as `PolicyFunc`s that load the principal's role permissions.
   Plugin tools get `lp_super_admin`, which keeps them out of other users' tool lists.
   `"superuser"` means PocketBase superusers and must not be used.
4. **Writes.** `propose_*`, `create_campaign_draft` and `propose_plugin` become write
   tools (proposals). `plugin_draft_write` and `plugin_draft_edit` run today without
   review, so they become `Approval: "auto"`. `propose_editor_patch` becomes an editor
   tool: its schema gets `editorId` and `baseRevision` (renamed from `revision`), and
   `Validate` reuses `validateAssistantProposal`. `Summarize` gives each tool the line
   the cards show today.
5. **`Setup` hook.** Rebuild the current system prompt: the base prompt, the page
   context, `flow.SchemaDoc` when the context mentions workflows, `assistantPluginDev`
   for super admins with plugins, and `agentContextText(refs)` (needs B4, B5). Limits by
   screen: side panel uses the AI builder timeout (≥180 s, or 3600 s for super admins);
   the Agent page uses 3600 s; 16 steps and a 128k output limit everywhere. Tool filter
   on the Agent page (B3).
6. **Providers.** Write a `catalog.Store` over List Pocket's settings (B2): "api" from
   env, "chatgpt" from `chatgpt.Store`, the default from `ai_settings`, and models from
   the existing 10-minute live cache. Keep Settings → AI and its routes as they are. Call
   `Reload` after every settings change. Workflow AI steps and the campaign builder stay
   on `internal/flow/llm`.
7. **Runner limits to match today:** `MaxConcurrentGlobal: 4`, `MaxConcurrentPerUser`
   at least 4 (the default of 2 would refuse a third parallel conversation, which works
   today), `MaxSteps: 16`, `MaxHistoryMessages: 60`. Check `MaxToolResultTokens` (8000)
   against `workflow_schema` / `workflow_catalog`, which the model must read in full.
8. **Feature flag.** Add `LISTPOCKET_CHAT_BACKEND=legacy|module`, per surface, so both
   engines run side by side until cutover.
9. **Old chats are deleted, not imported.** At cutover, add a migration that drops
   `agent_conversations`, `agent_messages` and `agent_attachments`. The side panel
   removes `listpocket_assistant_v1:*`, `listpocket_ai_builder_threads_v1` and
   `listpocket_ai_builder_legacy_owner` from `localStorage` on load. Remove the
   "Import previous chats" menu item and `importLegacy()`. Keep the
   `listpocket_agent_model` key (last model picked).
10. **Frontend.**
    - Upgrade `agent-panel` from v0.2.1 to at least v0.5.0. That spans four releases,
      and v0.4.0 refactored conversation handling, so the subclasses and slots need
      rework.
    - Build both hosts on `ChatController`. Use `bindAgentPanel` with
      `registerEntries: false` (supported) so List Pocket keeps its own proposal cards
      (operations diff, Review, Open link, warnings), tool-row labels, model picker,
      Undo / Resume / Retry, media pictures, the `@` picker, Paste data, `?ref=` deep
      links and the `assistant-applied` event.
    - Editor adapter: `snapshot` is the current `editor.snapshot()`, `validatePatch` /
      `applyPatch` wrap `patches.mjs`, `editorId` is `editor.key`, and outcomes are
      reported to `/proposals/{id}/outcome`. Undo stays client-side.
    - Media pictures: send them in `context` each turn, and persist them per
      conversation (B5 or `localStorage` keyed by conversation ID).
11. **Tests and docs.** Golden tests that compare legacy and new tool output for every
    tool and permission set. Port `ai_assistant_test.go`. Rewrite `assistant.cy.js` and
    `agent-page.cy.js` against module events (`text.delta`, `tool.*`, `proposal.*`,
    `turn.*`). Update `docs/ai-assistant.md` and `frontend/src/assistant/AGENTS.md`.

## The `/mcp` endpoint

Decided: List Pocket serves the module's HTTP MCP endpoint next to chat. It shares the
same tool registry, so MCP clients get the assistant's permission-checked tools.

| Topic | Setting | Why |
|---|---|---|
| Sign-in | **Bearer tokens only (`OAuthMode: off`) at first.** Clients send a List Pocket `users` token in `Authorization`, as `listpocket-mcp` does today. | `stockoauth` signs in through PocketBase's `auth-with-password`, which knows nothing about List Pocket's own TOTP 2FA (`cmd/auth.go`) or OIDC-only users. Turning OAuth on now would let a 2FA user sign in with only a password. Enable OAuth only once the module has a host-defined second step or hands off to an existing session (module roadmap, "Two-factor sign-in for remote MCP"). |
| What's offered | Read tools: `Channels: "both"`. Plugin tools: `"chat"` only. Editor tools are chat-only by design. | Reads are the same handler calls under the same permissions on both channels. |
| Writes | All proposal tools stay `"chat"` by default. An opt-in setting (for example `LISTPOCKET_MCP_ALLOW_WRITES`) moves only the old `listpocket-mcp` write set to `"both"`: create list, create contact, create template, contact note and `update_subscriber` (new, see below). | On MCP a write runs immediately, with no proposal or review in List Pocket. Sending, publishing, running workflows and installing plugins must never be reachable that way. `AllowWrites` has to be true for chat proposals, so channel membership is the gate. |
| Origins | `AllowedOrigins`: the app's own origin(s) | Avoid an open browser surface |
| Audit | `AuditFunc` writes to the app log. Payloads stay off. | Same visibility as chat failures |
| Rate limits | Not built in the module yet (milestone 6). Put per-IP limits at the reverse proxy until it is. | New network surface |
| `listpocket-mcp` (stdio) | **Decided 2026-11-19: removed**, in the same change that ships `/mcp`, so there's never a release without MCP. Remove `cmd/listpocket-mcp/`, `internal/mcpserver/`, the `modelcontextprotocol/go-sdk` dependency (`go mod tidy` also drops its indirect dependencies), the `/listpocket-mcp` line in `.gitignore`, and the local `listpocket-mcp` binary. Rewrite `docs/mcp.md` and the README line for the HTTP endpoint. | One MCP implementation instead of two |

Phase 1 additions: an MCP smoke test (an official client lists tools and calls one read
as an admin, a list-scoped user and a user without the permission); a check that
plugin and proposal tools are absent from `tools/list`; a check that a `_superusers`
token and a missing token get 401; and `docs/mcp.md` rewritten for the HTTP endpoint.

**What removing stdio changes for clients.** Existing stdio setups stop working and
must move to `https://<host>/mcp` with an `Authorization: Bearer <token>` header:

- Clients that support remote servers with custom headers (Claude Code, Cursor) need
  only a config change.
- Clients that require OAuth for remote servers (Claude Desktop and claude.ai custom
  connectors, ChatGPT) can't connect until OAuth is turned on, and that waits on the
  2FA work. Until then, the only path for them is a local bridge such as `mcp-remote`
  with a header. Document it in `docs/mcp.md`.
- Tool names change. The stdio server used `list_lists`, `get_subscriber` and so on;
  `/mcp` uses the assistant's names (`lists`, `subscribers` with `id`, and so on).
  Prompts or allowlists that name the old tools need updating.
- Some stdio tools have no assistant equivalent today:
  - reads: `get_subscriber_activity`, `get_subscriber_channels`,
    `get_dashboard_counts` and `get_dashboard_charts`;
  - the write `update_subscriber` (email, phone, names, attribs through
    `PATCH /subscribers/{id}`). The assistant's `propose_contact_update` changes only
    owner and company.

  Recommendation: port the four reads as `Channels: "both"` tools, and
  `update_subscriber` as a write tool in the opt-in MCP write set (a proposal on
  chat). They're thin wrappers over existing `/mailapi` handlers. With those added,
  every stdio capability exists on `/mcp`.

## Decisions

- **D1. Should the side panel and the Agent page share one conversation list?** After
  the move, both are server-side. For strict parity, keep them apart with a
  conversation `target` (for example `surface:panel` / `surface:agent`) and list by
  target. Unifying them is nicer but changes behaviour. Recommendation: keep them
  separate first, unify later as its own change.
- **D2. Should List Pocket expose `/mcp` over HTTP?** **Decided 2026-11-19: yes.**
  `DisableMCPEndpoint` stays off. See [The `/mcp` endpoint](#the-mcp-endpoint) for
  how it's set up.
- **D3. Should sticky attachments stay?** Sticky chips exist only because the side
  panel's server is stateless. With stored uploads, follow-up turns see earlier files
  without them. Recommendation: keep the sticky UI but stop re-uploading, by
  re-attaching the stored upload IDs.
- **D4. Image types:** accept narrowing to PNG, JPEG, GIF and WebP, or extend the module
  (V4).

## Phases

| Phase | Work | Done when |
|---|---|---|
| 0. Module | B1–B12, V1–V5 in Pocketbase-MCP. Release with submodule tags. **Done in v0.1.6** | Tagged release that List Pocket's `go.mod` and Docker build can consume ✅ |
| 1. Backend behind the flag | Items 1–8 | `go test ./cmd` green. Golden tool tests pass. Both engines selectable. **Done in `ea69a35..HEAD`** ✅ |
| 2. Agent page | Item 10 for `Agent.vue` | `agent-page.cy.js` parity suite passes on `module`. **Done:** `AgentRoute.vue` picks the host, `AgentChat.vue` binds the module controller, `chat-host.mjs` holds the pure logic, and `agent-page-chat.cy.js` passes; the legacy `agent-page.cy.js` is unchanged ✅ |
| 3. Side panel | Item 10 for `AIAssistant.vue`, editor adapter | `assistant.cy.js` parity suite passes. Editor apply and undo work on campaign, template and workflow editors |
| 4. Cutover | Default to `module`. Delete old chats (item 9). Remove `ai_assistant.go` turn machinery, `agent.go` conversation CRUD and part endpoints, `transcript.mjs`, `threads.mjs` | No legacy code paths or data. Docs updated. Keep `GET /{$}` (see notes) |

### Phase 1 notes

- In-process tool calls now set `Request.ContentLength` (`setToolBody`). Without
  it, writes from chat and MCP bound empty bodies.
- With no provider configured, the module rejects a turn with 400
  `invalid_request` before `Setup` runs, so the user doesn't get today's "AI
  isn't set up…" text. Phase 2 handles this on the Agent page: with no
  `choices` and no `models` from `/status`, `AgentChat.vue` shows the
  translated setup message and disables sending, so no such turn starts. The
  side panel still shows the generic line until phase 3.
- The chat default model, when Settings → AI has none, is the AI builder's model
  on the API key, as in the legacy assistant.
- A handler error on a write that may already have applied is recorded as failed
  with the generic "Tool execution failed.", because the module only marks a
  timeout or cancellation as unknown. Follow-up: ask the module for an exported
  unknown-outcome error for custom tools.
- Pre-existing, outside this migration: `internal/core` `HasSubscriberLists` and
  `CampaignHasLists` compare integer list IDs with the text record IDs in
  `subscriber_lists.list_id` / `campaign_lists.list_id`, so list-scoped users are
  refused single contacts and campaigns everywhere. It fails closed. Track
  separately.

### Phase 4 notes

- At cutover, keep `GET /{$}`. The chat module's all-method mount panics `serve` if `GET /` is registered again (the chat fixture test catches it). Without the module flag, `GET /` renders the home page for unknown paths; with `/{$}` those paths 404. Decide deliberately whether unknown public paths should 404.

## Parity checklist

Each line needs a test (Go or Cypress) on the module backend before cutover.

- [ ] Every tool returns the same data as legacy, for an admin, a list-scoped user and a
      user without the permission
- [ ] Plugin tools are invisible and uncallable for non-super-admins
- [ ] Analytics argument checks, the subscribers SQL refusal and the query-key allowlist
- [ ] Each write tool creates a proposal. Approve runs it exactly once (double click, two
      tabs). Reject. Edit-then-apply re-validates
- [ ] A 400 from a handler keeps the proposal pending with the message (B8). Other
      failures are never retried
- [ ] Flow proposal warnings appear on the card and in the model's tool result (B6)
- [ ] Plugin proposal approves the snapshotted files, not the live draft (B7)
- [ ] Editor patch: set, replace_text, replace_body, block, node and edge ops. Revision
      mismatch. Wrong editor open. Undo until the next edit or turn
- [ ] Applied proposal shows an Open link from `result.source.href`.
      `assistant-applied` fires
- [ ] `@` references: picker, chips on the sent message, preview modal, re-read fresh
      every turn, unreadable reference reported, cap of the 30 most recent (B4, B5)
- [ ] Attachments: images, PDF, text and pasted data. Size errors match the limits the
      UI states. Follow-up turns still see them
- [ ] Media-library pictures are passed as name and URL
- [ ] Model picker per conversation, last pick remembered. ChatGPT sign-in and the API
      key provider both work from Settings → AI, with no second sign-in
- [ ] Super-admin and Agent-page turns allow 1 hour. Side panel uses the AI builder
      timeout. 16 steps. Tool-limit message
- [ ] Output limit of 128k, `LISTPOCKET_AI_MAX_OUTPUT_TOKENS`, retry when refused,
      truncated reply fails (B9)
- [ ] Provider errors show the same actionable text (B11)
- [ ] One active turn per conversation (409). Global cap of 4. Stop. Reconnect resumes
      without re-running tools
- [ ] Titles from the first message. Rename, delete, account switch clears state
- [ ] After cutover, old chats are gone: `agent_*` collections dropped, legacy
      `localStorage` keys cleared, no import UI left

## What we gain

These aren't needed for parity, but come with the move: proposals and turns survive a
restart (today they're lost), side-panel history on the server and across devices,
token-budgeted history with summaries and a context ring, per-turn reasoning effort,
uploads in PocketBase file storage instead of base64 in records, and failed-turn
logging.
