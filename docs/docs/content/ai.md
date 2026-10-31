# AI and the agent

The **Agent** page, the assistant side panel, workflow AI steps and the campaign builder all
use the provider chosen under **Settings → AI**.

## Providers

- **API key.** Set `LISTPOCKET_AI_API_KEY`, and optionally `LISTPOCKET_AI_BASE_URL` and
  `LISTPOCKET_AI_MODEL`. Any OpenAI-compatible service works. The assistant asks for replies of
  up to 128,000 tokens, or `LISTPOCKET_AI_MAX_OUTPUT_TOKENS` if you set it, where `0` asks for no
  limit. If a model refuses that, its own limit is used instead.
- **ChatGPT plan.** Sign in with ChatGPT, and requests count toward your plan's Codex usage
  instead of per-token API costs. It needs Plus or Pro.

OpenAI offers plan usage for open-source and locally hosted apps. For a paid or remotely hosted
deployment, they ask you to apply through their
[interest form](https://help.openai.com/en/articles/20001542-using-your-chatgpt-plan-in-other-apps-and-sites).

### Signing in

Go to **Settings → AI → Sign in with ChatGPT**. List Pocket listens on `127.0.0.1:1455` for
OpenAI's callback, so this finishes by itself when the browser and List Pocket are on the same
computer. On a remote server, the sign-in ends on a page that can't be reached: copy that page's
full address and paste it into the box on the settings page.

You can also import a session exported by Go-Harness (`harness chatgpt login --export`).

### What's different on a plan

- Run costs show as $0, so workflow AI budgets don't apply. Limit AI steps with their maximum
  steps setting instead.
- Temperature and token limits aren't sent, because the plan route rejects them.
- When the plan's limit is reached, requests fail until it resets. Switch model or provider to
  carry on.
- Sessions renew automatically. Signing out revokes the session at OpenAI and returns anything
  defaulting to the plan to the API key.

Session tokens are stored in the `oauth_credentials` collection, which only superusers can read.
They aren't encrypted separately from the rest of `pb_data`, so protect its backups accordingly.

## The Agent page

Conversations are private to the person who has them, and each can use its own model.

- **Context.** Type `@` or use **Context** to attach contacts, conversations, campaigns, lists,
  templates, workflows and runs. It's read fresh on each turn, through the same permissions as
  the rest of the app, so nobody can attach what they can't already see. Click anything attached
  to see what the agent sees.
- **Files.** Attach or drop text files (up to 2 MB), images and PDFs (up to 6 MB), or paste data.
  Files go to the configured AI provider. Text files stay in context for the whole conversation,
  and images and PDFs are sent with the message they're attached to.
- **Proposals.** The agent can read freely, but every change comes back as a proposal. **Review**
  opens a preview you can edit: a graph of what changes for workflows, the rendered email for
  campaigns and templates, and the exact text for replies. Nothing happens until you apply it.
  A proposal that fails isn't retried, because it may have partly worked.
