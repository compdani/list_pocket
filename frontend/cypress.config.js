const { defineConfig } = require('cypress');

module.exports = defineConfig({
  env: {
    apiUrl: 'http://localhost:9000',
    serverInitCmd:
      'pkill -9 listpocket | cd ../ && LISTPOCKET_ADMIN_USER=admin LISTPOCKET_ADMIN_PASSWORD=listpocket ./listpocket --install --yes && ./listpocket > /dev/null 2>/dev/null &',
    serverInitBlankCmd:
      'pkill -9 listpocket | cd ../ && ./listpocket --install --yes && ./listpocket > /dev/null 2>/dev/null &',
    LISTPOCKET_ADMIN_USER: 'admin',
    LISTPOCKET_ADMIN_PASSWORD: 'listpocket',
  },
  viewportWidth: 1400,
  viewportHeight: 950,
  e2e: {
    experimentalRunAllSpecs: true,
    testIsolation: false,
    experimentalSessionAndOrigin: false,
    // We've imported your old cypress plugins here.
    // You may want to clean this up later by importing these.
    setupNodeEvents(on, config) {
      return require('./cypress/plugins/index.js')(on, config);
    },
    baseUrl: 'http://localhost:9000',
  },
});
