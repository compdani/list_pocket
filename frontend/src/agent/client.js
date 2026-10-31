import { pb } from '../api';

// Agent requests return their errors to the page, which shows them beside the composer.
export async function agentRequest(method, path, body) {
  const response = await fetch(`${pb.baseURL}/mailapi/${path}`, {
    method,
    headers: { Authorization: `Bearer ${pb.authStore.token}`, 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  const text = await response.text();
  let parsed = {};
  try {
    parsed = text ? JSON.parse(text) : {};
  } catch {
    parsed = {};
  }
  if (!response.ok) {
    throw new Error(parsed.message || `Request failed (${response.status})`);
  }
  return parsed.data ?? parsed;
}

export const agentGet = (path) => agentRequest('GET', path);
export const agentPost = (path, body = {}) => agentRequest('POST', path, body);
export const agentPatch = (path, body) => agentRequest('PATCH', path, body);
export const agentPut = (path, body) => agentRequest('PUT', path, body);
export const agentDelete = (path) => agentRequest('DELETE', path);
