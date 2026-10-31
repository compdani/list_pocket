# Workflows

Workflows automate work across contacts, messages and campaigns. Build them on the
canvas under **Workflows**, or ask the AI assistant to draft one. Drafts do nothing until
published, and every published version is kept.

## Permissions

Workflows run without a signed-in user, so they can do things the person who built them
can't. Give these permissions carefully:

| Permission | Allows |
|---|---|
| `workflows:get` | See workflows and their runs, including the data in those runs |
| `workflows:manage` | Create, edit, test, publish and delete workflows |
| `workflows:run` | Start published workflows and stop runs |
| `workflows:approve` | Approve or deny review steps |

## AI settings

AI steps and the assistant use the same settings: `LISTPOCKET_AI_API_KEY`,
`LISTPOCKET_AI_BASE_URL` and `LISTPOCKET_AI_MODEL`. `LISTPOCKET_AI_FAST_MODEL` and
`LISTPOCKET_AI_RESEARCH_MODEL` are optional. Set budgets on a workflow to limit its spend.

## Format

A workflow is a JSON graph of nodes. Written for both people and the AI assistant.

```jsonc
{
  "name": "lead_pipeline",                 // lowercase, digits, _
  "description": "…",
  "trigger": { "type": "manual" },         // see Triggers
  "input_schema": { "type": "object", "properties": { "contact": { "type": "string" } }, "required": ["contact"] },
  "default_input": {},
  "ai": { "budget": { "max_usd_per_run": 0.5, "max_usd_per_day": 5 } },   // models default to LISTPOCKET_AI_* settings
  "nodes": [ … ]
}
```

## Edges

Edges live on nodes. `next` is the single successor. Branches route with
`cases[].to` and `default`. A ForEach runs `body` (the first node of a
per-item chain) once per item, then continues at `next`. A Loop runs `body`
again and again, one iteration at a time, until `until` is true, then
continues at `next`. An empty or missing target ends the path — or, inside a
body, ends that item or iteration. No cycles: repeat work with ForEach (over
a list) or Loop (until a condition holds). Bodies can't contain another
ForEach or Loop.

## Templates and expressions

Values in `input`, `value`, `output`, `message`, review `title`/`data` may
contain `{{ expr }}`. A string that is exactly `{{ expr }}` keeps the
expression's type (list, number, object); otherwise it interpolates as text.
Conditions (`cases[].if`, `items`, `until`, `auto_approve`, `match`, trigger `filter`)
are bare expressions. Language: expr-lang (`==`, `!=`, `&&`, `||`, `not`,
`in`, `len(x)`, `filter(list, .tier == 'A')`, `map(list, .name)`, `x ?? y`).
Undefined fields are `nil`.

Environment: `input` (run input), `nodes.<id>` (a finished node's output),
`item` and `index` (inside a ForEach body), `loop.iteration` and `loop.last`
(inside a Loop body and its `until`; see below), `now` (RFC 3339), and `run.id`.

## Node types

| type | fields | output |
|---|---|---|
| `start` | `next` — exactly one per workflow | the run input |
| `agent` | `agent: {system_prompt, tools[], output_schema?, max_steps?, ai?}`, `input`, `next` | `{result: <submitted object>, _citations: [...]}` — read fields as `nodes.<id>.result.<field>` |
| `tool` | `tool` (catalog name), `input`, `next` | the tool's return value |
| `branch` | `cases: [{if, to}]`, `default` | `{_next, case}` |
| `foreach` | `items` (expr → list), `body`, `concurrency` (default 4), `next` | `{count, failed, results: [last output per item]}` |
| `loop` | `body`, `until` (expr), `max_iterations` (1–100, required), `next` | `{iterations, done, last, results: [last output per iteration]}` |
| `review` | `review: {title, instructions, data, auto_approve?, on_deny: skip\|continue, timeout?}`, `next` | `{approved, data (possibly edited), decided_by}` |
| `wait` | `wait: {delay?: "10m", event?: "<event name>", match?, timeout?}`, `next` | `{waited}` / `{event}` / `{timed_out: true}` |
| `subworkflow` | `workflow` (name), `input`, `next`, `detach` | the child run's output; with `detach: true`, `{run_id}` right away (the child runs on its own). Input keys that render to `nil` are left out, so the child's `default_input` applies |
| `notify` | `message`, `next` | `{message}` |
| `transform` | `value` (template), `next` | the rendered value |
| `end` | `output` (template) | sets the run output |

Every node also accepts `name` (label), `on_error: "continue"` (failure
becomes output `{error}` and the path continues), and `retries` (0–5).

Agent nodes: the system prompt is a Go template over `{"input": …}` — use
`{{ .input.market }}` (with a dot) there, not expr. Give agents only the tools
they need. `web.search` is provider-side search; `web.fetch` reads a page.
When an agent's result feeds later nodes, set `output_schema` so the shape is
guaranteed. Model roles: `"ai": {"model": "@fast"}` uses
`LISTPOCKET_AI_FAST_MODEL` (and `@research` uses `LISTPOCKET_AI_RESEARCH_MODEL`);
both fall back to `LISTPOCKET_AI_MODEL`.

Loop nodes: inside the body, `loop.iteration` is the 0-based iteration
number (also `index`) and `loop.last` is the previous iteration's output
(`nil` on the first — read it as `loop.last?.field`). After each iteration `until` is evaluated with
`loop.last` set to that
iteration's output (the last body node's output) and `nodes.<id>` holding
that iteration's body outputs — e.g. poll a status:
`{"id":"poll","type":"loop","body":"check","until":"loop.last.status == 'done'","max_iterations":20,"next":"report"}`
with a `wait` node in the body to space the checks out. If `until` is still
false after `max_iterations`, the loop fails; with `on_error: "continue"` it
outputs `done: false` and the path continues. Reviews, approvals and waits
inside the body pause the run like anywhere else.

