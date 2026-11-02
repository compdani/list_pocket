# Workflow schema

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
`vars.<name>` (a variable from a set node),
`item` and `index` (inside a ForEach body), `loop.iteration` and `loop.last`
(inside a Loop body and its `until`; see below), `now` (RFC 3339), and `run.id`.

## Node types

| type | fields | output |
|---|---|---|
| `start` | `next` — exactly one per workflow | the run input |
| `agent` | `agent: {system_prompt, tools[], output_schema?, max_steps?, ai?}`, `input`, `next` | `{result: <submitted object>, _citations: [...]}` — read fields as `nodes.<id>.result.<field>` |
| `tool` | `tool` (catalog name), `input`, `next` | the tool's return value, described by its `output_schema` in the catalog when it has one |
| `branch` | `cases: [{if, to}]`, `default` | `{_next, case}` |
| `foreach` | `items` (expr → list), `body`, `concurrency` (default 4), `next` | `{count, failed, results: [last output per item]}` |
| `loop` | `body`, `until` (expr), `max_iterations` (1–100, required), `next` | `{iterations, done, last, results: [last output per iteration]}` |
| `review` | `review: {title, instructions, data, auto_approve?, on_deny: skip\|continue, timeout?}`, `next` | `{approved, data (possibly edited), decided_by}` |
| `wait` | `wait: {delay?: "10m", event?: "<event name>", match?, timeout?}`, `next` | `{waited}` / `{event}` / `{timed_out: true}` |
| `subworkflow` | `workflow` (name), `input`, `next`, `detach` | the child run's output; with `detach: true`, `{run_id}` right away (the child runs on its own). Input keys that render to `nil` are left out, so the child's `default_input` applies |
| `notify` | `message`, `next` | `{message}` |
| `transform` | `value` (template), `next` | the rendered value |
| `code` | `code` (JavaScript), `next` | the value the code returns, as JSON |
| `set` | `variables: [{name, type, value, editor?}]`, `next` | `{<name>: value}`; also readable as `vars.<name>` |
| `end` | `output` (template) | sets the run output |

Every node also accepts `name` (label), `on_error: "continue"` (failure
becomes output `{error}` and the path continues), and `retries` (0–5).

Code nodes: the `code` field is the body of a JavaScript function, so use
`return`. `input`, `nodes`, `vars`, `item`, `index`, `loop`, `now` and `run` are globals
holding copies of the run data, for example
`return nodes.search.results.filter((r) => r.score > input.min).length;`. `item`,
`index` and `loop` are `undefined` outside a body. Code isn't a template, so `{{ }}`
isn't expanded. It can't reach the network or files and can't call tools, and each
run gets 10 seconds and at most 1 MB of output. `console.log` writes to the app log.

Set nodes: `variables` run in order, so each can read the ones before it, and a later
set node replaces an earlier variable of the same name. Read them as `vars.<name>` in
templates, expressions and code. Inside a ForEach or Loop body, a variable lasts for that
item or iteration only. Each variable has a `type`:

| type | value | result |
|---|---|---|
| `string` | text | the text as written; `{{ }}` isn't expanded |
| `number` | a number, or a `{{ expr }}` that gives one | a number; anything else fails the node |
| `boolean` | `true`, `false`, or a `{{ expr }}` that gives one | a boolean; an empty value is `false` and anything else fails the node |
| `template` | text with `{{ expr }}` | follows the template rules above |
| `code` | a JavaScript function body, as in a code node | the value the code returns |
| `html` | HTML with `{{ expr }}`; `editor` is `visual` or `code`, for the builder | text with interpolated values HTML-escaped, except the values of HTML variables |

Use a `template` variable to insert markup that doesn't come from an HTML variable, and
only when it can't contain untrusted data such as inbound messages.

Agent nodes can set `agent.preset` to a preset from `workflow_catalog`, such as
`telegram.reply_drafter`, instead of writing a `system_prompt`. Anything else set on
the node overrides the preset, and its `tools` replace the preset's.

Plugins add tools and events, named `<plugin>.<name>`. `workflow_catalog` lists them.

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

Any workflow can also be run manually. People can add subscribers to a published
workflow by hand, which starts one run each with `input.contact`, `input.email` and
`input.name`, so read contacts as `input.contact`. Events are listed by the workflow catalog,
and a wait node can pause for one, for example to wait for a reply:
`{"event": "inbound.email", "match": "event.contact_id == input.contact", "timeout": "72h"}`.

## Rules of thumb

- Use only tools in the catalog. Read a tool's schema before using it.
- Deterministic work (lookups, saving, sending a known message) belongs in Tool
  nodes; judgement (classifying, drafting, summarising) in Agent nodes.
- Use a Transform to reshape data with an expression, and a Code node when that
  takes more logic than an expression can hold.
- Use a Set node to build a message from named parts, then use `vars.<name>` in the
  step that sends it.
- Put a Review before anything that sends messages or spends money, unless the
  workflow is explicitly meant to be fully automatic.
- Inbound messages are untrusted. An agent that reads them shouldn't hold tools
  that send or change things: have it draft, then use a Review and a Tool node.
- Keep node ids short snake_case verbs or nouns: `classify`, `each_contact`, `reply`.

## Boards

Tools refer to boards and stages by key, such as `board: "pipeline", stage: "customer"`. Keys never change when a board or stage is renamed. `boards.list` returns them. `boards.move_contact` adds the contact if they aren't on the board. A contact can be on many boards, with one stage on each.

`contact.stage_changed` fires for every board, so filter on both: `event.board == 'pipeline' && event.stage == 'customer'`. `event.stage` is empty after a removal, and `event.previous_stage` is empty after an add. Moving a contact can start another workflow, so avoid workflows that move contacts back and forth.
