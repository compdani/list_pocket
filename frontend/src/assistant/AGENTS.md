# AGENTS.md: the assistant side panel and the Agent page

The assistant side panel (`<lp-agent-panel>`) and the full Agent page (`<lp-agent-page>`)
are both built on the **`agent-panel`** package
(https://github.com/compdani/agent-panel). It's installed from GitHub and pinned to
v0.5.1 in `frontend/package.json`. The package's own contract is in
`node_modules/agent-panel/AGENTS.md`, and its human guide is in
`node_modules/agent-panel/README.md`. Read the contract before changing anything here.
This file covers only how List Pocket extends and hosts the two elements.

## Rules

1. **Never edit `node_modules/agent-panel`.**
   - To change behaviour that any app could use, change the package repo
     (`/Volumes/mini512/Components/agent-panel` locally), release a new tag, then update
     the pin here with `npm install github:compdani/agent-panel#vX.Y.Z`.
   - Behaviour specific to List Pocket goes in `lp-agent-panel.js` (a subclass) or
     `AIAssistant.vue` (the host).
2. **Import by package name**: `agent-panel/element` (panel class, `h`, `icon`) and
   `agent-panel/page-element` (page class). These define no tags. Never import the
   package's files by path.
3. **Register the tags with Vue's compiler.** `vite.config.js` lists `agent-panel`,
   `agent-page`, `lp-agent-panel` and `lp-agent-page` in
   `compilerOptions.isCustomElement`. A new custom-element tag must be added there.
4. **Shared behaviour goes in `lp-agent-common.js`.** That covers the theme, labels,
   markdown, proposal cards and reference icons. Both subclasses use it, and it defines
   no tags.

## Files

| File | Role |
|---|---|
| `lp-agent-common.js` | `LP_STYLES` (Vuetify theme mapping and card CSS), `LP_LABELS`, `renderMarkdown`, `renderProposal(el, entry)`, `referenceIcon(ref)` (`@mdi/js` paths per context type) |
| `lp-agent-panel.js` | `ListPocketAgentPanel` (`<lp-agent-panel>`) and `ASSISTANT_ACCEPT` |
| `lp-agent-page.js` | `ListPocketAgentPage` (`<lp-agent-page>`) and `AGENT_PAGE_ACCEPT`; on the legacy host, adds the server's 2 MB limit for text files through `fileProblem` |
| `chat-host.mjs` | Pure logic for the module-backed Agent page: page context, per-submit snapshots, stale-error detection, history writes, stored `@` chips, `contextReferences`, proposal card and ReviewModal inputs, error wording, remembered model |
| `../views/AgentRoute.vue` | The `/agent/:id?` route: renders `Agent.vue` or `AgentChat.vue` from `serverConfig.chat_backend`, and nothing until `/api/config` has answered |
| `../views/AgentChat.vue` | The module host: `ChatClient` + `ChatController` + `bindAgentPanel`, composer controls, proposal decisions, `ReviewModal` |
| `../views/Agent.vue` | The Agent page on the legacy engine: server-side conversations, turns, uploads, context picker, proposal decisions |
| `../components/AIAssistant.vue` | The host: chats kept in the browser, streaming turns, proposal decisions, and the Vuetify controls placed in slots |
| `markdown.mjs`, `threads.mjs`, `transcript.mjs`, `patches.mjs`, `client.js`, `context.js` | Conversation logic, independent of the panel |
| `../../cypress/e2e/assistant.cy.js` | Browser tests for the side panel, including paste, drop and keeping attachments for the next message |
| `../../cypress/e2e/agent-page.cy.js` + `fixtures/agent-page-harness.*` | Browser tests for the legacy Agent page: conversations, references, `@` picker, paste and upload, rename and delete, narrow-screen drawer |
| `../../cypress/e2e/agent-page-chat.cy.js` + `fixtures/agent-page-chat-harness.*` | Browser tests for the module Agent page: URL loading, `@` context, streams, proposals, refused decisions, no provider, rename/delete, paste upload, overlapping sends, failed-send restore, the status gate, history, one Load more, plain user text |

## `<lp-agent-panel>` and `<lp-agent-page>`

Both subclasses do the following (from `lp-agent-common.js`):

- `renderMarkdown` uses `renderAssistantMarkdown` (markdown-it with `html: false`, images
  disabled, and links opening in a new tab with `rel=noopener`).
- User bubbles keep agent-panel's own inline markdown (`inlineMarkdownToHTML`
  in `renderText`), so `**x**` reads as bold there too. This is deliberate;
  don't override `renderText` to make them plain.
