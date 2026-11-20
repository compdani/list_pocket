# Pocketbase-MCP: suggested module changes

This list comes from moving List Pocket's assistant onto the module (v0.1.6). Each
item says what happens today, where the code is, what to change, and the List Pocket
workaround the change would let us delete. Paths are relative to the module root at
v0.1.6, and the adapter files are under `adapters/pocketbase/`.

They're ordered by how much they matter to List Pocket.

## Status in v0.1.8

List Pocket runs pocketbase-mcp v0.1.8 (and `@pocketbase-mcp/chat-client` 0.1.8,
agent-panel v0.5.1). v0.1.7 shipped items 1 to 11, and v0.1.8 closed what was left
of them. The workarounds those items named are gone:

- **#1** `disableWhenNotReady` disables the composer and shows the banner, with
  `errorText` mapping `no_provider` to List Pocket's sentence; the page has no
  alert of its own.
  The page still disables it until `/status` has answered, which the binding
  does not.
- **#2 and #3** No submit tracker. The controller reads `context()` when
  `send()` is called, the binding refuses a second submit (`state.sending`),
  and `restoreDraft` puts the draft and references back after a pre-accept failure.
- **#5** Chat routes are per HTTP method. The public home route is `GET /` again.
- **#6** A reopened `proposal_outcome` notice carries `result`, and the page
  draws the Open link from `result.source.href`.
- **#7** A write that may already have applied returns `pbmcp.OutcomeUnknown`.
  Validation and permission errors are unchanged. A 4xx conflict stays a
  definite failure.
- **#8** The card and the review modal use `entry.proposal`. The controller's
  `error` (a 409 or 422) is not on that object, so that one field is still
  read from state.
- **#9** `contextReferences` supplies the stored `@` chips. The "In context"
  list is still `contextReferencesFromTranscript`: the module has no equivalent.
- **#10** `errorText` maps `provider_auth` and `usage_limit`.
- **#11** `ChatOptions.ToolTimeout` is set to the same five minutes as
  `DecisionTimeout`.
- **#12** The client is the installed package. Nothing is vendored.

