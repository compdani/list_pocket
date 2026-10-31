// Helpers shared by the conversation, contact, task and board views.
// Stays in step with PLACEHOLDER_EMAIL_DOMAIN in channels.js.
const PLACEHOLDER_EMAIL_DOMAIN = 'no-email.invalid';

const CHANNEL_ICONS = {
  email: 'mdi-email-outline',
  sms: 'mdi-cellphone-message',
  'ch:whatsapp': 'mdi-whatsapp',
};

export function channelIcon(channel) {
  return CHANNEL_ICONS[channel] || 'mdi-message-text-outline';
}

// Timestamps come back as "2026-01-02 10:00:00.000Z", which not every browser parses.
export function parseStamp(value) {
  if (!value) {
    return null;
  }
  const date = new Date(String(value).trim().replace(' ', 'T'));
  return Number.isNaN(date.getTime()) ? null : date;
}

function isSameDay(a, b) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

// A short label for a list row: the time today, otherwise the date.
export function shortStamp(value, now = new Date(), locale = undefined) {
  const date = parseStamp(value);
  if (!date) {
    return '';
  }
  if (isSameDay(date, now)) {
    return date.toLocaleTimeString(locale, { hour: 'numeric', minute: '2-digit' });
  }
  const opts = { month: 'short', day: 'numeric' };
  if (date.getFullYear() !== now.getFullYear()) {
    opts.year = 'numeric';
  }
  return date.toLocaleDateString(locale, opts);
}

export function initials(name, fallback = '') {
  const source = String(name || fallback || '').trim();
  if (!source) {
    return '?';
  }
  const words = source.split(/[\s@._-]+/).filter(Boolean);
  const letters = words.length > 1 ? words[0][0] + words[1][0] : (words[0] || source).slice(0, 2);
  return letters.toUpperCase();
}

export function conversationName(conversation) {
  return conversation.name || conversation.email || conversation.address || '';
}

export function visibleEmail(email) {
  const value = String(email || '').trim();
  return value.toLowerCase().endsWith(`@${PLACEHOLDER_EMAIL_DOMAIN}`) ? '' : value;
}

// Splits messages into days, keeping their order, for the dividers in a thread.
export function groupByDay(messages, locale = undefined) {
  const groups = [];
  messages.forEach((message) => {
    const date = parseStamp(message.occurredAt);
    const key = date ? `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}` : '';
    let group = groups[groups.length - 1];
    if (!group || group.key !== key) {
      group = {
        key,
        label: date ? date.toLocaleDateString(locale, { dateStyle: 'medium' }) : '',
        messages: [],
      };
      groups.push(group);
    }
    group.messages.push(message);
  });
  return groups;
}

// Picks the channel a reply should start on: the last one the person wrote on, when it
// can be used, then the first that can.
export function defaultReplyChannel(options, messages) {
  const usable = (options || []).filter((o) => o.available);
  if (usable.length === 0) {
    return (options && options[0] && options[0].channel) || '';
  }
  const inbound = [...(messages || [])].reverse().find((m) => m.direction === 'inbound');
  if (inbound && usable.some((o) => o.channel === inbound.channel)) {
    return inbound.channel;
  }
  return usable[0].channel;
}

export function replySubject(messages) {
  const last = [...(messages || [])].reverse().find((m) => m.channel === 'email' && m.subject);
  if (!last) {
    return '';
  }
  return /^re:/i.test(last.subject) ? last.subject : `Re: ${last.subject}`;
}

// Which list on the tasks page a task belongs in.
export function taskBucket(task, now = new Date()) {
  const due = parseStamp(task.dueAt);
  if (!due) {
    return 'noDate';
  }
  if (isSameDay(due, now)) {
    return 'today';
  }
  return due < now ? 'overdue' : 'upcoming';
}

export function groupTasks(tasks, now = new Date()) {
  const out = {
    overdue: [], today: [], upcoming: [], noDate: [],
  };
  tasks.forEach((task) => out[taskBucket(task, now)].push(task));
  return out;
}

// Turns a <input type="datetime-local"> value into the RFC 3339 time the API wants.
export function localInputToRFC3339(value) {
  if (!value) {
    return '';
  }
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '' : date.toISOString();
}

// The document an inbound email is shown in. Its HTML comes from anyone who can send
// mail, so the frame it goes in must have scripts disabled, and this policy also stops
// it loading anything. Remote images are off until asked for, because they tell the
// sender when the email was read.
export function emailFrameDoc(html, { showImages = false } = {}) {
  const images = showImages ? 'data: cid: https: http:' : 'data: cid:';
  const csp = `default-src 'none'; img-src ${images}; style-src 'unsafe-inline'; font-src data:`;
  return `<!doctype html><html><head><meta charset="utf-8">`
    + `<meta http-equiv="Content-Security-Policy" content="${csp}">`
    + '<base target="_blank">'
    + '<style>html,body{margin:0;padding:0;background:#fff;color:#1f2933;font:14px/1.5 system-ui,sans-serif;overflow-wrap:anywhere}img{max-width:100%;height:auto}table{max-width:100%}</style>'
    + `</head><body>${html || ''}</body></html>`;
}
