import test from 'node:test';
import assert from 'node:assert/strict';
import {
  channelGroup,
  findChannel,
  isPlaceholderEmail,
  isTextMessenger,
  messengersInGroup,
  pluginChannels,
  templateTypeFor,
} from '../src/utils/channels.js';

const channels = [
  { id: 'email', kind: 'email' },
  { id: 'email-work', kind: 'email' },
  { id: 'webhooker', kind: 'email' },
  { id: 'quo', kind: 'text' },
  { id: 'ch:telegram', kind: 'text' },
];

test('postback and SMTP messengers count as email, matching the backend', () => {
  for (const m of ['email', 'email-work', 'webhooker']) {
    assert.equal(isTextMessenger(channels, m), false, m);
    assert.equal(channelGroup(channels, m), 'email', m);
    assert.equal(templateTypeFor(channels, m), 'campaign', m);
  }
});

test('quo is the sms group and plugin channels are their own group', () => {
  assert.equal(channelGroup(channels, 'quo'), 'sms');
  assert.equal(channelGroup(channels, 'ch:telegram'), 'ch:telegram');
  assert.equal(templateTypeFor(channels, 'quo'), 'campaign_sms');
});

test('unlisted messengers follow the backend default', () => {
  assert.equal(isTextMessenger([], 'quo'), true);
  assert.equal(isTextMessenger([], 'ch:gone'), true);
  assert.equal(isTextMessenger([], 'something'), false);
  assert.equal(isTextMessenger(undefined, ''), false);
});

test('messengersInGroup and pluginChannels filter the lists', () => {
  const names = channels.map((c) => c.id);
  assert.deepEqual(messengersInGroup(channels, names, 'email'), ['email', 'email-work', 'webhooker']);
  assert.deepEqual(messengersInGroup(channels, names, 'sms'), ['quo']);
  assert.deepEqual(pluginChannels(channels).map((c) => c.id), ['ch:telegram']);
  assert.equal(findChannel(channels, ' quo ').id, 'quo');
  assert.equal(findChannel(null, 'quo'), null);
});

test('placeholder emails are recognised and real ones are not', () => {
  assert.equal(isPlaceholderEmail('abc@no-email.invalid'), true);
  assert.equal(isPlaceholderEmail(' ABC@NO-EMAIL.INVALID '), true);
  assert.equal(isPlaceholderEmail('abc@example.com'), false);
  assert.equal(isPlaceholderEmail(''), false);
  assert.equal(isPlaceholderEmail(null), false);
});
