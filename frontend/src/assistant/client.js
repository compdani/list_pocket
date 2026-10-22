import { pb } from '../api';
export async function assistantRequest(path, body, signal) {
  const response = await fetch(`${pb.baseURL}/mailapi/ai/assistant/${path}`, {
    method: body === undefined ? 'GET' : 'POST', signal,
    headers: { Authorization: `Bearer ${pb.authStore.token}`, 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  if (!response.ok) { const error = await response.json().catch(() => ({})); throw new Error(error.message || `Assistant request failed (${response.status})`); }
  return response;
}
export async function assistantJSON(path, body) {
  const response = await assistantRequest(path, body);
  const result = await response.json();
  return result.data ?? result;
}
export async function streamAssistant(id, after, onEvent, signal) {
  const response = await assistantRequest(`turns/${id}/stream?after=${after}`, undefined, signal);
  const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = '';
  try {
    while (true) {
      const { value, done } = await reader.read();
      buffer += decoder.decode(value || new Uint8Array(), { stream: !done }).replace(/\r\n/g, '\n');
      let boundary = buffer.indexOf('\n\n');
      while (boundary >= 0) {
        const frame = buffer.slice(0, boundary); buffer = buffer.slice(boundary + 2);
        const data = frame.split('\n').filter((line) => line.startsWith('data:')).map((line) => line.slice(5).trimStart()).join('\n');
        if (data) onEvent(JSON.parse(data));
        boundary = buffer.indexOf('\n\n');
      }
      if (done) break;
    }
  } finally { reader.releaseLock(); }
}
