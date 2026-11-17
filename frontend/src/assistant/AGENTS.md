# AGENTS.md: the assistant side panel and the Agent page

The assistant side panel (`<lp-agent-panel>`) and the full Agent page (`<lp-agent-page>`)
are both built on the **`agent-panel`** package
(https://github.com/compdani/agent-panel). It's installed from GitHub and pinned to a tag
in `frontend/package.json`. The package's own contract is in
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
| `lp-agent-page.js` | `ListPocketAgentPage` (`<lp-agent-page>`) and `AGENT_PAGE_ACCEPT`; adds the server's 2 MB limit for text files through `fileProblem` |
| `models.js` | `useAgentModels()` (loads `agent/models` into the panel's built-in model menu: one entry per provider model, grouped by provider, provider errors as disabled entries), `modelKey(ai)` and `modelFromKey(id)` (menu ids are `provider/model`) |
| `../views/Agent.vue` | The Agent page host: server-side conversations, turns, uploads, context picker, proposal decisions |
| `../components/AIAssistant.vue` | The host: chats kept in the browser, streaming turns, proposal decisions, and the Vuetify controls placed in slots |
| `markdown.mjs`, `threads.mjs`, `transcript.mjs`, `patches.mjs`, `client.js`, `context.js` | Conversation logic, independent of the panel |
| `../../cypress/e2e/assistant.cy.js` | Browser tests for the side panel, including paste, drop and keeping attachments for the next message |
| `../../cypress/e2e/agent-page.cy.js` + `fixtures/agent-page-harness.*` | Browser tests for the Agent page: conversations, references, `@` picker, paste and upload, rename and delete, narrow-screen drawer |

## `<lp-agent-panel>` and `<lp-agent-page>`

Both subclasses do the following (from `lp-agent-common.js`):

- `renderMarkdown` uses `renderAssistantMarkdown` (markdown-it with `html: false`, images
  disabled, and links opening in a new tab with `rel=noopener`).
- The theme tokens map to Vuetify's `--v-theme-*` variables, with fallbacks.
- `referenceIcon` maps the context types in `CONTEXT_ICONS` (`agent/parts.mjs`) to
  `@mdi/js` paths. Add new types in both places.
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
  the key. A pick calls `page.addReference()`. **Context** in the page's built-in "+"
  menu (agent-panel 0.3.0) opens the picker without typing `@`.
- **Paste data** is the "+" menu's built-in paste form (labels from `agent.pasteData`,
  `agent.pasteName`, `agent.pasteContent`); it adds a text `File` whose type follows the
  extension (`leads.csv` → `text/csv`).
- **The model** is the page's built-in model menu: `:models` / `:model` /
  `:default-model` from `useAgentModels()`, and `agent-model-change` → `setModel`.
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

## `AIAssistant.vue`, the side panel host

- **Attributes and properties it sets:** `closable`, `sticky-attachments`, `accept`, the
  text attributes, `entries`, `suggestions`, `running`, `error`.
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
  | `subheader` | The label of the page being looked at (hidden when there is none) |
  | `transcript-end` | Undo, Resume and Retry |
  | `composer-chips` | Media-library pictures |

- **Built-in composer menus (agent-panel 0.3.0):** the model menu (`useAgentModels()`,
  `agent-model-change` → `setModel`) and the "+" menu, set with `:addMenu` to attach
  files, paste text, and "Add a picture from media" (handled in `agent-menu-select`,
  which opens the media library).

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

The composer is agent-panel's rich-text editor (a `contenteditable`, not a
`textarea`): tests type into `.input--rich` and read the text from the element's
`draft` property.

Known state: the test "streams replies, reviews targeted changes and supports undo across
navigation" fails before and after the panel was introduced. It waits for the text
`/analytics`, which the test page never shows. Don't treat it as a regression signal.