Still kept, because the module does not cover them: the busy/running flags on
proposal cards (the element redraws only when an entry's JSON changes), the
"In context" list, the composer hold until `/status` answers, and a listener
that captures the submit's `@` references (see below).

## Found while adopting v0.1.8

**A. Context read after the binding clears the chips (attachments).** In
`bindAgentPanel`'s `onSubmit` (`frontend/client/agent-panel.mjs`, ~1455), a
submit with files calls `element.clearComposer({ draft, attachments: false })`,
which also clears the references, and only then `deliver()`, which calls
`readContext()`. A host whose `context()` reads the element's references gets
none for a message with an attachment. Without files, `deliver()` runs inside
the handler before the element clears anything, so it works. Suggested: call
`readContext()` at the top of `onSubmit`, before any clearing, and pass it to
`deliver()`; or pass the submit's `detail.references` to the context provider.
List Pocket captures `detail.references` in an `agent-submit` listener
registered before the binding's.

**B. `disableWhenNotReady` leaves the composer enabled before `/status`.** It
only disables once `features.ready === false`, so the composer is usable while
`features` is still null, and a send then waits on `/status`. It also never
re-enables a composer the host disabled, so a host that holds it until
`/status` answers has to release it itself. Suggested: treat `features === null`
as not ready yet (disable without the `no_provider` line).

**C. `contextReferences` drops chips without a label.** A stored reference that
only has `type` and `id` disappears. Suggested: fall back to the id as the
label. List Pocket does that in `referencesFromContext`.

agent-panel v0.5.1 ships `COMPATIBILITY.md`. 0.5.1 itself changes nothing a
host has to do.

## 1. "No provider configured" should have its own error code

**Today.**
- A turn submitted when no provider or model is configured fails with `400 invalid_request` and the generic invalid-request message.
- `chat/http.go:1686` puts `ErrNoModelConfigured` in the same branch as `ErrModelUnknown`, `ErrModelAmbiguous` and `ErrEffortUnsupported`.
- This happens before `Setup` runs, so the host can't replace the message either.
- The binding then shows generic text, and the user can't tell "AI isn't set up" from a bad request.

**Suggested.**
- Give `ErrNoModelConfigured` its own code, for example `409 no_provider` or `503 provider_unavailable`, with a clear default message such as "No AI model is configured."
- Keep `invalid_request` for the other three, which really are bad requests.
- Add the new code to the client's `ChatApiError` handling and to `errorText()` in `frontend/client/agent-panel.mjs`. With no host code, the binding then says "AI isn't set up. Ask an administrator to configure a model."
- Optional: have `/status` report it explicitly, for example `features.ready: false` with `reason: "no_provider"`. Clients then don't have to infer it from empty `choices` and `models`.

**List Pocket today.**
- `AgentChat.vue` treats `status === 'ready'` with empty `models` and `choices` as "not set up". It shows `agent.noProvider` and disables the composer.
- That's inference: a status that briefly lists nothing would look the same. With a code, or `features.ready`, the host can use it directly.

## 2. Read the page context when `send()` is called, not after uploads

**Today.**
- `ChatController._send` (`frontend/client/controller.mjs:1712-1723`) calls the host's `context()` only after `newConversation()` (in the binding) and after attachment uploads have been awaited.
- It sets `running: true` only after that.
- Meanwhile the composer still accepts another submit. Whatever the host's `context()` reads then belongs to the next message, not this one.

**Suggested.**
- Call `context()` synchronously at the top of `send()`, before any `await`, and keep the value for that send and its retries.
- Do the same for anything else read from the page.
- Also set a `sending` (or `running`) flag synchronously at the start of `send()`, so the binding can refuse a second submit while the first is still creating its conversation or uploading.

**List Pocket today.**
- `createSubmitTracker` in `frontend/src/assistant/chat-host.mjs` snapshots `{ path, references }` in an `agent-submit` listener registered before the binding's.
- It blocks a second submit until `context()` has consumed the snapshot.
- All of that goes away once the controller reads the context at call time.

## 3. Tell the host how a send ended

**Today.**
- The binding calls `controller.send()` from its own `agent-submit` handler and doesn't report the outcome. That outcome is one of: accepted, failed before accept, or upload refused.
- `state.error` is the only signal, and it can be stale. An error left from an earlier send is still set when the next send fires its first `change`.

**Suggested.** Do either or both of these:
- Dispatch an event on the element or controller after each submit, for example `agent-sent` with `{ outcome: 'accepted' | 'failed' | 'refused', turnId?, error? }`.
- Add a `bindAgentPanel` option, `onSubmitOutcome(detail)`.

Also clear `state.error` synchronously at the start of `send()`, so a stale error can't be mistaken for this send's failure.

**List Pocket today.**
- `attachSubmitTracking` (chat-host.mjs) wraps `controller.send` and `controller.newConversation` on the instance to see the promises.
- It compares error objects by identity to ignore stale errors.
- The binding restores the draft only for upload failures. List Pocket restores it after any failure that happens before the server accepts the turn, which is the legacy behaviour.
- A built-in outcome event would let us delete the wrapper. Restoring the draft for every pre-accept failure is arguably the right default for the binding too.

## 4. Don't drop the context when a send happens before `/status`

**Today.**
- `features` is `null` until `init()` finishes, and `_send` attaches context only when `features.context` is true (`controller.mjs:1721`).
- A send made before `/status` answers goes out with no context, silently.

**Suggested.**
- Make `send()` wait for the first `/status` (or `init()`) before deciding.
- Alternatively, have the binding keep the composer disabled until `status === 'ready'`.

**List Pocket today.** The host disables the composer until the status is `ready`.

## 5. Register chat routes per method

**Today.**
- `adapters/pocketbase/chat.go:426` mounts the chat prefix with `r.Route("", path, ...)`, which matches every method.
- Go's `ServeMux` refuses that pattern next to a host's `GET /` catch-all. With chat enabled, List Pocket's `serve` panicked at startup:
  `pattern "/api/mcp/chat/v1" ... conflicts with pattern "GET /"`.

**Suggested.**
- Register only the methods the surface uses (GET, POST, PATCH, DELETE), for example by looping over the methods.
- Alternatively, register under a `{path...}` wildcard per method.
- Either way, check for conflicts when routes are registered and return an error instead of panicking when `BuildMux` runs.

**List Pocket today.**
- With the module on, the public home page is registered as `GET /{$}` instead of `GET /` (`cmd/handlers.go`), so unknown public paths become 404.
- That's a behaviour change List Pocket had to take just to start.

## 6. Keep terminal proposals reachable after reopening a conversation

**Today.**
- `GET /conversations/{id}` returns only pending, approved and executing proposals (`chat/http.go:907`).
- Once a proposal is applied or rejected, reopening the conversation shows only the `proposal_outcome` notice line.
- The card is gone, along with its result. List Pocket's result carries `source.href`, which is the "Open" link to the created or changed record.

**Suggested.** Do either of these:
- Return the conversation's terminal proposals too, perhaps behind a query flag or capped to the most recent N.
- Put a host-safe link in the `proposal_outcome` notice. For example, let `ToolSpec` return a `Link` (`{href, label}`) from the result, stored on the notice and rendered by the binding.

**List Pocket today.** After reopening a conversation, the Open link isn't reachable; this is a known gap. We'd otherwise have to call `GET /proposals/{id}` for each notice.

## 7. Report "may or may not have applied" from a custom tool

**Today.**
- In `custom.go:433-445`, any handler error that isn't a `ToolError` or `AccessError` becomes "Tool execution failed.".
- `chat_bridge.go` (~762) then records it as `failed`.
- Only a timeout or cancellation gives `chat.ErrOutcomeUnknown`.
- A handler that wrote and then errored (for example, a reply sent before a ledger update failed) is shown to the reviewer and the model as a definite failure, which invites a retry.

**Suggested.**
- Export an error constructor, for example `pbmcp.OutcomeUnknown(msg)`, that a custom tool can return. The chat executor would record it as `unknown`, with the message.
- Consider keeping the handler's safe message on `failed` instead of the generic text, through a `ToolError`-style wrapper the host opts into.

**List Pocket today.**
- `mcpToolCallError` returns a plain error for unexpected failures on writes, so these are recorded as `failed` with generic text.
- This is documented as a follow-up in the migration plan.

## 8. Let the binding's proposal renderer see the whole proposal

**Today.**
- The binding's proposal entry (`AgentProposalEntry`) carries only ids and display strings.
- A host that draws its own card with `registerEntries: false` has to look the proposal up in `controller.state.proposals` itself to get `arguments`, `warnings` and `result`.

**Suggested.** Pass the full `ChatProposalEntry` to the renderer: as `entry.proposal`, or as a second argument to a `renderProposal(entry, proposal)` option.

**List Pocket today.** The custom renderer in `AgentChat.vue` looks up `chat.state.proposals` by `proposalId`.

## 9. Show stored page context as reference chips

**Today.**
- With `StoreContext`, each user message has a `context` part, and `entriesFromState` ignores it.
- A host that shows what was attached has to post-process `entries`.

**Suggested.**
- Add a `bindAgentPanel` option such as `referencesFromContext(context) => AgentReference[]`. The binding would merge the result into the user text entry's `references`, alongside the attachment chips.
- Also an "in context" helper that returns the last N distinct references across the conversation.

**List Pocket today.**
- `lp-agent-page.js` overrides the `entries` setter to run `withContextReferences()` on every binding write.
- `contextReferencesFromTranscript()` builds the "In context" list.

## 10. Let hosts map error text

**Today.**
- `errorText()` in the binding maps failure codes to fixed English text, and there's no option to replace it.

**Suggested.** Add an `errorText(error, fallbackText)` option to `bindAgentPanel`, for translated or product-specific wording.

**List Pocket today.**
- The `error` setter on `lp-agent-page.js` compares the binding's exact strings for `provider_auth` and `usage_limit` and swaps in List Pocket's wording (`hostErrorText` in chat-host.mjs).
- That breaks silently if the module rewords those strings.

## 11. Separate the chat tool timeout from the MCP one

**Today.**
- `Config.MaxToolTimeout` bounds MCP tool calls and chat tool calls alike (`custom.go:405`).
- `ChatOptions.DecisionTimeout` bounds the approval request, and an approved write runs inside it.
- If `DecisionTimeout` is larger than `MaxToolTimeout`, the write is cut off early and recorded as `unknown`, even though it usually finishes.

**Suggested.**
- Either add a chat-specific tool timeout, or make `EnableChat` reject (or warn about) a `DecisionTimeout` larger than `MaxToolTimeout`.
- Document how the two relate in `docs/chat.md`.

**List Pocket today.** One constant (`assistantToolTimeout`, 5 minutes) sets both.

## 12. Publish the browser client as an installable package

**Today.**
- `frontend/client` (`@pocketbase-mcp/chat-client`) isn't on npm, and npm can't install a subdirectory of a git repo.

**Suggested.** Do one of these:
- Publish it to npm.
- Mirror it to its own repo with tags, like agent-panel, so hosts can `npm install github:compdani/pocketbase-mcp-chat-client#vX.Y.Z`.

**List Pocket today.**
- Installed as `@pocketbase-mcp/chat-client` from the mirror's v0.1.8 tag (a codeload tarball).
- `frontend/tests/chatClientVersion.test.mjs` keeps that package's version in step with `go.mod`.

## agent-panel (separate repo)

**Document breaking changes in COMPATIBILITY.md for every release.**
- The v0.5.0 package doesn't ship `COMPATIBILITY.md`, and the local copy stops at 0.2.2.
- The rich-text composer (0.3.0) and the "+" menu replacing the paperclip (0.3.0) were only in `AGENTS.md`.
