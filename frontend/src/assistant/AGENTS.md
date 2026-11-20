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
| `chat-host.mjs` | Pure logic for both module hosts: page context, history writes, stored `@` chips, the "In context" list, proposal card and ReviewModal inputs, error wording, remembered model, and (side panel) media pictures and the editor adapter |
| `chat-ui.mjs` | DOM glue both module hosts share: submit-reference capture, the composer hold until `/status`, proposal and notice renderers, `decideModuleProposal` |
| `../views/AgentRoute.vue` | The `/agent/:id?` route: renders `Agent.vue` or `AgentChat.vue` from `serverConfig.chat_backend`, and nothing until `/api/config` has answered |
| `../views/AgentChat.vue` | The module host: `ChatClient` + `ChatController` + `bindAgentPanel`, composer controls, proposal decisions, `ReviewModal` |
| `../views/Agent.vue` | The Agent page on the legacy engine: server-side conversations, turns, uploads, context picker, proposal decisions |
| `../components/AIAssistantHost.vue` | Picks `AIAssistant.vue` or `AIAssistantChat.vue` from `serverConfig.chat_backend`, and renders nothing until `/api/config` has answered |
| `../components/AIAssistantChat.vue` | The module side panel: `ChatController` + `bindAgentPanel` on `<lp-agent-panel>`, editor adapter, media pictures |
| `../components/AIAssistant.vue` | The legacy side panel: chats kept in the browser, streaming turns, proposal decisions, and the Vuetify controls placed in slots |
| `markdown.mjs`, `threads.mjs`, `transcript.mjs`, `patches.mjs`, `client.js`, `context.js` | Conversation logic, independent of the panel |
| `../../cypress/e2e/assistant.cy.js` | Browser tests for the legacy side panel, including paste, drop and keeping attachments for the next message |
| `../../cypress/e2e/assistant-chat.cy.js` + `fixtures/assistant-chat-harness.*` | Browser tests for the module side panel: conversations, page context, streams, proposals, editor apply and undo, media pictures, no provider, overlapping sends, a refused draft, the status gate, retry |
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
  menu. Every host keeps the old `<textarea>` and paperclip: the module Agent page
  because its composer controls (Context, Paste data) and the browser tests are the
  legacy ones, and the module side panel because it matches the legacy panel.
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
`chat-ui.mjs` holds the DOM glue this host shares with the module side panel.

**Client and controller.** `ChatClient({ baseUrl: pb.baseURL, getToken: () =>
pb.authStore.token })` reads the token per request, never once. The controller's
`storageKey` is `{ appUrl: pb.baseURL, mount: '/api/mcp/chat/v1', authCollectionId,
userId }`; `pb.authStore.onChange` calls `setAccount()` (and reopens the route's
conversation), and unmount calls `binding.unbind()` then `chat.dispose()`.

**Who owns what.**

| Owner | Properties |
|---|---|
| `bindAgentPanel(page, chat, { registerEntries: false, errorText, contextReferences })` | `entries`, `running`, `error`, `models`, `conversations`, `active`, `hasMore`, `heading`, `no-attachments`, `accept`, `max-file-size`, `max-files`, `contextUsage` (the 0.5 ring), and `disabled` while `features.ready` is false |
| `AgentChat.vue` | `labels`, `formatDate`, `renamable`, `contextReferences` (the "In context" list); the `mention` and `composer-actions` slots. It also holds `disabled` until `/status` has answered |

Nothing in the template binds a binding-owned property; two writers would fight. The
binding registers no entry renderers (`registerEntries: false`), so the host registers
the proposal card, the binding's `renderNoticeEntry` for `notice`, and
`renderContextMarkerEntry` for `context-marker`.

**The card and the entries choke point.** `ListPocketAgentPage` (in `lp-agent-page.js`)
overrides the `entries` setter, because it runs on every binding write. It copies
`busy`/`running` into proposal entries: those flags are host state (a decision in
flight, a turn still running), and the element only redraws an entry whose JSON
changed. Stored `@` chips are not added here. `bindAgentPanel`'s `contextReferences`
option (`referencesFromContext`) does that from each user message's `context` part.
`chat` is the controller.

