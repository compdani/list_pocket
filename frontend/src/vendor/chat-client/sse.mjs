// Server-Sent Events parser for fetch streams.
//
// `parseSSE` turns a ReadableStream of bytes into an async iterator of
// `{ id, event, data }` frames. It follows the SSE field rules closely enough
// for the chat API and for ordinary event streams:
//
//   - chunks may split anywhere, including inside a multi-byte UTF-8 sequence
//     (the decoder is fed with `{ stream: true }`);
//   - LF, CRLF and lone CR all end a line, including a CR at a chunk boundary;
//   - several `data:` lines join with "\n";
//   - comment lines start with ":" and never produce a frame; `onComment`
//     receives their text so a caller can reset an idle timer;
//   - a final frame without a trailing blank line is still dispatched;
//   - one frame is bounded by `maxEventBytes` (UTF-8 bytes, default 1 MiB).

const DEFAULT_MAX_EVENT_BYTES = 1024 * 1024;

/** An SSE framing or size error. `code` is stable; the message is for humans. */
export class SSEError extends Error {
  constructor(message, code = 'sse_error') {
    super(message);
    this.name = 'SSEError';
    this.code = code;
  }
}

/** UTF-8 bytes a string would occupy, without allocating an encoder. */
export function utf8ByteLength(value) {
  let bytes = 0;
  for (let i = 0; i < value.length; i += 1) {
    const c = value.charCodeAt(i);
    if (c < 0x80) {
      bytes += 1;
    } else if (c < 0x800) {
      bytes += 2;
    } else if (c >= 0xd800 && c <= 0xdbff && i + 1 < value.length) {
      const next = value.charCodeAt(i + 1);
      if (next >= 0xdc00 && next <= 0xdfff) {
        bytes += 4;
        i += 1;
      } else {
        bytes += 3;
      }
    } else {
      bytes += 3;
    }
  }
  return bytes;
}

function abortReason(signal) {
  const reason = signal?.reason;
  if (reason !== undefined && reason !== null) return reason;
  if (typeof DOMException === 'function') {
    return new DOMException('The operation was aborted.', 'AbortError');
  }
  const err = new Error('The operation was aborted.');
  err.name = 'AbortError';
  return err;
}

/**
 * Parse a byte stream as Server-Sent Events.
 *
 * @param {ReadableStream<Uint8Array>} stream
 * @param {object} [options]
 * @param {AbortSignal} [options.signal] Aborts the read and rejects the iteration.
 * @param {number} [options.maxEventBytes] Largest single frame, in UTF-8 bytes.
 * @param {(comment: string) => void} [options.onComment] Called for every `:` line,
 *   with the text after the colon (one leading space removed).
 * @returns {AsyncGenerator<{ id: string, event: string, data: string }>}
 */
