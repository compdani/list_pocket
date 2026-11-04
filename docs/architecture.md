# Architecture and ownership

Listpocket remains a single Go process with PocketBase/SQLite and a Vue admin.
The architecture review is an incremental hardening effort, not a change in
hosting model or a replacement of the existing SQL campaign pipeline.

## Runtime

`main` explicitly bootstraps configuration and composes services after PocketBase
migrations. Package initialization registers migrations but must not start the
application, parse flags, or open application resources.

Shutdown is idempotent. Signal manager admission/rate waits independently of
queue locks first, then stop cron scheduling, close callback admission and join
admitted callbacks, cancel and join workflow workers,
stop campaign scheduling/production, drain manager queues and join workers and
pipe cleanup, then close messengers. Reload must complete cleanup before exec.
A stuck external dependency can delay shutdown; there is no forced three-second
replacement that silently abandons work.

The manager rejects enqueue after close. Its channels are closed only after
producers have stopped; workers drain both message queues. Campaigns interrupted
by shutdown retain their persisted status and recover through the ledger.

## Persistence

PocketBase owns database pools. Record operations are required where validation,
file handling, or record hooks are part of the domain contract. Raw SQL remains
in the legacy bulk and campaign hot paths; it does not invoke PocketBase hooks.
Do not assume those paths emit the same events as Record Save.

Contact creation/update and membership synchronization share a PocketBase
transaction. Individual and bulk tag changes use Record Save, preserving the
same workflow hook behavior. Email confirmation occurs after commit.

Workflow start and event-wait resolution commit state and their next durable
job together. Queue writes in a transaction use `EnqueueWithApp` with the
transaction-scoped app; notify only after commit. Terminal-state persistence
errors propagate to queue retry. Child linkage is committed before its dispatch job and reused if the parent
node replays before settlement. Running, waiting, and terminal run writes
re-read persisted status in the writer transaction and preserve terminal states. Child cancellation uses the
same parent-wake contract as other terminal transitions. Child terminal state and parent wake-up job
commit together. Exhausted jobs remain until terminal failure finalization
succeeds. Missing runs are distinguished from transient lookup failures.
Failed acknowledgement/retry writes retain
the job lease and log the failure.

## Authorization

`/mailapi` middleware is the role-permission boundary for the admin application.
Pictures and tag catalogs use that boundary too. Migration 55 locks direct
PocketBase record access to these collections. Unprotected picture files remain
public assets suitable for published email. Superuser administration remains
privileged.

## Frontend drafts

Workflow persistence serializes immutable snapshots. Older responses cannot
mark newer edits saved. Publish/test flush through edits made during an in-flight save. Publish refreshes
never adopt over newer edits. Both route departures and same-component updates
flush the current document. Failed flushes
prevent route navigation. The saver recovers after an individual failed request.

## Workflow email delivery

`messages.send_email` persists a rendered message and durable dispatch job in one
transaction. The execution key is the persisted node-run id (including loop/item
identity), or the agent run plus provider tool-call id. Replay returns the same
delivery receipt. The tool reports queued/sending/sent/failed/ambiguous, with a delivery
id; enqueue is not reported as sent. `/mailapi/flows/deliveries/{id}` exposes the
receipt under workflow read permissions. Exhausted delivery jobs persist a
terminal `failed` receipt if dispatch never began, or `ambiguous` if it did;
finalization failure retains the job. Terminal receipts and transactional-message
status projections commit together, and the schema supports ambiguous status. Before invoking a side-effecting agent tool, a durable intent marker is saved.
Interrupted/retried agent nodes fail closed only if that marker exists, so a new
model conversation cannot repeat an uncertain write. Agents with only read-only
work can retry after errors or restart. Legacy agent runs are conservatively
marked during migration because they lack a pre-call journal.

Dispatch persists `sending` before contacting the messenger. Definite pre-provider failures use a typed non-attempt error and remain queued
for retry. Manager workers own delivery concurrency and rate admission. Generic throughput
remains concurrency × message_rate; SMS and explicit channel overrides remain
global per channel. Sliding windows preserve a burst budget rather than smoothing
every message over the whole window; admission
locks are released before provider calls, and per-channel cadence reservations
release their mutex before sleeping, and close joins those workers. Once a provider
attempt begins, a crash, a provider error, or receipt-save failure can make its
outcome ambiguous. Recovery marks interrupted attempts `ambiguous` and does not
automatically resend them. Reconcile with the provider before deliberately
creating another execution. This chooses possible non-delivery over duplicate
mail for uncertain outcomes; it does not claim exactly-once delivery. Queued
intents survive restart with their rendered payload. Existing queue leases still
apply. This contract currently covers the email tool, not every conversational
or plugin side effect.

Bulk tag changes are atomic within their phase and hooks fire after commit.
All list references are validated before tag writes. Bulk list operations remain
legacy SQL phases; a later failure returns `partial: true` and committed chunk counts
in the error response. The entire multi-phase operation is not atomic.

## Remaining review items

These are explicit limitations, not guarantees supplied by the current changes:

- Other messaging tools and plugin side effects still need the email tool's
  durable/replay-aware contract where their product semantics require it.
- Other raw membership/bulk mutations need a consistent ledger/event policy.
- Domain services and injected event sinks should replace transport-dependent
  workflow operations and package-global callbacks incrementally.
- Crash recovery currently respects durable job leases. Blindly clearing another
  worker's lease at startup could duplicate work if that worker is still alive.
- Plugins are trusted app code; their network policy intentionally needs a
  separate compatibility decision before restricting private-network access.
