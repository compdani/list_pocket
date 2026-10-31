// Handlers run on pooled VMs and share nothing, so all state lives in host.kv or in
// the app's tables. Host calls are synchronous.

function api(method, payload) {
  var s = host.settings.get();
  return host.fetch(s.api_base + '/bot' + s.bot_token + '/' + method, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

plugin.channel('ch:telegram', {
  send: function (msg) {
    var invalid = [];
    var messageId = '';
    var addresses = msg.to;
    for (var i = 0; i < addresses.length; i++) {
      var res = api('sendMessage', {
        chat_id: addresses[i],
        text: msg.body,
        disable_web_page_preview: Boolean(msg.fields && msg.fields.disable_preview),
      });
      if (res.status === 403 || res.status === 400) {
        // The user blocked the bot or the chat is gone. Retrying can't help.
        invalid.push(addresses[i]);
        continue;
      }
      if (!res.ok) {
        throw new Error('Telegram returned ' + res.status);
      }
      messageId = String(res.json().result.message_id);
    }
    return { providerMsgId: messageId, invalidAddresses: invalid };
  },
});

plugin.webhook('inbound', {
  verify: function (req) {
    var expected = host.settings.get().webhook_secret;
    var got = req.headers['x-telegram-bot-api-secret-token'] || '';
    return Boolean(expected) && host.crypto.timingSafeEqual(got, expected);
  },
  handle: function (req) {
    var update = req.json();
    // Telegram retries until it gets a 200, so handle each update once.
    if (!req.once('update:' + update.update_id, 7 * 24 * 3600)) {
      return;
    }
    var message = update.message;
    if (!message || !message.text) {
      return;
    }

    var chatId = String(message.chat.id);
    var name = [message.from && message.from.first_name, message.from && message.from.last_name]
      .filter(Boolean).join(' ') || chatId;
    var parts = message.text.trim().split(/\s+/);

    if (parts[0] === '/start') {
      // /start <list uuid> comes from a t.me/<bot>?start=<list uuid> link.
      var contact = host.contacts.upsertByAddress('ch:telegram', chatId, {
        name: name,
        lists: parts[1] ? [parts[1]] : [],
        consent: 'confirmed',
      });
      var seen = host.kv.get('contacts') || [];
      if (!seen.some(function (c) { return c.chat_id === chatId; })) {
        seen.push({ chat_id: chatId, name: name, first_seen: new Date(message.date * 1000).toISOString() });
        host.kv.set('contacts', seen);
      }
      host.events.record({
        channel: 'ch:telegram', direction: 'inbound', kind: 'subscribe',
        providerMsgId: 'u' + update.update_id, subscriberId: contact.id, address: chatId, body: message.text,
      });
      return;
    }

    if (parts[0] === '/stop') {
      host.events.optOut('ch:telegram', chatId);
    }

    var existing = host.contacts.findByAddress('ch:telegram', chatId);
    host.events.record({
      channel: 'ch:telegram', direction: 'inbound', kind: parts[0] === '/stop' ? 'opt_out' : 'message',
      providerMsgId: 'u' + update.update_id, subscriberId: existing ? existing.id : '', address: chatId, body: message.text,
    });
    host.workflows.emit('message_received', {
      chat_id: chatId, name: name, text: message.text, contact_id: existing ? existing.id : '',
    });
  },
});

// A workflow's Tool node or an agent can call this as telegram.get_chat.
plugin.tool('get_chat', function (input) {
  var res = api('getChat', { chat_id: input.chat_id });
  if (!res.ok) {
    throw new Error('Telegram returned ' + res.status);
  }
  var chat = res.json().result;
  return { id: String(chat.id), type: chat.type, title: chat.title || '', name: chat.first_name || '', username: chat.username || '' };
});

plugin.adminRoute('contacts', function () {
  return { json: { contacts: host.kv.get('contacts') || [] } };
});
