import test from 'node:test';
import assert from 'node:assert/strict';

import {
  defaultReplyChannel, emailFrameDoc, groupByDay, groupTasks, initials, parseStamp,
  replySubject, shortStamp, taskBucket, visibleEmail,
} from '../src/utils/crm.mjs';

test('parseStamp reads the API format and rejects junk', () => {
  assert.equal(parseStamp('2026-01-02 10:00:00.000Z').toISOString(), '2026-01-02T10:00:00.000Z');
  assert.equal(parseStamp(''), null);
  assert.equal(parseStamp('nope'), null);
});

test('shortStamp shows the year only when it differs', () => {
  const now = new Date(2026, 5, 15, 12, 0);
  assert.match(shortStamp(new Date(2026, 5, 15, 9, 5).toISOString(), now, 'en-US'), /9:05/);
  assert.equal(shortStamp(new Date(2026, 0, 3).toISOString(), now, 'en-US'), 'Jan 3');
  assert.equal(shortStamp(new Date(2024, 0, 3).toISOString(), now, 'en-US'), 'Jan 3, 2024');
});

test('initials handles names, addresses and nothing', () => {
  assert.equal(initials('Ada Lovelace'), 'AL');
  assert.equal(initials('', 'ada@example.com'), 'AE');
  assert.equal(initials('Cher'), 'CH');
  assert.equal(initials(''), '?');
});

test('visibleEmail hides placeholder addresses', () => {
  assert.equal(visibleEmail('a@example.com'), 'a@example.com');
  assert.equal(visibleEmail('x@no-email.invalid'), '');
});

test('groupByDay starts a new group each day and keeps order', () => {
  const groups = groupByDay([
    { id: 1, occurredAt: new Date(2026, 0, 1, 9).toISOString() },
    { id: 2, occurredAt: new Date(2026, 0, 1, 17).toISOString() },
    { id: 3, occurredAt: new Date(2026, 0, 2, 9).toISOString() },
  ]);
  assert.deepEqual(groups.map((g) => g.messages.map((m) => m.id)), [[1, 2], [3]]);
});

test('defaultReplyChannel prefers where the person last wrote, if usable', () => {
  const options = [
    { channel: 'email', available: true },
    { channel: 'sms', available: false, reason: 'opted_out' },
    { channel: 'ch:whatsapp', available: true },
  ];
  const inbound = (channel) => ({ channel, direction: 'inbound' });
  assert.equal(defaultReplyChannel(options, [inbound('email'), inbound('ch:whatsapp')]), 'ch:whatsapp');
  assert.equal(defaultReplyChannel(options, [inbound('sms')]), 'email', 'opted-out channel is skipped');
  assert.equal(defaultReplyChannel(options, []), 'email');
  assert.equal(defaultReplyChannel([{ channel: 'email', available: false }], []), 'email');
  assert.equal(defaultReplyChannel([], []), '');
});

test('replySubject answers the latest email without stacking prefixes', () => {
  assert.equal(replySubject([{ channel: 'email', subject: 'Hi' }, { channel: 'sms', subject: '' }]), 'Re: Hi');
  assert.equal(replySubject([{ channel: 'email', subject: 'RE: Hi' }]), 'RE: Hi');
  assert.equal(replySubject([{ channel: 'sms', subject: '' }]), '');
});

test('tasks sort into buckets by due date', () => {
  const now = new Date(2026, 5, 15, 12, 0);
  assert.equal(taskBucket({ dueAt: '' }, now), 'noDate');
  assert.equal(taskBucket({ dueAt: new Date(2026, 5, 15, 8).toISOString() }, now), 'today', 'earlier today is not overdue');
  assert.equal(taskBucket({ dueAt: new Date(2026, 5, 14).toISOString() }, now), 'overdue');
  assert.equal(taskBucket({ dueAt: new Date(2026, 5, 16).toISOString() }, now), 'upcoming');
  const grouped = groupTasks([{ dueAt: '' }, { dueAt: new Date(2026, 5, 14).toISOString() }], now);
  assert.equal(grouped.noDate.length, 1);
  assert.equal(grouped.overdue.length, 1);
});

test('emailFrameDoc blocks scripts and remote images until asked', () => {
  const blocked = emailFrameDoc('<img src="https://t.example/p.gif">');
  assert.match(blocked, /default-src 'none'/);
  assert.doesNotMatch(blocked, /img-src[^;]*https:/);
  assert.doesNotMatch(blocked, /script-src/);
  assert.match(emailFrameDoc('', { showImages: true }), /img-src[^;]*https:/);
});
