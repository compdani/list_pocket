import vue from '@vitejs/plugin-vue';
import { defineConfig, loadEnv } from 'vite';
import vuetify from 'vite-plugin-vuetify';

const fs = require('fs');
const path = require('path');

const resolveVueFlowAlias = (pkg) => {
  const workflowPath = path.resolve(__dirname, '../../workflow/frontend/node_modules/@vue-flow', pkg);
  if (fs.existsSync(workflowPath)) {
    return workflowPath;
  }

  return path.resolve(__dirname, './node_modules/@vue-flow', pkg);
};

// https://vitejs.dev/config/
export default defineConfig(({ _, mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const apiTarget = env.LISTPOCKET_API_URL || 'http://127.0.0.1:9000';
  return {
    plugins: [
      vue(),
      vuetify({ autoImport: true }),
    ],
    base: '/admin/',
    mode,
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
        '@vue-flow/background': resolveVueFlowAlias('background'),
        '@vue-flow/controls': resolveVueFlowAlias('controls'),
        '@vue-flow/core': resolveVueFlowAlias('core'),
        '@vue-flow/minimap': resolveVueFlowAlias('minimap'),
      },
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
            if (id.includes('tinymce')) {
              return 'tinymce';
            }
            if (id.includes('grapesjs')) {
              return 'grapesjs';
            }
            if (id.includes('@codemirror') || id.includes('/codemirror/')) {
              return 'codemirror';
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
        '^/(admin\/custom\.(css|js))': {
          target: apiTarget,
        },
        // Built-in MkDocs + OpenAPI (same origin as API in production)
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