- The theme tokens map to Vuetify's `--v-theme-*` variables, with fallbacks.
- `referenceIcon` maps the context types in `CONTEXT_ICONS` (`agent/parts.mjs`) to
  `@mdi/js` paths. Add new types in both places.
- The hosts set `plain-text` and `no-add-menu` on both elements. Since 0.3.0 the default
  composer is a contenteditable rich-text editor and the default attach control is a "+"
  menu. Both hosts keep the old `<textarea>` and paperclip until the side panel moves to
  the module in phase 3; the module Agent page keeps them too, because its composer
  controls (Context, Paste data) and the browser tests are the legacy ones.
- `defaultLabels` adds `review`, `view`, `discard`, `open` and `proposedEdit`. The side
  panel uses the English defaults; the Agent page passes translated ones.
- It adds the entry type `{ id, type: 'proposal', proposal, busy: boolean, running:
  boolean, actionId? }`. `proposal` follows the server's proposal object: `id`, `tool`, `status`,
  `arguments` (with an optional `operations[]`), `warnings[]`, `result.source.href`.
  - The card shows the title from `proposalTitle()`, the kind from `proposalInfo()`, a
    status badge once the proposal is no longer pending, any warnings, and each
    operation's before and after text.
- Proposal buttons call `emitAction(action, { id: actionId ?? proposal.id })`:
  - `apply`: shown when the status is `pending` and `!proposalInfo(tool).reviewFirst`.
    Disabled when `busy || running`.
  - `review`: shown when there are no `operations`. Labelled "Review" while pending and
    "View" otherwise.
  - `discard`: shown while pending. Disabled when `busy || running`.
  - For an applied proposal with `result.source.href`, an "Open" link replaces the
    buttons.
- **`ASSISTANT_ACCEPT`** is
  `image/png,image/jpeg,image/gif,image/webp,application/pdf,text/plain,.txt`.
  - It mirrors the backend: `validateAIBuilderStatusAttachments` /
    `normalizeAIBuilderStatusMIME` in `cmd/ai_campaign_builder.go`, plus the
    image/PDF/text check in `cmd/ai_assistant.go`.
  - Change both sides together. Otherwise files pass the browser and then fail on the
    server.

## `Agent.vue`, the Agent page host

- **Conversations live on the server**, unlike the side panel's, which live in the
  browser. The host loads `agent/conversations` and sets `conversations` (with `href`
  from the router), `active`, `heading`, `renamable` (once the conversation exists),
  `contextReferences` (`contextInUse(messages)`), `formatDate` (`shortStamp`) and
  translated `labels`.
- **`entries`** is built from the server's messages:
  - user messages become text entries with `references` (from `msg.context`);
  - assistant messages become a tools entry (status `failed` with the error as the
    detail when a tool erred), then the text, then one proposal entry per proposal part,
    with `actionId: "<messageId>:<partIndex>"`;
  - while a turn streams, `live:tools` and `live:text` entries are added.
- **Sending:**
  - `agent-submit` calls `send(text, attachments, references)`.
  - **Files are uploaded when sending**, not when attached, through `agent/attachments`
    (base64 for images and PDFs, text otherwise), after `ensureConversation()`. They're
    then sent as `{ type: 'file', id, label }` context together with the references.
  - If something fails, the draft, the references and (if the upload never finished)
    the files are put back in the composer.
- **The context picker:** `mention-trigger="@"`. `ContextPicker` sits in
  `<div slot="mention">` and is shown while `agent-mention.open`. `agent-mention-key` is
  forwarded to `pickerRef.key(event)`, and the event is cancelled when the picker used
  the key. A pick calls `page.addReference()`. The **Context** button
  (`composer-actions`) toggles `openMention()` / `closeMention()`.
- **Paste data** (`composer-actions`) is a Vuetify dialog that adds a text `File`
  through `page.addFiles()`.
- **Other events:**

  | Event | Response |
  |---|---|
  | `agent-select-conversation` | `preventDefault()`, then `router.push` |
  | `agent-new-conversation` | Opens a new chat |
  | `agent-delete-conversation` | `DELETE` |
  | `agent-rename` | `PATCH` |
  | `agent-action` | `apply` / `discard` → the parts endpoints; `review` → `ReviewModal` |
  | `agent-reference-open` | `ContextPreviewModal` |

- **`?ref=<type>&id=…&label=…` in the URL** pre-attaches a reference on mount.

## `AgentChat.vue`, the Agent page on the module chat API

`AgentRoute.vue` chooses the host: `serverConfig.chat_backend === 'module'` renders
`AgentChat.vue`, anything else renders `Agent.vue`, and nothing renders until
`serverConfig` is a loaded object. Both hosts are lazy imports. `chat-host.mjs` holds
everything that does not need Vue, with tests in `frontend/tests/chatHost.test.mjs`.

