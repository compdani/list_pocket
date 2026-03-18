import fs from 'node:fs';
import path from 'node:path';
import { baseCompile } from '@intlify/message-compiler';

const NAMED_PLACEHOLDER_RE = /\{([A-Za-z_][A-Za-z0-9_]*)\}/g;
const SIMPLE_PLACEHOLDER_RE = /\{([^{}]+)\}/g;
const PLACEHOLDER_TOKEN_PREFIX = '__LISTPOCKET_I18N_PLACEHOLDER_';

function isNamedPlaceholderName(value) {
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(value);
}

function isPlaceholderCandidate(value) {
  return /^[^"'`:[\].,{}\s]+$/.test(value);
}

function extractNamedPlaceholders(message) {
  const placeholders = [];
  if (typeof message !== 'string') {
    return placeholders;
  }

  message.replace(NAMED_PLACEHOLDER_RE, (_, name) => {
    placeholders.push(name);
    return _;
  });

  return placeholders;
}

function normalizeLocaleMessage(message, referenceMessage) {
  if (typeof message !== 'string' || message.length === 0) {
    return message;
  }

  const referencePlaceholders = extractNamedPlaceholders(referenceMessage);
  let placeholderIndex = 0;

  let normalized = message.replace(SIMPLE_PLACEHOLDER_RE, (match, inner) => {
    const token = inner.trim();
    const expected = referencePlaceholders[placeholderIndex];

    if (isNamedPlaceholderName(token)) {
      placeholderIndex += 1;
      if (expected && !referencePlaceholders.includes(token)) {
        return `{${expected}}`;
      }
      return match;
    }

    if (!expected || !isPlaceholderCandidate(token)) {
      return match;
    }

    placeholderIndex += 1;
    return `{${expected}}`;
  });

  const protectedPlaceholders = [];
  normalized = normalized.replace(NAMED_PLACEHOLDER_RE, (match) => {
    const token = `${PLACEHOLDER_TOKEN_PREFIX}${protectedPlaceholders.length}__`;
    protectedPlaceholders.push(match);
    return token;
  });

  normalized = normalized
    .replace(/\{/g, "{'{'}")
    .replace(/\}/g, "{'}'}")
    .replace(/@/g, "{'@'}");

  protectedPlaceholders.forEach((placeholder, index) => {
    normalized = normalized.replace(`${PLACEHOLDER_TOKEN_PREFIX}${index}__`, placeholder);
  });

  return normalized;
}

function normalizeLocaleMessages(messages, referenceMessages = {}) {
  if (Array.isArray(messages)) {
    return messages.map((value, index) => normalizeLocaleMessages(
      value,
      Array.isArray(referenceMessages) ? referenceMessages[index] : undefined,
    ));
  }

  if (!messages || typeof messages !== 'object') {
    return messages;
  }

  return Object.fromEntries(Object.entries(messages).map(([key, value]) => {
    const referenceValue = referenceMessages && typeof referenceMessages === 'object'
      ? referenceMessages[key]
      : undefined;

    if (typeof value === 'string') {
      return [
        key,
        normalizeLocaleMessage(value, typeof referenceValue === 'string' ? referenceValue : ''),
      ];
    }

    if (value && typeof value === 'object') {
      return [key, normalizeLocaleMessages(value, referenceValue)];
    }

    return [key, value];
  }));
}

const localeDir = path.resolve('..', 'i18n');
const files = fs.readdirSync(localeDir).filter((name) => name.endsWith('.json')).sort();
const en = JSON.parse(fs.readFileSync(path.join(localeDir, 'en.json'), 'utf8'));
const normalizedEn = normalizeLocaleMessages(en, en);

let count = 0;
for (const file of files) {
  const fullPath = path.join(localeDir, file);
  const messages = JSON.parse(fs.readFileSync(fullPath, 'utf8'));
  const normalized = file === 'en.json'
    ? normalizedEn
    : normalizeLocaleMessages(messages, normalizedEn);

  for (const [key, value] of Object.entries(normalized)) {
    if (typeof value !== 'string') {
      continue;
    }

    try {
      baseCompile(value, { jit: false, onError: (error) => { throw error; } });
    } catch (error) {
      count += 1;
      console.log(`${file}\t${key}\t${error.code ?? ''}\t${error.message}`);
    }
  }
}

console.log(`TOTAL_ERRORS=${count}`);
