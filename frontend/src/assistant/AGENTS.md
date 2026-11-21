# AGENTS.md: the assistant side panel and the Agent page

The assistant side panel (`<lp-agent-panel>`) and the full Agent page (`<lp-agent-page>`)
are both built on the **`agent-panel`** package
(https://github.com/compdani/agent-panel), pinned to v0.5.1 in `frontend/package.json`,
and backed by **`@pocketbase-mcp/chat-client`** (v0.1.9) against `/api/mcp/chat/v1`.
The package's own contract is in `node_modules/agent-panel/AGENTS.md`, and its human
guide is in `node_modules/agent-panel/README.md`. Read the contract before changing
anything here. This file covers only how List Pocket extends and hosts the two elements.

## Rules

1. **Never edit `node_modules/agent-panel` or `node_modules/@pocketbase-mcp/chat-client`.**
   - To change behaviour that any app could use, change the package repo, release a
     new tag, then update the pin here.
   - Behaviour specific to List Pocket goes in `lp-agent-panel.js`, `lp-agent-page.js`,
     `chat-host.mjs`, `chat-ui.mjs`, `AIAssistantChat.vue`, or `AgentChat.vue`.
2. **Import by package name**: `agent-panel/element` (panel class, `h`, `icon`) and
   `agent-panel/page-element` (page class). These define no tags. Never import the
   package's files by path.
3. **Register the tags with Vue's compiler.** `vite.config.js` lists `agent-panel`,
   `agent-page`, `lp-agent-panel` and `lp-agent-page` in
   `compilerOptions.isCustomElement`. A new custom-element tag must be added there.
4. **Shared behaviour goes in `lp-agent-common.js` (element theme/cards),
   `chat-host.mjs` (pure logic), and `chat-ui.mjs` (binding/DOM glue).**

## Files

| File | Role |
|---|---|
| `lp-agent-common.js` | `LP_STYLES` (Vuetify theme mapping and card CSS), `LP_LABELS`, `renderMarkdown`, `renderProposal(el, entry)`, `referenceIcon(ref)` (`@mdi/js` paths per context type) |
| `lp-agent-panel.js` | `ListPocketAgentPanel` (`<lp-agent-panel>`): adds `toolLabel` and proposal `busy`/`running` flags on every binding write |
| `lp-agent-page.js` | `ListPocketAgentPage` (`<lp-agent-page>`): adds proposal `busy`/`running` flags on every binding write |
| `chat-host.mjs` | Pure logic for both hosts: page context, history writes, stored `@` chips, the "In context" list, proposal card and ReviewModal inputs, error wording, remembered model, tool labels, legacy `localStorage` cleanup, media pictures and the editor adapter |
| `chat-ui.mjs` | DOM glue both hosts share: submit-reference capture, the composer hold until `/status`, proposal and notice renderers, `decideModuleProposal` |
| `../views/AgentChat.vue` | The `/agent/:id?` route: `ChatClient` + `ChatController` + `bindAgentPanel`, `@` context picker, Paste data, proposal decisions, `ReviewModal` |
| `../components/AIAssistantChat.vue` | The side panel mounted in `App.vue`: `ChatController` + `bindAgentPanel` on `<lp-agent-panel>`, editor adapter, undo, retry, media pictures |
| `markdown.mjs`, `patches.mjs`, `context.js` | Markdown rendering, editor patch application/revisioning, and active editor registration |
| `../../cypress/e2e/assistant-chat.cy.js` + `fixtures/assistant-chat-harness.*` | Browser tests for the side panel: conversations, page context, streams, proposals, editor apply and undo, wrong-editor pre-check, media pictures, no provider, overlapping sends, refused draft, status gate, retry, legacy storage cleanup |
| `../../cypress/e2e/agent-page-chat.cy.js` + `fixtures/agent-page-chat-harness.*` | Browser tests for the Agent page: URL loading, `@` context, streams, proposals, refused decisions, no provider, rename/delete, paste upload, overlapping sends, failed-send restore, status gate, history, Load more, inline markdown |

## `<lp-agent-panel>` and `<lp-agent-page>`

Both subclasses share the following from `lp-agent-common.js`:

- `renderMarkdown` uses `renderAssistantMarkdown` (markdown-it with `html: false`, images
  disabled, and links opening in a new tab with `rel=noopener`).
- User bubbles keep agent-panel's own inline markdown (`inlineMarkdownToHTML`
  in `renderText`), so `**x**` reads as bold there too. This is deliberate;
  don't override `renderText` to make them plain.
- The theme tokens map to Vuetify's `--v-theme-*` variables, with fallbacks.
- `referenceIcon` maps the context types in `CONTEXT_ICONS` (`agent/parts.mjs`) to
  `@mdi/js` paths. Add new types in both places.
- Both hosts use agent-panel's rich-text composer (`.input--rich`, bold/italic sent as
  markdown) and built-in "+" menu: the Agent page keeps the defaults (`attach`, `paste`,
  `context` via `mention-trigger="@"`), and the side panel passes `['attach', 'paste',
  { id: 'media', label: 'Add a picture from media', icon: 'image' }]` to open the media
  library and attach picture links as `pictureReferences`.
- Reasoning effort options appear automatically in the built-in options menu whenever the
  selected model choice from `/api/mcp/chat/v1/status` lists `efforts`.
- `defaultLabels` adds `review`, `view`, `discard`, `open` and `proposedEdit`. The side
  panel uses the English defaults; the Agent page passes translated ones.
