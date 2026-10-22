import { shallowReactive } from 'vue';
export const assistantContext = shallowReactive({ open: false, editor: null });
export function registerAssistantEditor(editor) {
  assistantContext.editor = editor;
  return () => { if (assistantContext.editor === editor) assistantContext.editor = null; };
}
