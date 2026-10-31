# WhatsApp

List Pocket can send campaigns and transactional messages on WhatsApp, record delivery and replies, and let staff answer from a subscriber's page. Set it up under **Settings → WhatsApp**.

It runs on one of two providers, and one is active at a time:

| | Meta Cloud API | Whapi.Cloud |
|---|---|---|
| Official | Yes | No |
| Campaign content | A template approved in WhatsApp Manager | Plain text you write in the campaign |
| Replies | Within 24 hours of the person's last message | Any time |
| WhatsApp Channel posts | Not available | Available |
| Default send rate | 40 per second | 1 per second |

Use the Cloud API unless you need Channel posts.

## Risks of Whapi

Whapi isn't an official WhatsApp API. It links a phone by QR code.

- Automating a linked number could get it restricted or banned. There's no reliable figure for how likely that is, or for what volume is safe, and the risk is highest for campaigns to individuals. Use a number you can afford to lose.
- It could stop working if WhatsApp changes.
- It's a paid third party that handles your messages.

You have to accept these risks in settings before it can be enabled. The default rate of 1 per second is deliberately cautious, not a known safe rate.

## Setting up the Cloud API

You need a Meta app with the WhatsApp use case, a WhatsApp Business Account, a phone number, and a system user token with the `whatsapp_business_messaging` and `whatsapp_business_management` permissions.

1. Enter the token, app secret, a verify token you make up, the phone number ID and the business account ID, then save.
2. Use **Test connection**.
3. In your Meta app, set the webhook to the URL shown in settings, with the same verify token.
4. Subscribe to the `messages` and `user_preferences` webhook fields.

Create templates in WhatsApp Manager. Only approved ones appear in List Pocket.

## Setting up Whapi

1. Accept the risk notice, enter your Whapi token and save. The app restarts.
2. Copy the webhook URL shown in settings into your Whapi channel. It contains a secret, so keep it private.
3. Enable the `messages` and `statuses` events, and `users` to be told when the phone disconnects.

## Consent

WhatsApp has its own consent, separate from email and SMS. A person only gets a campaign if they've consented for one of its lists on WhatsApp and have a phone number. Messaging you isn't consent for campaigns.

Consent can come from:

- **A `JOIN` message:** someone sends `JOIN <list uuid>`. Share it as a `https://wa.me/<number>?text=JOIN%20<list%20uuid>` link.
- **A subscriber's page:** under **Other channels**.
- **Copying from SMS:** under **Settings → WhatsApp**. It shows counts before changing anything, and never overrides an opt-out. Run it again to include newer subscribers.

Meta requires that your opt-in named your business and said people would receive messages from it. List Pocket can't check your wording, so copying consent asks you to confirm it.

Sending `STOP` or `UNSUBSCRIBE` opts someone out, along with anyone sharing that number, until they opt in again. A person stopping marketing messages in WhatsApp itself does the same. If they later resume, consent isn't restored automatically. An SMS STOP also withdraws consent copied from SMS, but a WhatsApp STOP doesn't affect SMS.

## Campaigns

Choose **WhatsApp** as the campaign's channel.

On the Cloud API, pick a template and fill in its values. A value can include `{{name}}`, `{{first_name}}`, `{{last_name}}` or `{{attribs.key}}`, and needs a fallback for subscribers where that's empty. Header media, body and header text, and URL button values are supported.

On Whapi, write the message in the Content tab.

Phone numbers need a `+` and country code. Others are skipped, because the API could otherwise reach the wrong person.

| Result | What happens |
|---|---|
| Number isn't on WhatsApp (131026) | The number is retired and a bounce is recorded. It isn't an opt-out, and a message from that number restores it |
| Person stopped marketing messages (131050) | They're opted out |
| Meta's per-user limit (131049) | Shown as failed. Not retried automatically |
| Rate limits, or account and template problems | Counts as a send error, and too many pause the campaign |

Bounces on WhatsApp never run the email bounce actions, so they can't unsubscribe or delete anyone.

As with email and SMS, if a campaign finishes with fewer errors than the pause threshold, those recipients aren't retried automatically.

## Transactional messages

Send to `/api/tx` with `messenger` set to `ch:whatsapp`. On the Cloud API, name the template in the data:

```json
{
  "subscriber_record_ids": ["<id>"],
  "template_id": 1,
  "messenger": "ch:whatsapp",
  "data": {
    "whatsapp_template": {
      "name": "order_update",
      "language": "en_US",
      "components": [{ "type": "body", "parameters": [{ "value": "{{first_name}}", "fallback": "there" }] }]
    }
  }
}
```

The subscriber needs WhatsApp consent. On Whapi, the List Pocket template's text is sent.

## Replies

A subscriber's page shows their WhatsApp conversation and, with the `whatsapp:reply` permission, a reply box. On the Cloud API you can only reply within 24 hours of their last message. Nobody who opted out can be messaged. Replies appear on the subscriber's timeline. Inbound media shows as its type, and isn't downloaded.

## WhatsApp Channel posts

With Whapi active, **Campaigns → WhatsApp Channel** posts to channels the linked account administers. This needs the `whatsapp:post` permission. Posts can be text, an image, a video or a link preview, and can be drafted or scheduled. A link preview needs a title, and its text must include the URL.

A post goes to the channel's followers, so it doesn't use lists or consent. A failed post is never retried automatically, because it might have gone out anyway. Check the channel, then retry it by hand. Whapi suggests 1 to 10 posts a day, and the page warns above that.
