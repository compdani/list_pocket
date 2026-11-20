# @pocketbase-mcp/chat-client

Dependency-free browser client and framework-neutral controller for the MCP chat
HTTP API (protocol v1). Plain ES modules plus hand-written type declarations; it
runs in browsers and in Node 22 using only standard web APIs.

## Usage

```js
import PocketBase from 'pocketbase';
import { ChatClient, ChatController } from '@pocketbase-mcp/chat-client';

const pb = new PocketBase('https://example.com');

const client = new ChatClient({
  baseUrl: 'https://example.com',
  // getToken runs before EVERY request. Never capture the token once.
  getToken: () => pb.authStore.token,
});

const chat = new ChatController({
  client,
  storageKey: {
    appUrl: pb.baseURL,
    mount: '/',
    authCollectionId: pb.authStore.record?.collectionId,
    userId: pb.authStore.record?.id,
  },
});

// Every state change dispatches "change" with no payload.
chat.addEventListener('change', () => render(chat.state));
await chat.init();
await chat.open('conversation-id');
await chat.send('Hello', { model: chat.state.choice || 'gpt-4.1' });
```

`chat.state` is an immutable snapshot:

```js
{
  status: 'idle' | 'loading' | 'ready' | 'error',
  conversations, nextCursor,
  activeConversationId, messages,
  transcript,      // one entry per message part (text, tool, notice, attachment)
  proposals,       // ChatProposalEntry[]: open proposals + live updates/outcomes
  pendingUploads,  // uploaded attachments not sent yet
  turn,            // TurnView | null: live text, tools, usage, proposals, lastSeq
  running, error,
  features,        // { proposals, uploads } from /status
  limits, models,  // models is the legacy flat list
  choices,         // ChatModelChoice[]: provider/model pairs from /status
  defaultChoice,   // the server's default choice id
  choice,          // the picker's current choice id
  context,         // how full the open conversation is, or null (see below)
}
```

## List sync

The conversation list follows the server without the host doing anything:

- after every turn ends (the server has moved the conversation's `updatedAt`
  and may have titled it);
- when the page becomes visible or the window regains focus;
- every `intervalMs` (60 s) while the page is visible;
- whenever another controller for the same account (another element on the
  page, such as a side panel next to a full chat page, or another tab) creates,
  renames, deletes or sends. They talk over a `BroadcastChannel` named after
  the account key, so another account never hears it.

`refreshConversations()` does the same on demand. It merges the first page
over what is loaded: new and renamed rows appear, rows the page covers but no
longer returns are dropped, and rows loaded with `loadMore()` stay. If the open
conversation is missing, it is cleared only after the server answers 404 for
it. A refresh that raced a local write reads the list again rather than undo
the write.

```js
new ChatController({ client, storageKey, sync: { intervalMs: 30000 } });
new ChatController({ client, storageKey, sync: false }); // only after-turn refreshes
```

`startDraft()` leaves the open conversation without creating one; the next
`send()` (or the binding's first message) creates it. The binding's **New
chat** uses it, so pressing it twice leaves no empty conversations behind.

Call `chat.setAccount({ appUrl, mount, authCollectionId, userId })` whenever the
signed-in user can change. A different key aborts in-flight requests, clears all
state and reloads, so one user's data is never shown to another.

## Provider choices

With a provider catalog, `/status` returns labelled `choices` such as
`{ id: "codex-work::gpt-5-codex", provider: "codex-work", model: "gpt-5-codex",
label: "GPT-5 Codex", group: "Codex (work plan)" }`. The controller keeps them
in `state.choices`, sets `state.choice` to the server default, and resolves the
picker's value to `{ provider, model }` on `send()`. A choice id is opaque;
`agent-panel` receives labelled objects and emits the chosen id. Without a
catalog the old `models` strings and a plain `model` value still work.

A choice may list `efforts` (reasoning effort levels, lowest first).
`send(text, { effort: 'high' })` sends one with the turn, but only when the
resolved choice lists it; otherwise it is dropped and the model's default
applies. The agent-panel binding adds a "Reasoning effort" entry to the
element's options menu while the selected model has efforts ("Model default"
sends none), keeps any options the host set itself, and passes the pick on
each `agent-submit`. `effortOptionFromState(state, model)` returns that entry.