- Both subclasses override the `entries` setter to copy host-owned `busy`/`running` flags
  onto proposal entries (`lp-agent-panel.js` also fills in `toolLabel` on tool rows).
  `repaint()` reassigns `this.rawEntries` so busy states show immediately.

## `AgentChat.vue`, the Agent page

**Client and controller.** `ChatClient({ baseUrl: pb.baseURL, getToken: () =>
pb.authStore.token })` reads the token per request, never once. The controller's
`storageKey` is `{ appUrl: pb.baseURL, mount: '/api/mcp/chat/v1', authCollectionId,
userId }`; `pb.authStore.onChange` calls `setAccount()` only when the account actually
changes (ignoring same-user token refreshes), and unmount calls `binding.unbind()` then
`chat.dispose()`.

**Who owns what.**

| Owner | Properties |
|---|---|
| `bindAgentPanel(page, chat, { registerEntries: false, errorText, contextReferences })` | `entries`, `running`, `error`, `models`, `conversations`, `active`, `hasMore`, `heading`, `no-attachments`, `accept`, `max-file-size`, `max-files`, `contextUsage` (the 0.5 ring), and `disabled` while `features.ready` is false |
| `AgentChat.vue` | `labels`, `formatDate`, `renamable`, `contextReferences` (the "In context" list); the `mention` and `composer-actions` slots. It also holds `disabled` until `/status` has answered |

Nothing in the template binds a binding-owned property; two writers would fight. The
binding registers no entry renderers (`registerEntries: false`), so the host registers
the proposal card (`renderModuleProposal`), `renderNoticeWithOpen` for `notice`, and
`renderContextMarkerEntry` for `context-marker`.

**Proposals.** `cardProposalOf()` maps module states to `pending`, `applied` (`succeeded`),
`failed`, `rejected`, `expired`, `applying`, `approved`, `stale`, and `unknown`;
`unknown` keeps its warning line and never reads as success or plain failure. Approve/Reject
call `chat.decide(id, 'approve' | 'reject')`; a 409/422 resolves with `{ error }`, shown on
the card. Review opens `ReviewModal` with `reviewPartOf(proposal)` (read-only once decided).
A `succeeded` approval dispatches the `assistant-applied` window event with `{ tool, result }`.

**Page context & composer.** `pageContext(path, references)` sends `{ screen: 'agent', path,
references }` (captured in an `agent-submit` listener registered before the binding so
attachments don't strip references). `contextReferencesFromTranscript` computes the "In
context" list (30 most recent distinct references newest-first). `?ref=` pre-attaches on
mount; reference chips open `ContextPreviewModal`, attachment chips download through the
binding. Model picks persist under `localStorage['listpocket_agent_model']`.

## `AIAssistantChat.vue`, the side panel

Mounted directly in `App.vue` (not on auth pages). On mount, `clearLegacyAssistantStorage`
removes `listpocket_assistant_v1:*` and `listpocket_ai_builder_legacy_owner` from
`localStorage` while keeping `listpocket_agent_model`, `listpocket_assistant_pictures_v1:*`,
and `listpocket_ai_builder_threads_v1` (still used by `TemplateForm.vue`'s AI builder dialog).

**Conversations & rename.** Uses the module's server-backed conversation list. Rename is
a host button in `switcher-end` calling `chat.rename()`, since `bindAgentPanel` only
listens for `agent-rename` on a page element. Deleting a conversation clears its stored
pictures and active undo state.

**Page context.** Built at send time from the current route: `screen: "panel"`, `path`,
`page`, `params`, `filters`, the open editor (`editorContextOf`), media pictures
(`pictureReferences`), and any `@` references.

**Editor adapter & undo.** `snapshot()` returns the open editor (`editorId` = `editor.key`,
revision from `patches.mjs`), or null. Before calling `chat.decide`, `approve()` checks
`editorPrecheckError`: if no editor or a different editor is open, it shows "Open the
original editor before applying this proposal." and leaves the proposal pending so the
user can return to the original editor and apply it. Inside the adapter, `validatePatch`
reports stale revisions as `stale` and refuses a mid-flight wrong editor; `applyPatch`
applies via `editor.apply` and returns the new revision. The controller posts
`/proposals/{id}/outcome`. Undo restores `before` while the same editor is open and
unchanged; the next turn, switching/deleting the conversation, or switching editor clears
it.

**Pictures & Retry.** Media pictures persist per user and conversation in
`localStorage['listpocket_assistant_pictures_v1:<userId>']`; draft pictures move onto the
created conversation id on first send. Retry is offered only for a failed accepted turn
(`state.turn.error` with no active pre-accept `state.error`) and resends that turn's
message, stored upload IDs, context references, and the selected `model` and `effort`.
When `/status` fails or reports `features.ready === false`, opening the panel or regaining
window focus re-runs `chat.init()`. Changing Settings → AI (`ai-settings-updated`) always
refreshes the catalog, including when the chat is already ready, so model capabilities
and reasoning effort options update on both chat surfaces.

## Checking a change

Run from `frontend/`:

```sh
npm run test:unit
npx vite build
npx vite --port 5199 --force &   # --force: re-bundle agent-panel if it changed
env -u ELECTRON_RUN_AS_NODE npx cypress run \
  --spec cypress/e2e/assistant-chat.cy.js,cypress/e2e/agent-page-chat.cy.js \
  --config baseUrl=http://localhost:5199
```
