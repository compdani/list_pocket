import test from 'node:test';
import assert from 'node:assert/strict';
import { decodeEmailSubject } from '../src/utils/emailSubject.mjs';

test('decodes UTF-8 base64, quoted printable, and adjacent encoded words', () => {
  assert.equal(decodeEmailSubject('=?UTF-8?B?SGVsbG8g4pyT?='), 'Hello ✓');
  assert.equal(decodeEmailSubject('=?utf-8?Q?Final=20notice_=E2=9C=93?='), 'Final notice ✓');
  assert.equal(decodeEmailSubject('=?UTF-8?Q?Hello_?=\r\n =?UTF-8?B?d29ybGQ=?='), 'Hello world');
  assert.equal(decodeEmailSubject('=?ISO-8859-1?Q?caf=E9?='), 'café');
});

test('retains plain subjects and undecodable encoded words', () => {
  assert.equal(decodeEmailSubject('Re: Ordinary subject'), 'Re: Ordinary subject');
  assert.equal(decodeEmailSubject('=?unsupported?B?YQ==?='), '=?unsupported?B?YQ==?=');
  assert.equal(decodeEmailSubject('=?utf-8?B?%%%?='), '=?utf-8?B?%%%?=');
  assert.equal(decodeEmailSubject(null), '');
});
