// RFC 2047 encoded-words can arrive unchanged from older inbox records.
export function decodeEmailSubject(value) {
  const subject = String(value || '');
  return subject.replace(/(\?=)\s+(=\?)/g, '$1$2').replace(/=\?([^?]+)\?([bq])\?([^?]*)\?=/gi, (word, charset, encoding, content) => {
    try {
      const raw = encoding.toLowerCase() === 'b'
        ? atob(content)
        : content.replace(/_/g, ' ').replace(/=([\da-f]{2})/gi, (_, hex) => String.fromCharCode(parseInt(hex, 16)));
      return new TextDecoder(charset, { fatal: true }).decode(Uint8Array.from(raw, (c) => c.charCodeAt(0)));
    } catch {
      return word;
    }
  });
}