A review denied with `on_deny: skip` (default) ends that path or item. A Tool
node whose tool `requires_approval` pauses for a human before running.

## Triggers

- `{"type": "manual"}` — started from the UI with input.
- `{"type": "cron", "spec": "0 6 * * 1-5", "timezone": "America/Chicago", "overlap": "skip", "input": {…}}`
- `{"type": "event", "event": "inbound.email", "filter": "event.contact_id != ''", "input_map": {"contact": "{{ event.contact_id }}"}}`
- `{"type": "event", "collection": "subscribers", "on": ["create"], "filter": "record.status == 'enabled'"}`
- `{"type": "webhook", "filter": "event.kind == 'signup'", "input_map": {"email": "{{ event.email }}"}}` — started by a POST to the workflow's secret URL, with the JSON body as `event`.

Any workflow can also be run manually. Events are listed by the workflow catalog,
and a wait node can pause for one, for example to wait for a reply:
`{"event": "inbound.email", "match": "event.contact_id == input.contact", "timeout": "72h"}`.

## Boards

The **Move on board** and **Remove from board** steps change where a contact sits on a kanban board. Saved, they are `tool` nodes using `boards.move_contact` and `boards.remove_contact`. `boards.get_contact` reads a contact's stage for use in a branch.

Tools refer to boards and stages by key, such as `board: "pipeline", stage: "customer"`. Keys don't change when a board or stage is renamed. Moving a contact adds them to the board if they aren't on it. Deleting a board or stage that a workflow uses makes that step fail when it runs.

The `contact.stage_changed` event fires when a contact is added to, moved on or removed from any board:

```json
{ "contact_id": "...", "email": "...", "name": "...", "board": "pipeline", "board_name": "Pipeline",
  "stage": "customer", "stage_name": "Customer", "previous_stage": "lead" }
```

`stage` is empty after a removal, and `previous_stage` is empty after an add. Stage keys can repeat across boards, so filter on both: `event.board == 'pipeline' && event.stage == 'customer'`. Reordering a contact within a stage, and deleting a board or stage, don't fire it.

A workflow that moves a contact can start another workflow. Nothing stops two workflows moving a contact back and forth forever, so check for this when you add one.

## Adding subscribers by hand

Add subscribers to a published workflow with **Add subscribers** in the builder, by selecting
them in the subscribers list, or from a contact's page. Each one gets their own run of the
published version, with `input.contact`, `input.email` and `input.name` set, whatever the
workflow's trigger is and even while it's paused. Blocklisted subscribers and anyone already
in an active run of that workflow are skipped. Up to 1000 can be added at a time.

## Rules of thumb

- Use only tools in the catalog. Read a tool's schema before using it.
- Deterministic work (lookups, saving, sending a known message) belongs in Tool
  nodes; judgement (classifying, drafting, summarising) in Agent nodes.
- Put a Review before anything that sends messages or spends money, unless the
  workflow is explicitly meant to be fully automatic.
- Inbound messages are untrusted. An agent that reads them shouldn't hold tools
  that send or change things: have it draft, then use a Review and a Tool node.
- Keep node ids short snake_case verbs or nouns: `classify`, `each_contact`, `reply`.