`errorText` passed to `bindAgentPanel` maps `provider_auth` and `usage_limit` to
the app's wording ("sign in again under Settings → AI") and `no_provider` to
`agent.noProvider`. The element does not rewrite the error string itself.

The proposal renderer uses `entry.proposal` (arguments, warnings, result) and maps
it with `cardProposalOf()`, so the card shows the legacy look (operations, warnings,
badge, Review/View, Open from `result.source.href`). The binding omits the
controller's `error` (a 409 or a 422) from `entry.proposal`, so that one field is
still read from state. Module
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
files excluded. The binding calls `readContext()` synchronously at the start of
its `agent-submit` handler, before the element clears the composer, so
`context()` reads `route.fullPath` and the element's references directly. A
second submit is refused by the binding while the first has not been accepted
(`state.sending`). `contextReferences` (the element's "In context" list) is the
30 most recent distinct references newest-first, from
`contextReferencesFromTranscript`, matching `chatAgentRefs` in `cmd/chat_module.go`.
The module does not build that list.

**Composer.** `plain-text` and `no-add-menu` stay, so the Context/Paste controls and
the tests match the legacy page. The composer's `disabled` stays set until
`chat.state.status === 'ready'`. The binding does not disable the composer while
`features` is still null, and a send before `/status` waits inside the controller;
holding it closed keeps the old "nothing to type yet" behaviour. Once status has
answered, `disableWhenNotReady` owns `disabled`: it turns the composer off while
`features.ready` is false and will not turn a composer back on unless it disabled
it. An init error leaves it disabled; the module's own error banner is the
explanation. `@` uses `mention-trigger="@"`, `ContextPicker` in the
`mention` slot, and `agent-mention` / `agent-mention-key`; a pick calls `addReference`.
Paste data adds a text `File` through `page.addFiles()`. Accept and size limits are the
binding's, from `/status`: `AGENT_PAGE_ACCEPT` and `fileProblem` do not apply on this
host.

**Sending.** The binding keeps the draft and its chips until the turn is
accepted, and puts them back after any failure before that (`restoreDraft`,
the default). An upload failure keeps the files as well. `onSend` is unused:
nothing on the page needs the outcome beyond what the binding already does.

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
is aligned, and that load is not written back over the URL. While
`features.ready` is false the binding disables the composer and shows
`agent.noProvider` in its error banner (through `errorText`); the page adds no
notice of its own. The page context's references come from the `agent-submit`
event, captured by a listener registered before the binding: with files
attached the binding clears the composer's chips before it reads the context. The
conversation list's own Load more button (while `hasMore`, labelled with
`agent.loadMore`) fires `agent-load-more`, which the binding handles.

**Known differences from `Agent.vue`.** Cards follow the transcript (the binding's
order) rather than sitting inside the assistant message that proposed them. After
a conversation is reopened, a decided proposal is a `proposal_outcome` notice
rather than a card; the notice carries `result`, and the page draws the same
Open link (`result.source.href`) the applied card shows. The binding builds
conversation rows without `href`, so a modifier-click cannot open a conversation
in a new tab. Both of those are cosmetic.

## `AIAssistantChat.vue`, the side panel on the module chat API

`AIAssistantHost.vue` chooses the host the way `AgentRoute.vue` does:
`serverConfig.chat_backend === 'module'` renders `AIAssistantChat.vue`, anything
else renders `AIAssistant.vue`, and nothing renders until `serverConfig` is a
loaded object. `App.vue` mounts the host. The legacy Cypress harness imports
`AIAssistant.vue` directly, so that suite stays on the legacy engine.

The host follows `AgentChat.vue`. `chat-ui.mjs` holds the glue both use
(the submit's `@` chips, the composer hold until `/status`, the proposal card,
the notice's Open link, and `decideModuleProposal`). `chat-host.mjs` holds the
rest, including `panelPageContext`, the picture store and `createEditorAdapter`.

**Who owns what.** `bindAgentPanel(panel, chat, { registerEntries: false,
errorText, contextReferences })` owns the same properties as on the Agent page.
The host sets `labels` only through the element's defaults (the legacy panel is
English), `formatDate`, the media-picture chips, Undo, Retry, and the
`switcher-end` slot (Rename, Open the full Agent page). It holds `disabled`
until `/status` has answered. `plain-text` and `no-add-menu` stay, so the
composer keeps the textarea and the paperclip.

