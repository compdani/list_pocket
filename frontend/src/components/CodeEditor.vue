<template>
  <div ref="editor" class="code-editor" />
</template>

<script>
import { EditorState, Prec } from '@codemirror/state';
import {
  EditorView, keymap, highlightActiveLine, lineNumbers, highlightActiveLineGutter, tooltips,
} from '@codemirror/view';
import { acceptCompletion, autocompletion } from '@codemirror/autocomplete';
import { markdown } from '@codemirror/lang-markdown';
import { javascript, localCompletionSource } from '@codemirror/lang-javascript';
import { json } from '@codemirror/lang-json';
import { css } from '@codemirror/lang-css';
import { html } from '@codemirror/lang-html';
import {
  defaultKeymap, history, historyKeymap, indentWithTab,
} from '@codemirror/commands';
import { defaultHighlightStyle, syntaxHighlighting, bracketMatching } from '@codemirror/language';
import { search, searchKeymap, highlightSelectionMatches } from '@codemirror/search';
import { vsCodeLight } from './editor-theme';
import { codeHintPath } from '../utils/codeHintPath.mjs';

export default {
  emits: ['update:modelValue', 'blur'],

  props: {
    modelValue: { type: String, default: '' },
    lang: { type: String, default: 'html' },
    disabled: Boolean,
    autofocus: { type: Boolean, default: true },
    // For JavaScript: takes a property path ([] for globals) and returns CodeMirror completions.
    completions: { type: Function, default: null },
  },

  data() {
    return {
      data: '',
      editor: null,
    };
  },

  methods: {
    getAssistantSelection() {
      const state = this.editor?.state;
      if (!state) return '';
      const { from, to } = state.selection.main;
      return state.doc.sliceString(from, to);
    },
  },

  mounted() {
    const onUpdate = EditorView.updateListener.of((update) => {
      if (update.docChanged) {
        this.$emit('update:modelValue', update.state.doc.toString());
      }
    });
    const onBlur = EditorView.domEventHandlers({
      blur: () => { this.$emit('blur'); },
    });

    // Set the chosen language.
    let langs = [];
    switch (this.lang) {
      case 'html':
        langs = [html()];
        break;
      case 'css':
        langs = [css()];
        break;
      case 'javascript':
        langs = [javascript()];
        break;
      case 'json':
        langs = [json()];
        break;
      case 'markdown':
        langs = [markdown()];
        break;
      default:
        langs = [html()];
    }

    const hints = [];
    if (this.completions && this.lang === 'javascript') {
      const source = (context) => {
        const at = codeHintPath(context);
        const options = at ? this.completions(at.path) : [];
        if (!options.length) return null;
        return { from: context.pos - at.name.length, options, validFor: /^[\w$]*$/ };
      };
      hints.push(
        // Replaces the language's own list, which has things such as import that may not apply.
        autocompletion({ override: [source, localCompletionSource] }),
        // Inside a panel the popup would be clipped, so it hangs off the page instead.
        tooltips({ parent: document.body }),
        Prec.high(keymap.of([{ key: 'Tab', run: acceptCompletion }])),
      );
    }

    // Prepare the full config.
    const stateCfg = EditorState.create({
      // Initial value.
      doc: this.modelValue,

      extensions: [
        EditorView.baseTheme({}),
        ...langs,
        ...hints,
        history(),
        highlightActiveLine(),
        bracketMatching(),
        highlightSelectionMatches(),
        lineNumbers(),
        highlightActiveLineGutter(),
        keymap.of([...defaultKeymap, ...historyKeymap, ...searchKeymap, indentWithTab]),

        // Readonly?
        EditorState.readOnly.of(this.disabled),
        EditorView.editable.of(!this.disabled),

        // Syntax highlighting and theme.
        syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
        EditorView.lineWrapping,

        vsCodeLight,

        search({
          top: true, // Places the search panel at the top of the editor
        }),

        // On content change.
        onUpdate,
        onBlur,
      ],
    });

    // Create the editor.
    this.editor = new EditorView({
      state: stateCfg,
      parent: this.$refs.editor,
    });

    if (this.autofocus) {
      this.$nextTick(() => {
        window.setTimeout(() => {
          this.editor.focus();
        }, 100);
      });
    }
  },

  beforeUnmount() {
    if (this.editor) {
      this.editor.destroy();
    }
  },

  watch: {
    modelValue(val) {
      if (!this.editor) {
        return;
      }

      // Typing comes back here as the same text; only replace the document for outside changes.
      const next = val ?? '';
      if (next === this.editor.state.doc.toString()) {
        return;
      }

      this.editor.dispatch({
        changes: { from: 0, to: this.editor.state.doc.length, insert: next },
      });
    },
  },
};
</script>