**Client and controller.** `ChatClient({ baseUrl: pb.baseURL, getToken: () =>
pb.authStore.token })` reads the token per request, never once. The controller's
`storageKey` is `{ appUrl: pb.baseURL, mount: '/api/mcp/chat/v1', authCollectionId,
userId }`; `pb.authStore.onChange` calls `setAccount()` (and reopens the route's
conversation), and unmount calls `binding.unbind()` then `chat.dispose()`.

**Who owns what.**

| Owner | Properties |
|---|---|
| `bindAgentPanel(page, chat, { registerEntries: false })` | `entries`, `running`, `error`, `models`, `conversations`, `active`, `hasMore`, `heading`, `no-attachments`, `accept`, `max-file-size`, `max-files`, `contextUsage` (the 0.5 ring) |
| `AgentChat.vue` | `labels`, `formatDate`, `renamable`, `disabled`, `contextReferences`; the `mention` and `composer-actions` slots |

Nothing in the template binds a binding-owned property; two writers would fight. The
binding registers no entry renderers (`registerEntries: false`), so the host registers
the proposal card, the binding's `renderNoticeEntry` for `notice`, and
`renderContextMarkerEntry` for `context-marker`.

**The card and the entries choke point.** `ListPocketAgentPage` (in `lp-agent-page.js`)
has two overridden accessors, because they run on every binding write:

- `entries`: applies `withContextReferences(entries, chat.state.transcript)` (the stored
  `context` part of each user turn becomes `{ type, id, label }` chips before the
  binding's attachment chips) and copies `busy`/`running` into proposal entries, since
  the element only redraws an entry whose JSON changed (D3). `chat` is the controller.
- `error`: `hostErrorText()` swaps the binding's generic `provider_auth` /
  `usage_limit` lines for the app's wording ("sign in again under Settings → AI").

The proposal renderer looks the full proposal up in `chat.state.proposals` by
`entry.proposalId` and maps it with `cardProposalOf()`, so the card shows the legacy
look (operations, warnings, badge, Review/View, Open from `result.source.href`). Module
states map to `pending`, `applied` (`succeeded`), `failed`, `rejected`, `expired`,
`applying`, `approved`, `stale`, and `unknown`; `unknown` keeps its own warning line,
"may or may not have applied — check before retrying", and never reads as success or a
plain failure. `proposal.error` (a 409 conflict or a 422 refusal) is shown on the card.
Approve/Reject call `chat.decide(id, 'approve' | 'reject')`; a 409/422 resolves with
`{ error }`, which the card shows. Review opens the existing `ReviewModal` with
`reviewPartOf(proposal)` (a decided proposal opens read-only); applying edited
arguments calls `chat.decide(id, 'approve', { arguments })`. A `succeeded` approval
fires the `assistant-applied` window event with `{ tool, result }`.

**Page context.** `pageContext(path, references)` returns `{ screen: 'agent', path,
references }`; references are this message's `@` chips as `{ type, id, label }`,
files excluded. The controller reads `context` only after `newConversation` and
uploads, so a host `agent-submit` listener is registered *before*
`bindAgentPanel` and stores that submit's path and chips
(`createSubmitTracker`). A second submit is cancelled
(`preventDefault` and `stopImmediatePropagation`) until `context()` has read
the snapshot or the attempt has settled, so the chips cannot move to the wrong
turn. `context()` marks the snapshot consumed; `running` becoming true does
not. If the binding refuses the submit without calling `send`, the snapshot is
dropped on the next microtask. `tests/chatHost.test.mjs` pins the listener
order and the snapshot rules. `contextReferences` is the 30 most recent
distinct references newest-first, matching `chatAgentRefs` in `cmd/chat_module.go`.

**Composer.** `plain-text` and `no-add-menu` stay, so the Context/Paste controls and
the tests match the legacy page. The composer's `disabled` stays set until
`chat.state.status === 'ready'` (the same mechanism as no provider), because
`features` is null until `init()` finishes and a send before that would drop
the page context. An init error leaves it disabled; the module's own error
banner is the explanation. `@` uses `mention-trigger="@"`, `ContextPicker` in the
`mention` slot, and `agent-mention` / `agent-mention-key`; a pick calls `addReference`.
Paste data adds a text `File` through `page.addFiles()`. Accept and size limits are the
binding's, from `/status`: `AGENT_PAGE_ACCEPT` and `fileProblem` do not apply on this
host.

**Sending.** The draft and its chips are kept until `send()` settles. A turn in
the result means the server accepted it, and the draft stays cleared. A new
`state.error` (a different object from the one at submit time) with no turn
puts the draft and chips back. An error that was already set, including one
still present when the send's first `choice` change fires, does not. A rejected
promise is an upload or a precondition failure; the binding restores that draft
and its files. `attachSubmitTracking` is what hears the promise: the binding
does not hand it to the host.

**Rest.** `?ref=` pre-attaches on mount. Reference chips (not attachment chips) open
`ContextPreviewModal` on `agent-reference-open`; an attachment chip still downloads
through the binding. The empty state keeps the translated suggestions, title and help
text, and the composer its translated placeholder and labels. The model menu is the
element's own (`models` from `/status`); the
last pick is stored as the legacy `{ provider, model }` under
`localStorage['listpocket_agent_model']` and preselected when still offered. Selecting
a conversation, starting a new chat, and deleting the open conversation use
`router.push`. The id assigned to a draft by its first send uses `router.replace`,
so `/agent` does not stay as its own history step. A second state change for
that same id does not write again while the navigation is in flight, so a push
is not followed by a replace. The route watcher opens back/forward targets. A
change that arrives before the first `open()` resolves is applied once the page
is aligned, and that load is not written back over the URL. With no choices
and no models from `/status` once status is `ready`, the
page shows `agent.noProvider` and sets `disabled`, replacing the module's
generic 400. The conversation list's own Load more button (while `hasMore`,
labelled with `agent.loadMore`) fires `agent-load-more`, which the binding handles.

