// Helpers over the `channels` list in /mailapi/config. A channel "group" is what
// the campaign form's picker selects: all email-style messengers share 'email',
// Quo is 'sms', and every plugin channel is its own group.

export const EMAIL_GROUP = 'email';
export const SMS_GROUP = 'sms';
export const QUO_MESSENGER = 'quo';
export const PLUGIN_PREFIX = 'ch:';

// Contacts who only exist on a plugin channel store this domain, because email is
// required in the database. It's never shown or sent to.
export const PLACEHOLDER_EMAIL_DOMAIN = 'no-email.invalid';

export function isPlaceholderEmail(email) {
  return String(email || '').trim().toLowerCase().endsWith(`@${PLACEHOLDER_EMAIL_DOMAIN}`);
}

export function findChannel(channels, messenger) {
  const id = String(messenger || '').trim();
  return (Array.isArray(channels) ? channels : []).find((c) => c.id === id) || null;
}

export function isTextMessenger(channels, messenger) {
  const id = String(messenger || '').trim();
  const channel = findChannel(channels, id);
  if (channel) {
    return channel.kind === 'text';
  }
  // Stays in step with models.ChannelFor when a messenger isn't in the list.
  return id === QUO_MESSENGER || id.startsWith(PLUGIN_PREFIX);
}

export function channelGroup(channels, messenger) {
  const id = String(messenger || '').trim();
  if (!isTextMessenger(channels, id)) {
    return EMAIL_GROUP;
  }
  return id === QUO_MESSENGER ? SMS_GROUP : id;
}

export function messengersInGroup(channels, messengers, group) {
  return (Array.isArray(messengers) ? messengers : [])
    .filter((m) => channelGroup(channels, m) === group);
}

export function pluginChannels(channels) {
  return (Array.isArray(channels) ? channels : [])
    .filter((c) => String(c.id).startsWith(PLUGIN_PREFIX));
}

export function templateTypeFor(channels, messenger) {
  return isTextMessenger(channels, messenger) ? 'campaign_sms' : 'campaign';
}
