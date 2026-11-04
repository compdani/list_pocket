# Architecture council

The primary council uses Cursor Grok 4.7 and GPT Sol 6.1. Copilot Sonnet 5.5
provided two limited checks of lifecycle and authorization; those are not full
architecture assessments. The user subsequently requested a full Sonnet review
for round 5. That full review scored 8.3/10 and identified further fixes. Scores reflect each review snapshot, before fixes made
in response to that round. No score is inferred from a subsequent fix.

| Reviewer | Round 1 | Round 2 | Round 3 | Round 4 | Round 5 | Round 6 | Round 7 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Cursor Grok 4.7 | 7/10 | 8/10 | 8.5/10 | 8.5/10 | 9/10 | 8/10 | 9/10 |
| GPT Sol 6.1 | 7/10 | 7.5/10 | 8/10 | 8.5/10 | 9/10 | 8.5/10 | 9/10 |
| Copilot Sonnet 5.5 | 6.5/10 (limited) | 7.5/10 (limited) | not requested | not requested | 8.3/10 (full review) | 8.5/10 | 9/10 |

Current status: all three round 7 reviewers independently approved at 9/10.
The requested council target is met within the documented scope. Notes below the table are historical snapshots; their
references to pending reviews describe the time of each entry.

Implemented ownership and failure contracts are described in architecture.md.
Round 3's remaining blockers were agent re-execution under new delivery keys,
terminal delivery finalization, child linkage/cancellation wake-up, and partial
bulk chunk counts. Fixes and focused fault tests are present for each; fresh
independent review is still required to establish the requested 9/10.

Validation includes the full Go suite, frontend build and 114 unit tests,
targeted race checks, and database-backed failure tests for transaction rollback,
parent wake-up, job finalization, outbox replay/restart and ambiguous outcomes,
and bulk partial progress.

Changes do not claim exactly-once external delivery. Interrupted agents with a persisted side-effect intent marker fail
closed pending reconciliation; unmarked read-only executions can retry. Other conversational/plugin side effects and
legacy SQL event-policy migration remain explicitly bounded follow-up work.

Round 4 Sol found linked-child replay and concurrent cancellation overwrite.
Both have focused crash/transaction-boundary regression tests and fixes;
new scores are required after these changes. Grok round 4 confirmed the same linked-child replay blocker. Round 5 reviews
the updated attached/detached replay guard and transactional terminal-state checks.

Round 5 Sol approved at 9/10 with no remaining blockers in the agreed scope.
Independent validation passed full Go tests, targeted transition race tests,
flow/delivery/queue tests and all 114 frontend unit tests. Grok also approved round 5 at 9/10 with no new blocker. The requested full
Copilot Sonnet round 5 review remains pending.

Round 5 full Sonnet followup: fixed ambiguous transactional status schema and
atomic projection from delivery receipts (including finalization), typed safe
non-attempt retries, plain-text inline conversation receipts without adding
template receipts, manager-worker delivery with shared rate admission and no
provider I/O under admission locks, and durable pre-call side-effect markers so
read-only agents can retry while uncertain writes remain fail-closed. Focused
fault and live-delivery shutdown tests cover these contracts. Round 6 rechecks
the changed code; prior approvals are not inferred to cover these changes.

Round 6 Sol identified saturated admission blocking stop and manager signaling
after workflow join. Stop signaling now has independent ownership and occurs
at the start of application shutdown. Saturation and application-level active/
queued delivery tests pass, including race validation. Fresh review remains
required; the other round 6 reviews are pending.

Round 6 Sonnet found a shared-cadence concurrency throughput regression. Generic
limits now preserve concurrency × message_rate while channel/SMS overrides stay
global and sliding windows retain bursts. Cadence and pre-call durable agent
marker tests were added. Grok round 6 is still pending.

Round 6 Grok confirmed the cadence regression and found fallback web search
misclassified as a write. Admission now reserves per-channel times under a short
mutex and sleeps outside it; timestamp reservation tests cover 100/s generic
throughput, independent channels, and burst-then-hold windows. Fallback search
implements ReadOnly, with a real runner/subrequest test checking its intent flag.
All round 6 blockers are addressed; round 7 rechecks the final code.

Round 7 Sol independently approved at 9/10 after source inspection, finding no
remaining blocker. Tests were inspected but not rerun in that review.

Round 7 Copilot Sonnet approved at 9/10 with no concrete blocker, independently
passing go build, uncached full Go tests, and manager/delivery/flow/cmd race
tests. Frontend tests were not rerun in this review.

Round 7 Cursor Grok approved at 9/10 with no blocker after source inspection.
It did not rerun tests. All three reviewers now independently approve the same
implementation at 9/10. Changes remain uncommitted; no PR was created.
