// The model list for the panel's built-in model menu (agent-panel 0.3.0), from
// `agent/models`: one entry per provider model, grouped by provider. A provider that
// failed to list its models shows its error as a disabled entry.
import { computed, ref } from 'vue';
import { agentGet } from '../agent/client';

// Menu ids are "provider/model"; model ids may contain "/" themselves.
export const modelKey = (ai) => (ai?.model ? `${ai.provider}/${ai.model}` : '');

export function modelFromKey(key) {
  if (!key) return null;
  const at = key.indexOf('/');
  return { provider: key.slice(0, at), model: key.slice(at + 1) };
}

export function useAgentModels() {
  const data = ref(null);
  const load = async () => {
    try {
      data.value = await agentGet('agent/models');
    } catch (err) {
      data.value = { providers: [], error: err.message };
    }
  };
  load();
  const models = computed(() => (data.value?.providers || []).flatMap((p) => [
    ...(p.models || []).map((m) => ({
      id: `${p.name}/${m.id}`, label: m.id, description: m.name && m.name !== m.id ? m.name : '', group: p.label || p.name,
    })),
    ...(p.error ? [{ id: `${p.name}/`, label: p.error, group: p.label || p.name, disabled: true }] : []),
  ]));
  // Names the server's default in the menu's Default entry and the trigger.
  const defaultModel = computed(() => data.value?.default?.model || '');
  return { models, defaultModel, load };
}