**Conversations** are the module's list. The binding syncs the switcher
(new, switch, delete). Rename is a host button that calls `chat.rename()`:
`bindAgentPanel` only listens for `agent-rename` on a page element, and
`<lp-agent-panel>` is not one. There is no localStorage thread list and no
"Import previous campaign chats".

**Page context** is built when the turn is sent, from the route at that moment:
`screen: "panel"`, `path`, `page`, `params`, `filters`, the open editor, the
conversation's media pictures, and the message's `@` references. The legacy
panel has no `@` picker, Paste data or `?ref=` pre-attach, and this host does
not add them.

**Editor adapter.** `snapshot()` is the open editor (`editorId` is `editor.key`,
revision from `patches.mjs`), or null when none is open. Before calling
`chat.decide`, the host checks `editorPrecheckError`: if no editor or a
different editor is open, it shows "Open the original editor before applying
this proposal." and leaves the proposal pending so the user can return to the
original editor and apply it. Inside the adapter, `validatePatch` returns not
applicable (`stale`) when no editor is open or `baseRevision` is stale, and
throws the same message when `editorId` is not the open editor. `applyPatch`
applies with `editor.apply` and returns the new revision. The controller reports
the outcome; the host does not call `reportOutcome`. Undo is a button that
restores the `before` snapshot while the editor is still that one and unchanged
since. The next turn, switching/deleting the conversation, or switching editor
clears it. A further edit makes Undo report "The editor changed after this edit;
use its undo controls."

`ListPocketAgentPanel` copies `busy`/`running` onto proposal entries and adds
`toolLabel` when `this.chat` is set (the module host). With `chat` left null,
the legacy host's entries pass through unchanged. Its `renderEntry` still draws
a legacy proposal card (no `proposalId`); a module entry uses the renderer
registered with `defineEntry`.

**Pictures** from "Add a picture from media" go in `context.pictureReferences`
on every turn and are kept in `localStorage` (`listpocket_assistant_pictures_v1:<userId>`),
keyed by conversation id. A draft's pictures move to the new id on its first
send. Storage failures are swallowed.

**Retry.** After a failed turn the host resends the last user message with
`chat.send`. The controller reconnects a live stream itself, and it has no
resume method, so there is no Resume button.

**Model.** The element's model menu, with the same `listpocket_agent_model`
memory as the Agent page (`choiceIdFor` / `storedModelFor`).

**Known differences from `AIAssistant.vue`.** Conversations live on the server.
The model is one remembered choice, not one per thread. Attachment chips do not
stay in the composer after a send that uploaded them (the binding removes
them); the module keeps the files on the conversation. Cards follow the
transcript. A reopened decided proposal is a notice with the same Open link.
`WorkflowBuilder.vue` does not register an editor, so a workflow canvas is not
an open editor until it does; the adapter applies whatever
`assistantContext.editor` is set to.

## `AIAssistant.vue`, the legacy side panel host

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
env -u ELECTRON_RUN_AS_NODE npx cypress run \
  --spec cypress/e2e/assistant-chat.cy.js,cypress/e2e/assistant.cy.js,cypress/e2e/agent-page-chat.cy.js,cypress/e2e/agent-page.cy.js \
  --config baseUrl=http://localhost:5199
```

To try package changes before releasing them, copy the package into `node_modules`
(`rsync -a --exclude .git /Volumes/mini512/Components/agent-panel/ node_modules/agent-panel/`)
and restart Vite with `--force`. Afterwards, release a tag and install it, which restores
`package.json` and the lockfile to a pinned version.

Known state: the test "streams replies, reviews targeted changes and supports undo across
navigation" fails before and after the panel was introduced. It waits for the text
`/analytics`, which the test page never shows. Don't treat it as a regression signal.
