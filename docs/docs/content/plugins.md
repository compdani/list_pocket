# Plugins

A plugin adds to a running List Pocket without a rebuild. It can:

- send new types of message, including new channels with their own addresses and consent
- receive webhooks at its own URLs
- add pages, settings tabs and other UI to the admin
- add tools, events, agent presets and field editors to workflows

Only super admins can install plugins. **A plugin runs with full access to the app and its data**, so only install plugins you trust. Its browser code can act as any logged-in admin who loads it.

`examples/plugins/telegram` is a complete working plugin that uses all of this.

## Add-on types

A plugin declares what it adds in its manifest, and can add any mix of these. The Plugins page lists what each one adds.

| Add-on | Manifest key | Code | What it adds |
|---|---|---|---|
| Channel | `channels` | `plugin.channel` | A new way to send campaigns and messages |
| Webhook | `webhooks` | `plugin.webhook` | A public URL that receives requests |
| Admin route | `adminRoutes` | `plugin.adminRoute` | An API for the plugin's own UI |
| Settings | `settings` | None | A generated tab under **Settings** |
| Admin UI | `ui` | `host.routes`, `host.nav`, `host.slots` | Pages, navigation and page sections |
| [Workflow tool](#workflow-tools) | `tools` | `plugin.tool` | An action for Tool nodes and agents |
| [Workflow event](#workflow-events) | `events` | `host.workflows.emit` | Something a workflow can start on or wait for |
| [Agent preset](#agent-presets) | `agentPresets` | None | A ready-made agent for Agent nodes |
| [Field editor](#field-editors) | `fieldEditors` | `host.fields.add` | A custom input for a tool's field |

Plugins add to workflows through tools rather than new node types. A tool already works with approvals, agents, retries and run history.

## Building a plugin with the assistant

Super admins can ask the AI assistant to write or change a plugin, from the assistant panel or the **Agent** page. **Build with AI** on the **Plugins** page opens it. Other roles don't have these tools.

The assistant can read this guide and an installed plugin's source. It works in a draft, which it can write a few files at a time and change with targeted edits. A draft of an installed plugin starts as a copy of it. Drafts are saved on the server, so they outlast a conversation turn and a restart. They run nothing and aren't installed, so writing to one needs no approval. An untouched draft is removed after 30 days. Stored settings and secrets aren't available to the assistant.

When the draft is finished, the assistant proposes it:

1. Its manifest and script syntax are checked, and the assistant fixes any errors it gets back. None of the plugin's code runs at this point, so a plugin that passes may still fail to load.
2. Open **Review** to read the code. You can edit it there. For a new version, you can also see what changed from the installed one.
3. **Install** adds it, disabled, and removes the draft. A new version of an installed plugin keeps its images and other files that aren't text.
4. Enable it under **Settings → Plugins**. The app restarts, and any load error appears on that page. Ask the assistant to read it and fix the problem.

Installing a new version of an enabled plugin needs no further approval to go live: it takes over at the next restart, even one you didn't start for this reason. Disable the plugin first if you aren't ready for that.

Only plain text files can be written this way: `.js`, `.mjs`, `.json`, `.css`, `.md`, `.txt`, `.svg` and `.html`, up to 2 MiB of source in all. Upload a zip for anything else. A draft of a plugin that's already been replaced by an upload can't be proposed, because it would undo that upload. The assistant has to start it again.

Inbound messages and other data can try to steer an AI model. Be wary of any plugin you didn't ask for, whatever it says it's for.

## Installing

Go to **Settings → Plugins → Install plugin** and upload a zip. A new plugin stays disabled until you enable it.

| Action | Takes effect |
|---|---|
| Install, change settings | Straight away |
| Enable, disable, upgrade, uninstall | After an automatic restart, which waits for running campaigns |

A plugin can't be disabled while a running or scheduled campaign uses one of its channels.

You can also copy a plugin to `<pb_data>/plugins/<id>/<version>/`. It's found at the next start and stays disabled until you enable it.

### If a plugin stops the app from starting

Set `LISTPOCKET_plugins__safe_mode=true` to start with no plugins loaded, then disable or uninstall the plugin. A plugin that only fails to load doesn't need this: it's marked as failed and the app carries on. If a campaign's channel is unavailable, the campaign pauses rather than being cancelled.

To test the admin UI without any plugin's browser code, add `?plugins=off` to the URL.

## Package layout

```
plugin.json     the manifest
server.js       server-side code
ui/index.js     optional browser module; only ui/ is served publicly
i18n/en.json    optional messages
```

## Manifest

```json
{
  "schemaVersion": 1,
  "id": "telegram",
  "name": "Telegram",
  "version": "1.0.0",
  "server": { "entry": "server.js", "concurrency": 4, "timeoutMs": 15000 },
  "ui": { "entry": "ui/index.js", "css": ["ui/index.css"] },
  "channels": [{
    "id": "ch:telegram",
    "label": "Telegram",
    "icon": "<SVG path data>",
    "addressSource": "channel",
    "addressLabel": "Chat ID",
    "contentTypes": ["plain"],
    "campaignFields": [{ "key": "disable_preview", "type": "boolean", "label": "Hide link previews" }],
    "rateLimit": { "maxPerSecond": 25 }
  }],
  "webhooks": [{ "path": "inbound", "methods": ["POST"], "maxBodyBytes": 1048576 }],
  "adminRoutes": [{ "path": "contacts", "methods": ["GET"], "permission": "plugin_telegram:manage" }],
  "settings": [{ "key": "bot_token", "type": "secret", "label": "Bot token", "required": true }],
  "permissions": [{ "group": "plugin_telegram", "permissions": ["plugin_telegram:manage"] }],
  "i18n": { "en": "i18n/en.json" }
}
```

- **`id`:** lowercase letters, numbers and underscores, starting with a letter. `whatsapp` is reserved for the built-in channel.
- **Channel IDs:** `ch:<plugin id>` or `ch:<plugin id>_<name>`.
- **`addressSource`:** `channel` keeps addresses in their own table. `phone` uses the subscriber's phone number.
- **`contentTypes`:** only `plain` is supported.
- **`campaignBody`:** set to `none` to hide the campaign's content step, for channels that send something else, such as a provider-side template. A UI slot can set the channel's options on `campaign.channelFields`.
- **`settings` and `campaignFields` types:** `text`, `textarea`, `secret`, `select` (with `options`), `number` or `boolean`. Settings get a generated tab under **Settings**, so a plugin needs no UI code for configuration.
- **Permission groups:** must be `plugin_<id>`. To label a group on the role form, add `globals.terms.plugin_<id>` to your messages.

## Server code

`server.js` is a classic script, with no `import`. It runs on a small pool of JavaScript VMs that share nothing, so keep state in `host.kv`, not in globals. Host calls are synchronous and there's no `setTimeout`. A call that runs past `timeoutMs` is stopped.

```js
plugin.channel('ch:telegram', {
  send(msg) {
    // msg.to, msg.body, msg.subscriber, msg.campaign
    // msg.fields: your campaignFields, or the data of a transactional message
    return { providerMsgId: '123', invalidAddresses: [] };
  },
});

plugin.webhook('inbound', {
  verify(req) { return req.verifyHmacSha256(host.settings.get().secret, req.headers['x-signature'], { prefix: 'sha256=' }); },
  handle(req) { return { status: 200, json: { ok: true } }; },
});

plugin.adminRoute('contacts', (req) => ({ json: { contacts: [] } }));
```

### Sending

Throw `host.errors.unsendable('reason')` if the recipient can never receive this, or return their addresses in `invalidAddresses`. Those addresses are retired and the recipient isn't retried. Any other error counts as a retryable send failure, and too many pause the campaign.

### Webhooks

Webhooks are public, at `/webhooks/plugins/<id>/<path>`. `verify` is required unless the manifest says `"verify": "none"`, and it must return exactly `true`.

| Response | Meaning |
|---|---|
| 404 | Unknown or disabled plugin, or unknown path |
| 405, 413 | Method not declared, or body over `maxBodyBytes` |
| 401 | `verify` didn't return `true` |
| 503, 504 | All VMs busy, or the handler timed out |
| 500 | The handler threw. Details go to the log only |

`req` has `method`, `path`, `query`, `headers` (lowercase), `body`, `bodyBase64`, `json()`, `verifyHmacSha256(secret, signature, { prefix, encoding })` and `once(key, ttlSeconds)`. A handler that returns nothing sends 200 `{"ok":true}`; otherwise return `{ status, headers, json }` or `{ status, headers, body }`.

Always use `verifyHmacSha256` for body signatures. It checks the raw bytes in constant time, and a string in JavaScript can't round-trip them. Providers retry, so use `req.once` to handle each delivery once.

### Host API

| Call | Purpose |
|---|---|
| `host.settings.get()` | Stored settings, including secrets |
| `host.kv.get(key)`, `set(key, value, { ttlSeconds })`, `delete(key)` | Plugin storage |
| `host.fetch(url, { method, headers, body, timeoutMs })` | HTTP. Returns `status`, `ok`, `headers`, `body`, `bodyBase64`, `json()` |
| `host.log.info(...)`, `warn`, `error` | Logging. Secret setting values are redacted |
| `host.crypto.hmacSha256Hex`, `sha256Hex`, `timingSafeEqual` | Crypto helpers |
| `host.contacts.findByAddress(channel, address)`, `get(id)` | Look up a contact, or `null` |
| `host.contacts.upsertByAddress(channel, address, { name, lists, consent, source })` | Find or create a contact. `lists` takes list UUIDs, and `source` records where consent came from |
| `host.contacts.setConsent(id, listId, channel, status, { source })` | `confirmed` or `unsubscribed` |
| `host.contacts.importSMSConsent(channel, { dryRun, includeInherited })` | Grant consent from SMS consent. Returns `explicit`, `inherited` and `applied` counts. Existing consent rows are never overwritten |
| `host.events.record({ channel, direction, kind, status, providerMsgId, subscriberId, address, body, payload })` | Add to the subscriber's timeline. Returns `false` for a repeated `providerMsgId` |
| `host.events.status(channel, providerMsgId, status, { permanent })` | Delivery update for a sent message. `permanent` also records a hard bounce and retires the address |
| `host.events.list({ subscriberId, channel, limit })` | A subscriber's latest events, newest first |
| `host.events.counts(campaignId)` | A campaign's sends by delivery status |
| `host.events.bounce({ channel, subscriberId, campaignId, type, meta })` | Record a bounce. Email bounce actions don't run for it |
| `host.events.optOut(channel, address)` | Withdraw consent on that channel, for everyone who owns the address |
| `host.messages.send({ channel, subscriberId, subject, body })` | Send one message, if the contact has consented |
| `host.workflows.emit(event, payload)` | Raise one of this plugin's [workflow events](#workflow-events) |

A plugin can only use its own channels.

## Workflow add-ons

Workflow add-ons are named `<plugin id>.<name>` everywhere they're used. Names use lowercase letters, numbers and underscores. If a workflow's plugin is disabled, publishing it fails validation and any run that reaches the missing tool or preset fails with an error saying so.

### Workflow tools

A tool appears in the builder's Tool node and in an Agent node's tool list. The assistant can also find it when it builds workflows.

```json
"tools": [{
  "name": "get_chat",
  "description": "Look up a Telegram chat by ID.",
  "readOnly": true,
  "inputSchema": {
    "type": "object",
    "properties": { "chat_id": { "type": "string", "description": "Telegram chat ID" } },
    "required": ["chat_id"]
  }
}]
```

```js
plugin.tool('get_chat', (input, ctx) => {
  // ctx.config is this workflow's tool_config for the tool. ctx.runId is the workflow
  // run's ID in a Tool node, or the agent run's ID when an agent calls it.
  return { title: 'General' };
});
```

- **`description`:** required. Models read it to decide when to use the tool, so say what it does and when to use it.
- **`inputSchema`:** input is checked against it before your handler runs.
- **`readOnly`:** set this only if the tool changes and sends nothing.
- **`requiresApproval`:** a person approves each call from a Tool node. It defaults to `true` unless the tool is read-only. Agents can't pause for approval, so an agent that calls a tool needing it gets an error back. Setting it to `false` on a tool with side effects lets workflows and agents act with no check, so only do that for low-risk actions.
- **Results:** return any JSON value, up to 1 MiB. Later nodes read it as `nodes.<id>`, and models see it, so never return a secret.
- **Errors:** throw to fail the call. Secret setting values are removed from the message. A model can read the error and try again.

The name of the plugin and the tool can be 62 characters together. A tool can't replace one that already exists: it's skipped and the log says why.

### Workflow events

```json
"events": [{ "name": "message_received", "description": "Someone messaged the bot. event: {chat_id, text}" }]
```

```js
host.workflows.emit('message_received', { chat_id: '42', text: 'Hello' });
```

Workflows can then start on `telegram.message_received` or wait for it, and read the payload as `event.*`. Say what the payload holds in the description, because that's what people and models see when they pick the event. Payloads can be up to 1 MiB.

Only emit from a handler, not while the script loads. Emit once for each real event: webhook providers retry, so check `req.once` first. Anyone who can reach an unverified webhook can trigger what it emits, so verify the request before you emit.

### Agent presets

A preset is a ready-made agent. No code is needed.

```json
"agentPresets": [{
  "name": "reply_drafter",
  "description": "Drafts a short reply.",
  "systemPrompt": "Draft a reply to: {{ .input.text }}",
  "tools": ["telegram.get_chat"],
  "maxSteps": 3,
  "outputSchema": { "type": "object", "properties": { "reply": { "type": "string" } } }
}]
```

Only `name` and `systemPrompt` are required. `ai` sets model options. Anything set on the Agent node overrides the preset, and the node's tools replace the preset's rather than adding to them. Workflows warn about tools with side effects in a preset the same way they do for the node's own tools.

### Field editors

A field editor replaces the plain input for one field of a tool in the builder.

```json
"fieldEditors": [{ "name": "chat", "description": "Pick a chat." }],
"tools": [{ "name": "get_chat", "description": "…", "inputSchema": { "type": "object", "properties": {
  "chat_id": { "type": "string", "x-ui": "telegram.chat" }
} } }]
```

```js
export default function setup(host) {
  host.fields.add('chat', defineComponent({
    props: ['modelValue', 'label', 'hint', 'readonly', 'schema', 'options'],
    emits: ['update:modelValue'],
    setup(props, { emit }) { /* render an input */ },
  }));
}
```

`x-ui` is an editor's name, or `{ "widget": "telegram.chat", ...options }` to pass `options` to it. An editor needs a `ui` entry, and it can be used by another plugin's tools. If it isn't running, the field gets a plain input, so the stored value must make sense on its own. People can also type `{{ }}` expressions into tool inputs, so an editor shouldn't reject them.

## Consent

Channel consent is separate from email and SMS. A contact gets a campaign on a plugin channel only if they have confirmed consent for one of its lists on that channel and an active address. With no consent row, nothing is sent. Opt-in campaigns aren't available on plugin channels.

An opt-out is channel-wide and stays until new consent is given. A rejected address is different: it stops sends to that address but isn't an opt-out, and a message from that address clears it. Status updates can arrive in any order, including before the send is recorded, and never move backwards.

A contact created by `upsertByAddress` has no email. They're stored with a placeholder under `no-email.invalid`, which is never shown or sent to and can't be claimed by anyone. Entering a real address on their subscriber page replaces it.

## Browser code

`ui/index.js` is an ES module that exports a setup function. Imports of `vue`, `vuetify` and `vuetify/components` resolve to the host's copies, so keep them external if you use a bundler. Global Vuetify components such as `v-btn` are available through `resolveComponent`.

```js
import { defineComponent, h, resolveComponent } from 'vue';

export default function setup(host) {
  const Page = defineComponent({
    setup: () => () => h(resolveComponent('v-alert'), { text: host.t('telegram.note') }),
  });

  host.routes.add({ name: 'contacts', path: 'contacts', title: 'telegram.contacts', component: Page, permission: 'plugin_telegram:manage' });
  host.nav.add({ route: 'contacts', label: 'Contacts', icon: '<SVG path data>', parent: 'settings' });
  host.settings.addTab({ key: 'advanced', label: 'Advanced', component: Page });
  host.slots.add('subscriber.form.after', Page);
}
```

- **`host`** also has `fetch(path, options)` for your `adminRoutes`, `fields` for [field editors](#field-editors), `t`, `can`, `router` and `toast`.
- **Paths and names** are namespaced: the route above is `/plugins/telegram/contacts`.
- **Icons:** use SVG path data. Named `mdi-*` icons only work if the host already bundles them.
- **Slots:** `campaign.form.after`, `template.form.after`, `subscriber.form.after`, `subscriber.activity.item`, `dashboard.widgets`.
- **Failures:** a module that throws or takes over 5 seconds is reported on the Plugins page and skipped. A component that throws is replaced by an error notice.
- **Secrets:** files in `ui/` are served without login, so never put secrets in them.
