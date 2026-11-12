# AGENTS.md: the assistant side panel

The assistant side panel is built on the **`agent-panel`** package
(https://github.com/compdani/agent-panel). It's installed from GitHub and pinned to a tag
in `frontend/package.json`. The package's own contract is in
`node_modules/agent-panel/AGENTS.md`, and its human guide is in
`node_modules/agent-panel/README.md`. Read the contract before changing anything here.
This file covers only how List Pocket extends and hosts the panel.

## Rules

1. **Never edit `node_modules/agent-panel`.**
   - To change behaviour that any app could use, change the package repo
     (`/Volumes/mini512/Components/agent-panel` locally), release a new tag, then update
     the pin here with `npm install github:compdani/agent-panel#vX.Y.Z`.
   - Behaviour specific to List Pocket goes in `lp-agent-panel.js` (a subclass) or
     `AIAssistant.vue` (the host).
2. **Import the panel by package name**: `agent-panel/element` for the class and `h()`;
   `agent-panel` defines the plain `<agent-panel>` tag. Never import the files by path.
3. **Register the tag with Vue's compiler.** `vite.config.js` lists `lp-agent-panel` and
   `agent-panel` in `compilerOptions.isCustomElement`. A new custom-element tag must be
   added there.

## Files

| File | Role |
|---|---|
| `lp-agent-panel.js` | `ListPocketAgentPanel` (`<lp-agent-panel>`) and `ASSISTANT_ACCEPT` |
| `../components/AIAssistant.vue` | The host: chats kept in the browser, streaming turns, proposal decisions, and the Vuetify controls placed in slots |
| `markdown.mjs`, `threads.mjs`, `transcript.mjs`, `patches.mjs`, `client.js`, `context.js` | Conversation logic, independent of the panel |
| `../../cypress/e2e/assistant.cy.js` | Browser tests for the side panel, including paste, drop and keeping attachments for the next message |

## `<lp-agent-panel>`

- `renderMarkdown` uses `renderAssistantMarkdown` (markdown-it with `html: false`, images
  disabled, and links opening in a new tab with `rel=noopener`).
- The theme tokens map to Vuetify's `--v-theme-*` variables, with fallbacks.
- It adds the entry type `{ id, type: 'proposal', proposal, busy: boolean, running:
  boolean }`. `proposal` follows the server's proposal object: `id`, `tool`, `status`,
  `arguments` (with an optional `operations[]`), `warnings[]`, `result.source.href`.
  - The card shows the title from `proposalTitle()`, the kind from `proposalInfo()`, a
    status badge once the proposal is no longer pending, any warnings, and each
    operation's before and after text.
- Proposal buttons call `emitAction(action, { id: proposal.id })`:
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

## `AIAssistant.vue`, the host

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
npx vite --port 5199 &
npx cypress run --spec cypress/e2e/assistant.cy.js --config baseUrl=http://localhost:5199
```

Known state: the test "streams replies, reviews targeted changes and supports undo across
navigation" fails before and after the panel was introduced. It waits for the text
`/analytics`, which the test page never shows. Don't treat it as a regression signal.
