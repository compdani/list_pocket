import vue from '@vitejs/plugin-vue';
import { defineConfig, loadEnv } from 'vite';
import vuetify from 'vite-plugin-vuetify';

const path = require('path');

// https://vitejs.dev/config/
export default defineConfig(({ _, mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  return {
    plugins: [
      vue(),
      vuetify({ autoImport: true }),
    ],
    base: '/admin',
    mode,
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
        '@vue-flow/background': path.resolve(__dirname, '../../frontend/node_modules/@vue-flow/background'),
        '@vue-flow/controls': path.resolve(__dirname, '../../frontend/node_modules/@vue-flow/controls'),
        '@vue-flow/core': path.resolve(__dirname, '../../frontend/node_modules/@vue-flow/core'),
        '@vue-flow/minimap': path.resolve(__dirname, '../../frontend/node_modules/@vue-flow/minimap'),
        bulma: require.resolve('bulma/bulma.sass'),
      },
    },
    define: {
      __VUE_OPTIONS_API__: true,
      __VUE_PROD_DEVTOOLS__: false,
      __VUE_PROD_HYDRATION_MISMATCH_DETAILS__: false,
    },
    build: {
      assetsDir: 'static',
    },
    server: {
      port: env.LISTPOCKET_FRONTEND_PORT || 8080,
      proxy: {
        '^/$': {
          target: env.LISTPOCKET_API_URL || 'http://127.0.0.1:9000',
        },
        '^/(api|webhooks|subscription|public|health)': {
          target: env.LISTPOCKET_API_URL || 'http://127.0.0.1:9000',
        },
        '^/admin/login': {
          target: env.LISTPOCKET_API_URL || 'http://127.0.0.1:9000',
        },
        '^/(admin\/custom\.(css|js))': {
          target: env.LISTPOCKET_API_URL || 'http://127.0.0.1:9000',
        },
      },
    },
  };
});
