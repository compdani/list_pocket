---
name: orchestrate
description: Plan-then-delegate workflow. A main model analyzes a directive, splits it into independent work units, and dispatches coding sub-agents (same or different providers/models) to implement them, then reviews and integrates the results. Use when asked to "orchestrate", "delegate coding", or for multi-part features.
---

# Orchestrate: planner + coder sub-agents

You are the **orchestrator**. You do NOT write the bulk of the code. You interpret the directive, plan, delegate, verify, and integrate.

## 1. Process the directive
- Restate the goal and acceptance criteria in 2–4 lines.
- Read only enough code to plan (use `search_code_subagent` / grep). Follow repo conventions (Go handlers in `cmd/`, logic in `internal/core/`, Vue in `frontend/src/`, record-id-first APIs, numbered migrations in `internal/migrations/`).
- Ask the user only if a missing decision materially changes the result.

## 2. Split into work units
Produce a table: `id | scope (files/dirs) | description | depends_on | model`.
- Units must own **disjoint files**; if two touch the same file, merge them or serialize via `depends_on`.
- Each unit has a clear done-check (build/test command).
- Typical split: migration+models → core → handlers → frontend → tests/docs.
- Track in the session `todos`/`todo_deps` tables.

## 3. Choose workers
Call `orchestrator_capabilities` once to see available providers/models. Default routing (override if the user names a model):
- Mechanical edits, boilerplate, tests → fast/cheap model.
- Complex logic, concurrency, hot-path SQL, migrations → strongest coding model.
- Mixing providers is encouraged (e.g. one implements, another reviews).
If a requested provider/model is unavailable, tell the user; never silently substitute.

## 4. Dispatch
- Same provider, no special settings: native `task` tool (`general-purpose`).
- Other provider/model or isolated checkout: `delegate_task` with `target:{providerInstanceId, model}`; `isolation:"worktree"` for units that could conflict, else `shared`.
- Launch all units with no unmet dependencies **in parallel**; don't poll — wait for completion notifications.

### Worker brief template (workers get no parent history)
```
Context: <repo + relevant conventions, 3-5 lines>
Goal: <unit description>
Files you own: <paths>  (do not edit others)
Interfaces to honor: <signatures/JSON shapes from other units>
Done when: <commands, e.g. go build ./... && go test ./internal/core -count=1>
Constraints: minimal surgical changes, no unrelated refactors, no secrets, no commits.
Return: summary of changes, files touched, test output, open concerns.
```

## 5. Review and integrate
- Read each result and inspect `git --no-pager diff`.
- Run the smallest build/test covering the change, then relevant suites (`go test ./cmd ./internal/core -count=1`, `cd frontend && npm run test:unit`).
- Fix small problems yourself. For substantive ones, send a **new** delegate with the findings; max 2 retry rounds per unit, then report.
- Optional: a different-provider reviewer (`role:"review"`) on the final diff for risky changes.

## 6. Report
Short summary: what was done, worker/model per unit, test results, unresolved items. Don't commit unless asked.

## Guardrails
- Never let workers edit the same file concurrently.
- Don't delegate work you can finish in a few direct edits.
- Don't send sensitive data (`.env`, `config.toml`, `pb_data`) to workers.