export async function* parseSSE(stream, options = {}) {
  const { signal, onComment } = options;
  const maxEventBytes = options.maxEventBytes ?? DEFAULT_MAX_EVENT_BYTES;

  if (!stream || typeof stream.getReader !== 'function') {
    throw new TypeError('parseSSE() requires a ReadableStream');
  }
  if (maxEventBytes <= 0 || !Number.isFinite(maxEventBytes)) {
    throw new RangeError('maxEventBytes must be a positive finite number');
  }
  if (signal?.aborted) throw abortReason(signal);

  const reader = stream.getReader();
  const decoder = new TextDecoder('utf-8');
  let buffer = '';
  let finalFlush = false;
  let aborted = null;
  let finished = false;

  const onAbort = () => {
    aborted = abortReason(signal);
    // cancel() makes a pending read() settle, so the loop below can throw.
    try {
      Promise.resolve(reader.cancel(aborted)).catch(() => {});
    } catch {
      // The stream may already be closed.
    }
  };
  signal?.addEventListener('abort', onAbort, { once: true });

  // Last event id persists across frames, as the SSE spec requires.
  let lastId = '';
  let data = '';
  let hasData = false;
  let event = '';
  let eventBytes = 0;

  const resetFrame = () => {
    data = '';
    hasData = false;
    event = '';
    eventBytes = 0;
  };

  const tooLarge = () => new SSEError(
    `SSE event exceeds ${maxEventBytes} bytes.`,
    'event_too_large',
  );

  // Process one line; returns a frame when a blank line completes one.
  const takeLine = (raw) => {
    if (raw === '') {
      if (!hasData) {
        resetFrame();
        return undefined;
      }
      const value = data.endsWith('\n') ? data.slice(0, -1) : data;
      const frame = { id: lastId, event: event || 'message', data: value };
      resetFrame();
      return frame;
    }
    if (raw.charCodeAt(0) === 58 /* : */) {
      onComment?.(raw.slice(1).replace(/^ /, ''));
      return undefined;
    }
    const colon = raw.indexOf(':');
    const field = colon === -1 ? raw : raw.slice(0, colon);
    let value = colon === -1 ? '' : raw.slice(colon + 1);
    if (value.charCodeAt(0) === 32 /* space */) value = value.slice(1);

    switch (field) {
      case 'data':
        data += `${value}\n`;
        hasData = true;
        eventBytes += utf8ByteLength(value) + 1;
        break;
      case 'event':
        event = value;
        eventBytes += utf8ByteLength(value);
        break;
      case 'id':
        if (!value.includes('\0')) {
          lastId = value;
          eventBytes += utf8ByteLength(value);
        }
        break;
      default:
        // "retry" and unknown fields are ignored, as in the SSE spec.
        break;
    }
    if (eventBytes > maxEventBytes) throw tooLarge();
    return undefined;
  };

  // Pull one complete line out of the buffer. Returns null when more bytes are
  // needed, including a trailing CR that may turn out to be CRLF.
  const readLine = () => {
    let i = 0;
    while (i < buffer.length) {
      const c = buffer.charCodeAt(i);
      if (c === 10 /* \n */) {
        const line = buffer.slice(0, i);
        buffer = buffer.slice(i + 1);
        return line;
      }
      if (c === 13 /* \r */) {
        if (i + 1 < buffer.length) {
          const skip = buffer.charCodeAt(i + 1) === 10 ? 2 : 1;
          const line = buffer.slice(0, i);
          buffer = buffer.slice(i + skip);
          return line;
        }
        if (finalFlush) {
          const line = buffer.slice(0, i);
          buffer = '';
          return line;
        }
        return null;
      }
      i += 1;
    }
    return null;
  };

  try {
    for (;;) {
      let chunk;
      try {
        chunk = await reader.read();
      } catch (err) {
        if (aborted) throw aborted;
        if (signal?.aborted) throw abortReason(signal);
        throw err;
      }
      if (aborted) throw aborted;
      if (chunk.done) break;
      if (chunk.value !== undefined && chunk.value !== null) {
        buffer += decoder.decode(chunk.value, { stream: true });
      }
      for (;;) {
        const raw = readLine();
        if (raw === null) break;
        const frame = takeLine(raw);
        if (frame !== undefined) yield frame;
      }
      // Only an unterminated remainder is measured here: a large chunk made
      // of many complete lines is normal and must not trip the limit. UTF-8
      // never uses fewer bytes than JavaScript code units, so a remainder
      // longer than the limit is certainly oversized.
      if (buffer.length > maxEventBytes + 4096) throw tooLarge();
    }

    if (aborted) throw aborted;
    finalFlush = true;
    buffer += decoder.decode();
    for (;;) {
      const raw = readLine();
      if (raw === null) break;
      const frame = takeLine(raw);
      if (frame !== undefined) yield frame;
    }
    if (buffer !== '') {
      const raw = buffer;
      buffer = '';
      const frame = takeLine(raw);
      if (frame !== undefined) yield frame;
    }
    // A frame that ends the stream without a blank line still counts.
    const tail = takeLine('');
    if (tail !== undefined) yield tail;
    finished = true;
  } finally {
    signal?.removeEventListener('abort', onAbort);
    if (!finished) {
      try {
        await reader.cancel();
      } catch {
        // The stream may already be closed or errored.
      }
    }
    try {
      reader.releaseLock();
    } catch {
      // A reader released by cancel() is fine.
    }
  }
}
