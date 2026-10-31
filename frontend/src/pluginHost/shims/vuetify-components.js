// Built separately by scripts/build-plugin-shims.mjs and loaded only when a UI
// plugin is enabled, so the main bundle keeps its tree-shaking.
import * as components from 'vuetify/components';
import * as directives from 'vuetify/directives';

export { components, directives };
export * from 'vuetify/components';