**Known differences from `Agent.vue`.** Cards follow the transcript (the binding's
order) rather than sitting inside the assistant message that proposed them. The server
returns only proposals that can still be decided, so an applied or rejected proposal
comes back as a notice line after reopening the conversation, without the card's Open
link. The binding builds conversation rows without `href`, so a modifier-click cannot
open a conversation in a new tab. The first and third are cosmetic; the second is the
one place a past result is less reachable than on the legacy page.

## `AIAssistant.vue`, the side panel host

- **Attributes and properties it sets:** `closable`, `sticky-attachments`, `plain-text`,
  `no-add-menu`, `accept`, the text attributes, `entries`, `suggestions`, `running`,
  `error`.
- **`entries`** is a `computed` that copies the transcript into plain objects. It reads
  every field shown (tool status and detail, proposals copied with JSON), so that changes
  made in place re-render the panel. It also puts `busy` and `running` into proposal
  entries.
- **Events it handles:**

  | Event | Response |
  |---|---|
  | `agent-submit` | `send(text, attachments)`, which reads `base64()` into `{ filename, mimeType, data }`. The event is cancelled when a turn is already running or there's no chat |
  | `agent-stop` | Cancels the turn |
  | `agent-close` | `assistantContext.open = false` |
  | `agent-action` | `apply` / `discard` → `decide(proposal, …)`; `review` → opens `ReviewModal` |
  | `agent-error-dismiss` | `error = ''` |

- **Slots it fills.** Each is a native wrapper element that carries the `slot` attribute:

  | Slot | Content |
  |---|---|
  | `header-start` | Chat switcher menu |
  | `header-actions` | New-chat button |
  | `subheader` | Model picker and the label of the page being looked at |
  | `transcript-end` | Undo, Resume and Retry |
  | `composer-chips` | Media-library pictures |
  | `composer-actions` | Media-library button |

- **Methods it calls through the ref:** `scrollToBottom({ force: true })` when switching
  chats, `clearAttachments()` when the signed-in account changes, and `attachments` for
  Retry.
- **Sticky attachments:** attachments stay in the composer after sending and go with
  every turn. The server is stateless between turns, so this is how follow-up questions
  still see a pasted screenshot.

## Checking a change

Run from `frontend/`:

```sh
npm run test:unit
npx vite build
npx vite --port 5199 --force &   # --force: re-bundle agent-panel if it changed
npx cypress run --spec cypress/e2e/assistant.cy.js,cypress/e2e/agent-page.cy.js \
  --config baseUrl=http://localhost:5199
```

To try package changes before releasing them, copy the package into `node_modules`
(`rsync -a --exclude .git /Volumes/mini512/Components/agent-panel/ node_modules/agent-panel/`)
and restart Vite with `--force`. Afterwards, release a tag and install it, which restores
`package.json` and the lockfile to a pinned version.

Known state: the test "streams replies, reviews targeted changes and supports undo across
navigation" fails before and after the panel was introduced. It waits for the text
`/analytics`, which the test page never shows. Don't treat it as a regression signal.