## Context window

When the server manages the model's context window (`features.contextManagement`,
see [chat.md section 11](../../docs/chat.md#11-context-window-and-summaries)),
`state.context` is `{ tokens, window, estimated, summarizedThrough,
summarizedAt, compacting, canCompact }`. It comes with the conversation on
`open()`, is refreshed after every turn, and is null for a draft or an older
server. While a background summary is being written (`compacting`), the
controller re-reads it every 3 seconds until it lands.

```js
await chat.compact();       // POST /conversations/{id}/compact; null on failure (see state.error)
await chat.refreshContext(); // GET /conversations/{id}/context
```

`compact()` sets `context.compacting` while it runs. A refusal (`too_soon`
within 30 s of the last summary, `active_turn`, `conflict`) keeps the last
numbers and puts the error in `state.error`. The client methods are
`client.getContext(id)` and `client.compactConversation(id)`.

`bindAgentPanel` shows it: agent-panel 0.5 draws a ring in the composer once
60% of the window is used, with a card whose **Compact** button calls
`compact()`. With `{ contextUsage: 'slot' }`, older agent-panel versions get a
small "72% context" button in the `composer-actions` slot instead (click to
compact); `{ contextUsage: false }` shows nothing. The transcript gets a thin
"Earlier messages are summarized for the model" divider (a `context-marker`
entry) after the last summarized message.

## Page context

When the server turns on `TurnContext`, `/status` reports `features.context`
and each turn may carry a plain object describing the user's page. Give the
controller a `context` function; it is read once per send (retries reuse the
same value) and only when the server accepts contexts:

```js
const chat = new ChatController({
  client,
  context: () => ({ screen: 'campaign-editor', campaignId, editorRevision: editor.revision, selection: editor.selectedText() }),
});
await chat.send('Tighten the intro'); // sends the context above
await chat.send('Hi', { context: { screen: 'dashboard' } }); // overrides it
```

The server decides what the context changes (the system prompt, the limits);
it never grants access by itself. A server can refuse a turn for its context
with `400 turn_refused`, which surfaces as a `ChatApiError` with the server's
message.

## API

### `ChatClient`

| Method | HTTP |
| --- | --- |
| `status()` | `GET /status` |
| `listConversations({ limit, cursor, target })` | `GET /conversations` |
| `createConversation({ title, target })` | `POST /conversations` |
| `getConversation(id, { limit, cursor })` | `GET /conversations/{id}` |
| `renameConversation(id, title)` | `PATCH /conversations/{id}` |
| `deleteConversation(id)` | `DELETE /conversations/{id}` |
| `getContext(id)` | `GET /conversations/{id}/context` |
| `compactConversation(id)` | `POST /conversations/{id}/compact` |
| `importConversation({ sourceId, title, createdAt, messages })` | `POST /conversations/actions/import` |
| `submitTurn(id, { text, provider, model, attachments, idempotencyKey })` | `POST /conversations/{id}/turns` |
| `getTurn(id)` | `GET /turns/{id}` |
| `cancelTurn(id)` | `POST /turns/{id}/cancel` |
| `streamTurn(id, { after, signal, onEvent, onHeartbeat })` | `GET /turns/{id}/events` |
| `getProposal(id)` | `GET /proposals/{id}` |
| `decideProposal(id, { decision, revision, arguments })` | `POST /proposals/{id}/decision` |
| `reportOutcome(id, { revision, status, editorRevision, hunks, message })` | `POST /proposals/{id}/outcome` |
| `uploadFile(conversationId, file, name?)` | `POST /uploads` (multipart) |
| `getUploadMeta(id)` | `GET /uploads/{id}/meta` |
| `downloadUpload(id)` | `GET /uploads/{id}` (returns a `Blob`) |

`submitTurn` returns `{ turn, created }` and generates an `Idempotency-Key` when
none is given. `streamTurn` resolves with `{ lastSeq, terminal }`; a stream that
ends without a terminal event is a normal end, not an error.

Errors are typed: `ChatApiError` (`status`, `code`, `message` from the server
envelope), `ResyncRequiredError` (a 409 `resync_required`), `ChatNetworkError`
(the request never reached a response) and `ChatProtocolError` (a version or
framing violation). A decision or outcome conflict (409 `conflict`, `stale`,
`expired`) also carries the authoritative stored proposal as
`err.proposal`; adopting it avoids a second request.

`uploadFile` sends `multipart/form-data` with exactly the server's field names,
`conversationId` and `file`, so the Content-Type boundary is generated by
`fetch`. `downloadUpload` authenticates with the Authorization header and
returns the bytes as a `Blob`; no token is ever placed in a URL.

## Proposals

`chat.state.proposals` merges the conversation's open proposals from
`GET /conversations/{id}` with every live `proposal.created` / `proposal.updated`
event. Merging is by id with the newest revision winning, so an out-of-order
frame can never undo a newer state and no proposal is ever duplicated. Terminal
states (`rejected`, `expired`, `succeeded`, `failed`, `unknown`, `applied`,
`stale`) stay visible through the rest of the view — the next `open()` is
authoritative again — so a user sees what happened to the change they reviewed.

```js
// Approve or reject at exactly the revision on screen.
const { proposal, replayed, error } = await chat.decide('proposal-id', 'approve');

// A 409 conflict is not thrown: the stored proposal is adopted and `error`
// carries the server's code and message for that proposal row.
if (error) renderConflict(proposal, error);
```

`decide()` uses the revision currently in state. On a 409, or a 422 `refused`
(the tool refused the approved change before writing anything, and the
proposal is pending again), it adopts `err.proposal`, records the message on
that entry (`proposal.error`) and resolves with `error` set; the rest of the
view is untouched. Every other failure throws.

`reportOutcome(proposalId, { status, editorRevision, hunks, message })` records
what a browser editor did with an approved editor proposal. It is an
acknowledgement of a local draft, never proof that anything was saved.

## Editor adapter

Editor proposals (`location: 'editor'`) are applied by the host page, never by
the server. Pass an adapter to the controller:

```js
const chat = new ChatController({
  client,
  storageKey,
  editor: {
    snapshot: () => ({ editorId: 'note-1', revision: currentRevision }),
    validatePatch: (proposal) => patchStillApplies(proposal), // false => stale
    applyPatch: (proposal) => {
      const revision = applyLocally(proposal.arguments);
      return { revision, hunks: [{ id: 'h1', status: 'applied' }] };
    },
    undo: (token) => undoLocally(token), // host-owned
  },
});
```

Approving an editor proposal calls `validatePatch(proposal)`:

- not applicable → the controller reports `stale` (with the editor's revision
  when `snapshot()` names one);
- applicable → `applyPatch(proposal)`, then reports `applied` with the returned
  revision (and hunks);
- `applyPatch` throws → reports `failed` with its message.

The outcome replaces the proposal in state, so the UI shows exactly what was
recorded. Nothing here saves anything: saving stays in the app's own protected
API. Approving an editor proposal without an adapter is refused client-side with
`code: 'editor_unavailable'`.

## Attachments

```js
// 1. Validate and upload; a conversation is created first when none is open.
const uploads = await chat.attach(fileInput.files);

// 2. Send with the pending uploads, upload ids, or fresh File/Blob values.
await chat.send('Summarise this', { attachments: uploads.map((u) => u.id) });
```

`attach()` checks the client-known limits from `/status` before uploading:
`limits.attachmentTypes`, `limits.maxUploadBytes` and
`limits.maxAttachmentsPerTurn`. A refusal throws a `ChatApiError` with the
server's own code (`unsupported_media`, `payload_too_large`, `invalid_attachment`)
so a UI branches on one set of codes. The server re-validates every file and
assigns the MIME type from its bytes.

With a provider catalog, `limits.attachmentTypes` is the union every ready
model accepts. Pass `attach(files, { model })` (a choice id or model id) to
check against one model instead. `send()` always checks new files against the
model it sends to, and the server refuses a turn whose model can't read an
attachment with `unsupported_attachment`.

`send(text, { attachments })` accepts upload ids and `File`/`Blob` values; blobs
are uploaded first. Uploads stay in `state.pendingUploads` until a turn accepts
them, and `clearAttachments()` drops them. An account switch clears both
proposals and pending uploads.

The agent-panel binding turns `agent-submit`'s files into that argument, mirrors
`/status`'s type/size/count limits onto the composer, and renders the stored
attachment of each user message as a chip. A chip downloads through
`downloadUpload`, so the bearer token stays in the `Authorization` header and
there is never a public or data URL in the transcript.

## Transcript entries

`state.transcript` is one entry per message part:
`{ id: "<messageId>:<index>", messageId, role, turnId, createdAt, ...part }`.
That makes a notice message (`role: 'notice'`, part type `proposal_outcome`,
with `proposalId`, `state`, `tool` and the server-built `text`) and an
`attachment` part (`uploadId`, `name`, `mime`) ordinary entries a host can
render, instead of making every consumer walk `messages[].parts` itself.
`transcriptFromMessages(messages)` is exported for custom hosts.

### `ChatController`

`init`, `loadMore`, `refreshConversations`, `open`, `newConversation`,
`startDraft`, `rename`, `remove`, `send`,
`attach`, `clearAttachments`, `decide`, `reportOutcome`, `stop`, `setAccount`,
`dispose`, plus `whenSettled()` for tests and shutdown.

### `bindAgentPanel` / entry mapping

> **Requires agent-panel v0.4.0 or later; v0.5.0 recommended.** Install it with
> `npm install github:compdani/agent-panel#v0.5.0`. 0.4.0 adds the panel's
> conversation switcher (`conversations-menu`, with delete), the built-in
> "Load more" (`has-more` / `agent-load-more`), and the panel's
> `conversations` / `active` properties that the list sync writes. With 0.3.x,
> `<agent-panel>` shows no conversation list and the page's list has no
> "Load more". 0.4.1 changes only the panel's look: the switcher becomes the
> header title (with New chat beside Close), the `switcher-end` slot and the
> `floating` attribute are new, and the binding needs no change for them.
> 0.5.0 adds the composer's context ring (`contextUsage`, `agent-compact`),
> which the binding fills from `state.context`; older versions show no ring.

Binds one controller to one `<agent-panel>` or `<agent-page>` from
[`agent-panel`](https://github.com/compdani/agent-panel). The element is passed
in, so this module imports nothing and the two packages stay independent.

```js
import 'agent-panel/page';                       // defines <agent-page>
import { bindAgentPanel } from '@pocketbase-mcp/chat-client/agent-panel';

const page = document.querySelector('agent-page');
page.renamable = true;                            // the page owns the title UI
const chat = new ChatController({ client, storageKey });
const binding = bindAgentPanel(page, chat);
await chat.init();

// Later, when signing out or unmounting:
binding.unbind();
chat.dispose();
```

A side panel gets the same list with agent-panel's switcher: the header's
title becomes the open conversation's title and a chevron, opening a dropdown
of conversations (each with its date and Delete), with New chat beside Close.
`heading` is shown only while there are no conversations yet; put a fixed mark
in `header-start` (it stays before the switcher), and actions under the list in
`switcher-end`. `floating` (0.4.1) draws the panel as a card over the page:

```html
<agent-panel heading="Assistant" conversations-menu floating closable>
  <span slot="header-start" aria-hidden="true">✦</span>
  <button slot="switcher-end" type="button">Open in the full page</button>
</agent-panel>
```

```js
import 'agent-panel';
const panel = document.querySelector('agent-panel');
bindAgentPanel(panel, new ChatController({ client, storageKey }));
```

Give the panel its own controller even when a page is open too: the two keep
their lists in step (see [List sync](#list-sync)).

On every controller `change` it writes:

| Property | Value |
|---|---|
| `entries` | The transcript, the live turn, notices and proposals, in agent-panel's entry shape (below). |
| `running` | `state.running`. |
| `error` | A plain-language line mapped from `state.error` (and a failed turn's error). Raw server messages are never shown. |
| `models` | `state.models`, for the model menu. |
| `no-attachments`, `accept`, `max-file-size`, `max-files` | Attachment attributes mirrored from `/status` once it answers; `no-attachments` when uploads are off or no type is accepted. |
| `conversations`, `active`, `hasMore` | The list rows, the selected id, and whether "Load more" shows (`nextCursor`). On `<agent-page>` and `<agent-panel>` (shown with its `conversations-menu` attribute). |
| `heading` | Page only: the open conversation's title. A panel keeps the heading the host gave it (with `conversations-menu`, the switcher shows the conversation's title in its place). |

Entry shapes follow agent-panel's `element.d.ts`, plus two custom types the
binding registers with `defineEntry()`:

```js
{ id, type: 'text', role: 'user' | 'assistant', content, references? }
{ id, type: 'tools', tools: [{ id, name, status: 'running' | 'completed' | 'failed' }] }
{ id, type: 'notice', content, proposalId?, state?, tool?, isError? }
{
  id, type: 'proposal', proposalId, tool, summary, state, location, revision,
  error, status, detail, warnings, canDecide,
}
```

- Transcript messages become text entries, and each assistant message that
  called tools becomes one tools entry. Tool results in `role: 'tool'`
  messages move the call to `completed` or `failed`.
- A `role: 'notice'` message (`proposal_outcome`) becomes a small system line,
  never a user or assistant bubble.
- An `attachment` part becomes a chip on its user message (`references`, type
  `attachment`, id = upload id, label = stored file name, plus `mime`). No
  bytes and no data URLs go into the panel; a chip click downloads through the
  client. A message with only attachments is a chip-only entry.
- Proposals follow the transcript: pending ones show **Approve**/**Reject**;
  every state has its own plain line ("applying on the server…", "applied in
  the editor — not saved yet", "may or may not have applied — check before
  retrying", and so on). An approval never reads as a save.
- The live turn adds only what the transcript does not have yet: the streaming
  text arrives incrementally, a running tool shows a spinner, and both are
  replaced by canonical messages after the terminal reload.
- The message just submitted is echoed as a user bubble until the reloaded
  transcript contains it, so the first second of a turn is not empty.
- `detail` is left out: raw tool arguments and results are protocol internals,
  and the panel's default tool rows do not need them.

Events handled:

| Element event | Action |
|---|---|
| `agent-submit` | `controller.send(text, { model, effort, attachments })`, where `detail.model` is a choice id (or a model id), `effort` is `detail.options.effort` and `detail.attachments[].file` becomes the File list. Files are uploaded first; the event is cancelled so the chips survive until the turn is accepted, then they are removed. A failure keeps the chips (with an error state) and restores the draft. With no open conversation, one is created first (`options.autoCreate = false` disables that). |
| `agent-action` (`proposal-decision`) | `controller.decide(id, decision)`; the buttons in a proposal card emit this. A failure is shown in the panel's error banner. |
| `agent-action` (`attachment-download`) | Downloads one attachment, for a host-drawn chip. |
| `agent-reference-open` | Downloads an attachment chip (`type: 'attachment'`); other references are left to the host. |
| `agent-stop` | `controller.stop()`. |
| `agent-error-dismiss` | Forgets the binding's copy, so a repaint does not bring the banner back. |
| `agent-select-conversation` | `controller.open(id)`; the event is cancelled so the row does not navigate. |
| `agent-new-conversation` | `controller.startDraft()`: the first message creates the conversation. With `autoCreate: false`, `controller.newConversation()`. |
| `agent-delete-conversation` | `controller.remove(id)`. |
| `agent-rename` | `controller.rename(activeId, title)`; cancelled when no conversation is open. |
| `agent-model-change` | Repaints, so the options menu offers the newly selected model's efforts. |
| `agent-load-more` | `controller.loadMore()`. agent-panel 0.4.0+ emits it from the list's built-in "Load more" (shown while `hasMore`). `binding.loadMore()` does the same from code. |

Other `agent-action` kinds are ignored. Downloads use the `ChatClient` passed
as `options.client`, then the controller's own client, and can be replaced with
`options.download(uploadId, reference)`. `options.registerEntries: false` skips
the automatic `defineEntry()` calls; `registerProposalEntry(element)` does the
same thing explicitly (agent-panel keeps renderers per instance, so pass it an
element, not the class).

`unbind()` removes every listener and stops writing to the element. The pure
helpers are exported for tests and custom hosts: `entriesFromState(state,
{ pendingText })`, `proposalEntryOf(proposal)`,
`proposalEntriesFromState(state)`, `proposalErrorText(error)`,
`errorText(error)`, `conversationsFromState(state)`, `hasConversationList(element)`,
`modelsFromState(state)`, `effortOptionFromState(state, model)` and
`isPageElement(element)`, `contextUsageFromState(state)` and
`renderContextMarkerEntry(entry)`. `errorText` maps the failure codes a failed
turn records (`provider_error`, `max_steps`, `context_too_long`,
`internal_error`, …) to plain language and never shows raw server text.

### `parseSSE` / reducer helpers

```js
import { parseSSE } from '@pocketbase-mcp/chat-client/sse';
import { createTurnView, applyEvent } from '@pocketbase-mcp/chat-client/reducer';
```

`parseSSE(body, { signal, maxEventBytes, onComment })` yields `{ id, event,
data }`. The reducer is an immutable port of the server's Go reducer:
duplicates return the same view, gaps throw `GapError`, a foreign turn throws
`ForeignTurnError`, and an event after a terminal state throws
`AfterTerminalError`.

## Guarantees

- **No token in URLs.** `getToken()` is called before every request and its
  value goes only in the `Authorization` header. Token query parameters are
  rejected by the server and never produced here.
- **Idempotent send.** A lost POST response is retried with the same
  `Idempotency-Key`, so it can never create a second turn.
- **Reconnect without replaying turns.** The controller reconnects the event
  stream from its last sequence; it never re-submits a turn. Idle streams (no
  event or heartbeat) reconnect with backoff.
- **Resync is explicit.** A 409 `resync_required` or a reducer gap reloads the
  turn and transcript, then polls until the turn is terminal.
- **Account switch clears state.** Account keys are percent-encoded per field,
  so different identities cannot collide, and late responses from the previous
  account are ignored.
- **Nothing is persisted.** Server history is canonical; this version has no
  `localStorage` history and no offline fallback.
- **No implied saves.** A proposal, an approval or a browser editor outcome is
  never presented as a saved write; only the server's recorded result is.
- **No dependencies.** `package.json` has no `dependencies` and no
  `devDependencies`; tests run with Node's built-in `node --test`.

## Scope

This is the v1.2 chat surface: conversations, turns, live text, tool progress,
usage, proposals with decisions and editor outcomes, and attachments. The
agent-panel binding maps all of it onto the existing `<agent-panel>` /
`<agent-page>` components, and `examples/chat/{plain,vue,svelte}` show it in
plain JS, Vue and Svelte.

## Tests

```sh
node --test
```
