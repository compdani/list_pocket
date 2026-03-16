import vue from '@vitejs/plugin-vue2';
import { defineConfig, loadEnv } from 'vite';

const path = require('path');

// https://vitejs.dev/config/
export default defineConfig(({ _, mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  return {
    plugins: [vue()],
    base: '/admin',
    mode,
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
        bulma: require.resolve('bulma/bulma.sass'),
      },
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
