import vue from '@vitejs/plugin-vue';
import { defineConfig, loadEnv } from 'vite';
import vuetify from 'vite-plugin-vuetify';

const path = require('path');

// https://vitejs.dev/config/
export default defineConfig(({ _, mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const apiTarget = env.LISTPOCKET_API_URL || 'http://127.0.0.1:9000';
  return {
    plugins: [
      // The agent panel is a framework-free custom element (the agent-panel package).
      vue({ template: { compilerOptions: { isCustomElement: (tag) => ['agent-panel', 'agent-page', 'lp-agent-panel', 'lp-agent-page'].includes(tag) } } }),
      vuetify({ autoImport: true, styles: { configFile: 'src/styles/settings.scss' } }),
    ],
    base: '/admin/',
    mode,
    resolve: {
      alias: [
        // Vendored from the pocketbase-mcp module (see src/vendor/chat-client/VENDORED.md).
        // Most specific first, so the subpaths don't fall through to index.mjs.
        { find: /^@pocketbase-mcp\/chat-client\/agent-panel$/, replacement: path.resolve(__dirname, './src/vendor/chat-client/agent-panel.mjs') },
        { find: /^@pocketbase-mcp\/chat-client\/controller$/, replacement: path.resolve(__dirname, './src/vendor/chat-client/controller.mjs') },
        { find: /^@pocketbase-mcp\/chat-client\/client$/, replacement: path.resolve(__dirname, './src/vendor/chat-client/client.mjs') },
        { find: /^@pocketbase-mcp\/chat-client\/sse$/, replacement: path.resolve(__dirname, './src/vendor/chat-client/sse.mjs') },
        { find: /^@pocketbase-mcp\/chat-client\/reducer$/, replacement: path.resolve(__dirname, './src/vendor/chat-client/reducer.mjs') },
        { find: /^@pocketbase-mcp\/chat-client$/, replacement: path.resolve(__dirname, './src/vendor/chat-client/index.mjs') },
        { find: '@', replacement: path.resolve(__dirname, './src') },
      ],
    },
    define: {
      __VUE_OPTIONS_API__: true,
      __VUE_PROD_DEVTOOLS__: false,
      __VUE_PROD_HYDRATION_MISMATCH_DETAILS__: false,
    },
    build: {
      assetsDir: 'static',
      rollupOptions: {
        output: {
          manualChunks(id) {
            if (!id.includes('node_modules')) {
              return undefined;
            }
            // TinyMCE plugins share circular ESM exports; a forced vendor chunk
            // rewrites those as Export 'anchor_exports' is not defined.
            if (id.includes('tinymce')) {
              return undefined;
            }
            if (id.includes('grapesjs')) {
              return 'grapesjs';
            }
            if (id.includes('@codemirror') || id.includes('/codemirror/')) {
              return 'codemirror';
            }
            // Keep the Vue runtime in its own chunk; otherwise it gets merged
            // into vue-flow and that whole chunk is preloaded on every page.
            if (/node_modules\/(@vue\/|vue\/|vue-router|vuex|vue-i18n|@intlify)/.test(id)) {
              return 'vue';
            }
            if (id.includes('@vue-flow')) {
              return 'vue-flow';
            }
            if (id.includes('chart.js') || id.includes('vue-chartjs')) {
              return 'charts';
            }
            if (id.includes('vuetify')) {
              return 'vuetify';
            }
            return undefined;
          },
        },
      },
    },
    server: {
      port: env.LISTPOCKET_FRONTEND_PORT || 8080,
      proxy: {
        '^/$': {
          target: apiTarget,
        },
        '^/(api|mailapi|webhooks|subscription|public|health)': {
          target: apiTarget,
        },
        '^/admin/setup': {
          target: apiTarget,
        },
        '^/admin/plugin-assets': {
          target: apiTarget,
        },
        '^/(admin\/custom\.(css|js))': {
          target: apiTarget,
        },
        // Built-in docs site + OpenAPI (same origin as API in production)
        '^/docs': {
          target: apiTarget,
        },
        '^/openapi.yaml': {
          target: apiTarget,
        },
        '^/swagger': {
          target: apiTarget,
        },
      },
    },
  };
});
