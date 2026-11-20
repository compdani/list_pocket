// Types for sse.mjs (@pocketbase-mcp/chat-client/sse).

/** One parsed SSE frame. `event` is "message" when the stream set no type. */
export interface SSEFrame {
  id: string;
  event: string;
  data: string;
}

export interface ParseSSEOptions {
  /** Aborts the read; the iteration rejects with the signal's reason. */
  signal?: AbortSignal;
  /** Largest single frame in UTF-8 bytes; defaults to 1 MiB. */
  maxEventBytes?: number;
  /** Called for every comment line (text after ":", one leading space removed). */
  onComment?: (comment: string) => void;
}

/** An SSE framing or size error. */
export declare class SSEError extends Error {
  code: string;
  constructor(message: string, code?: string);
}

/** UTF-8 bytes a string would occupy. */
export declare function utf8ByteLength(value: string): number;

/** Parse a byte stream as Server-Sent Events. */
export declare function parseSSE(
  stream: ReadableStream<Uint8Array>,
  options?: ParseSSEOptions,
): AsyncGenerator<SSEFrame, void, unknown>;
