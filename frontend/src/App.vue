<template>
  <v-app>
    <a href="#app-content" class="skip-link">{{ $t('menu.skipToContent') }}</a>
    <template v-if="isAuthPage">
      <v-main id="app-content" class="app-main auth-main" tabindex="-1">
        <router-view :key="$route.name" />
      </v-main>
    </template>
    <template v-else>
      <v-navigation-drawer
        v-if="$root.isLoaded"
        v-model="drawer"
        :rail="!isMobile && rail"
        :rail-width="72"
        :temporary="isMobile"
        :permanent="!isMobile"
        width="264"
        color="surface"
        border="end"
        class="app-drawer"
      >
        <div class="app-drawer-brand">
          <router-link :to="{ name: 'dashboard' }" class="app-brand-link">
            <img class="full" :src="logoSrc" :alt="$t('menu.dashboard')" />
            <img class="favicon" src="@/assets/favicon.png" :alt="$t('menu.dashboard')" />
          </router-link>
        </div>

        <navigation
          :active-item="activeItem"
          :opened-groups="openedGroups"
          :rail="!isMobile && rail"
          @updateOpenedGroups="updateOpenedGroups"
          @navigate="onNavigate"
        />

        <template v-if="!isMobile" #append>
          <div class="app-drawer-foot">
            <v-btn
              :icon="rail ? 'mdi-chevron-right' : 'mdi-chevron-left'"
              variant="text"
              size="small"
              :aria-label="rail ? $t('globals.buttons.expandNav') : $t('globals.buttons.collapseNav')"
              :title="rail ? $t('globals.buttons.expandNav') : $t('globals.buttons.collapseNav')"
              data-cy="nav-collapse"
              @click="toggleRail"
            />
          </div>
        </template>
      </v-navigation-drawer>

      <v-app-bar v-if="$root.isLoaded" flat color="surface" border="b" class="app-bar">
        <v-app-bar-nav-icon
          v-if="isMobile"
          :aria-label="$t('globals.buttons.openMenu')"
          @click="drawer = !drawer"
        />

        <v-toolbar-title class="app-title">
          <span>{{ pageTitle }}</span>
        </v-toolbar-title>

        <v-spacer />
        <v-btn icon="mdi-creation-outline" variant="text" aria-label="Open AI assistant" @click="assistantContext.open = !assistantContext.open" />

        <v-btn
          icon="mdi-refresh"
          variant="text"
          :aria-label="$t('globals.buttons.refresh')"
          @click="emitPageRefresh"
        />

        <div class="app-user-menu">
          <v-menu
            v-model="accountMenuOpen"
            location="bottom end"
          >
            <template #activator="{ props }">
              <v-btn
                v-bind="props"
                variant="text"
                class="app-user-trigger"
                :aria-label="$t('globals.buttons.accountMenu')"
              >
                <v-avatar size="32" color="primary" class="mr-2">
                  <img v-if="profile.avatar" :src="profile.avatar" :alt="profile.username" />
                  <span v-else>{{ userInitial }}</span>
                </v-avatar>
                <span class="app-user-name">{{ profile.username }}</span>
                <v-icon icon="mdi-chevron-down" size="18" class="ml-1" />
              </v-btn>
            </template>

            <v-list width="300">
              <v-list-item :title="profile.username" :subtitle="profile.name" />
              <v-divider />
              <v-list-item prepend-icon="mdi-account-outline" @click="goToUserProfile">
                <v-list-item-title>{{ $t('users.profile') }}</v-list-item-title>
              </v-list-item>
              <v-list-item :title="$t('users.theme')" prepend-icon="mdi-theme-light-dark">
                <template #append>
                  <v-btn-toggle
                    :model-value="themeChoice"
                    mandatory
                    density="compact"
                    variant="text"
                    divided
                    @update:model-value="setTheme"
                  >
                    <v-btn
                      v-for="opt in themeOptions"
                      :key="opt.value"
                      :value="opt.value"
                      :icon="opt.icon"
                      size="small"
                      :aria-label="opt.label"
                      :title="opt.label"
                    />
                  </v-btn-toggle>
                </template>
              </v-list-item>
              <v-list-item prepend-icon="mdi-logout" @click="doLogout">
                <v-list-item-title>{{ $t('users.logout') }}</v-list-item-title>
              </v-list-item>
            </v-list>
          </v-menu>
        </div>
      </v-app-bar>

      <v-main id="app-content" class="app-main" tabindex="-1">
        <div v-if="$root.isLoaded" class="app-shell">
          <div class="global-notices" v-if="isGlobalNotices">
            <v-alert
              v-if="serverConfig.needs_restart"
              type="error"
              variant="tonal"
              class="mb-4"
            >
              <div class="d-flex align-center justify-space-between ga-4 flex-wrap">
                <span>{{ $t('settings.needsRestart') }}</span>
                <v-btn color="error" variant="flat" size="small" @click="$utils.confirm($t('settings.confirmRestart'), reloadApp)">
                  {{ $t('settings.restart') }}
                </v-btn>
              </div>
            </v-alert>

            <template v-if="serverConfig.update">
              <v-alert
                v-if="serverConfig.update.update.is_new"
                type="success"
                variant="tonal"
                class="mb-4"
              >
                {{ $t('settings.updateAvailable', {
                  version: `${serverConfig.update.update.release_version} (${$utils.getDate(serverConfig.update.update.release_date).format('DD MMM YY')})`,
                }) }}
                <a :href="serverConfig.update.update.url" target="_blank" rel="noopener noreferrer">{{ $t('globals.messages.viewLink') }}</a>
              </v-alert>

              <v-alert
                v-for="m in serverConfig.update.messages"
                :key="m.title"
                :type="m.priority === 'high' ? 'error' : 'info'"
                variant="tonal"
                class="mb-4"
              >
                <div class="font-weight-bold" v-if="m.title">{{ m.title }}</div>
                <p v-if="m.description" class="mb-0">{{ m.description }}</p>
                <a v-if="m.url" :href="m.url" target="_blank" rel="noopener noreferrer">{{ $t('globals.messages.viewLink') }}</a>
              </v-alert>
            </template>

            <v-alert
              v-if="serverConfig.has_legacy_user"
              type="error"
              variant="tonal"
              class="mb-4"
            >
              Remove the <code>admin_username</code> and <code>admin_password</code> fields from the TOML configuration file or environment variables.
              Visit <router-link :to="{ name: 'users' }">Admin -> Settings -> Users</router-link>.
              <a :href="$docsUrl('upgrade/')" target="_blank" rel="noopener noreferrer">{{ $t('globals.buttons.learnMore') }}.</a>
            </v-alert>
          </div>

          <!-- Campaign initialization must rerun when moving from new to a saved ID. -->
          <router-view :key="$route.name === 'campaign' ? `${$route.name}:${$route.params.id}` : $route.name" />
        </div>

        <v-overlay
          :model-value="!$root.isLoaded"
          class="align-center justify-center"
          persistent
        >
          <div v-if="$root.initError" class="text-center pa-4">
            <p class="mb-4">{{ $root.initError }}</p>
            <v-btn color="primary" variant="flat" @click="retryInit">
              {{ $t('globals.buttons.retry') }}
            </v-btn>
          </div>
          <v-progress-circular v-else indeterminate size="56" width="5" color="primary" />
        </v-overlay>
      </v-main>
    </template>

    <v-overlay
      :model-value="$root.isRestarting"
      class="align-center justify-center"
      persistent
    >
      <div class="text-center" role="status" aria-live="polite">
        <v-progress-circular indeterminate size="56" width="5" color="primary" />
        <p class="mt-4 text-white">{{ $t('globals.messages.reloading') }}</p>
      </div>
    </v-overlay>

    <AIAssistant v-if="$root.isLoaded && !isAuthPage" />
    <confirm-dialog />

    <v-snackbar-queue
      v-model="toastQueue"
      location="bottom right"
      closable
      variant="elevated"
    />
  </v-app>
</template>

<script>
import { computed, ref } from 'vue';
import { mapState } from 'vuex';
import { useDisplay, useTheme } from 'vuetify';
import { saveThemeChoice, storedThemeChoice } from './plugins/vuetify';
import logoLight from './assets/logo.svg';
import logoDark from './assets/logo-dark.svg';
import { uris } from './constants';
import { pb } from './api';
import AIAssistant from './components/AIAssistant.vue';
import { assistantContext } from './assistant/context';
import Navigation from './components/Navigation.vue';
import ConfirmDialog from './components/ConfirmDialog.vue';
import { toastQueue } from './utils/toast';
import { events } from './utils/events';

const RAIL_KEY = 'listpocket_nav_collapsed';

function savedRail() {
  try {
    return localStorage.getItem(RAIL_KEY) === '1';
  } catch {
    return false;
  }
}

export default {
  name: 'App',

  components: {
    Navigation,
    AIAssistant,
    ConfirmDialog,
  },

  setup() {
    const { width } = useDisplay();
    const isMobile = computed(() => width.value <= 960);

    const theme = useTheme();
    const themeChoice = ref(storedThemeChoice());
    const setTheme = (choice) => {
      themeChoice.value = choice;
      theme.change(choice);
      saveThemeChoice(choice);
    };

    const logoSrc = computed(() => (theme.current.value.dark ? logoDark : logoLight));

    return {
      isMobile, themeChoice, setTheme, logoSrc,
    };
  },

  data() {
    return {
      activeItem: {},
      openedGroups: [],
      accountMenuOpen: false,
      drawer: true,
      rail: savedRail(),
      railBeforeAI: undefined,
      eventsTopic: 'events/error',
      toastQueue,
    };
  },

  computed: {
    assistantContext() { return assistantContext; },
    ...mapState(['serverConfig', 'profile']),

    themeOptions() {
      return [
        { value: 'light', icon: 'mdi-white-balance-sunny', label: this.$t('users.themeLight') },
        { value: 'dark', icon: 'mdi-weather-night', label: this.$t('users.themeDark') },
        { value: 'system', icon: 'mdi-theme-light-dark', label: this.$t('users.themeSystem') },
      ];
    },

    isGlobalNotices() {
      return (this.serverConfig.needs_restart
        || this.serverConfig.has_legacy_user
        || (this.serverConfig.update
          && this.serverConfig.update.messages
          && this.serverConfig.update.messages.length > 0));
    },

    isAuthPage() {
      return Boolean(this.$route.meta && this.$route.meta.authPage);
    },

    pageTitle() {
      if (this.isAuthPage) {
        const authTitle = this.$route.meta && this.$route.meta.title;
        return typeof authTitle === 'string' ? this.$t(authTitle) : this.$t('menu.dashboard');
      }
      if (this.$route.name === 'dashboard') {
        return this.$t('menu.overview');
      }
      const titleKey = this.$route.meta && this.$route.meta.title;
      if (typeof titleKey === 'string' && titleKey) {
        return this.$t(titleKey);
      }
      return this.$t('menu.dashboard');
    },

    userInitial() {
      return this.profile && this.profile.username ? this.profile.username[0].toUpperCase() : '?';
    },
  },

  watch: {
    $route: {
      immediate: true,
      handler(to) {
        const activeName = to.name === 'campaign' && to.params.id !== 'new'
          ? 'campaigns' : (to.meta.nav || to.name);
        this.activeItem = { [activeName]: true };
        if (to.meta.group) {
          this.openedGroups = [to.meta.group];
        } else {
          this.openedGroups = [];
        }
        if (this.isMobile) {
          this.drawer = false;
        }
      },
    },
    isMobile(mobile) {
      if (!mobile) {
        this.drawer = true;
      }
    },
  },

  methods: {
    toggleRail() {
      this.rail = !this.rail;
      // Remembered unless the assistant collapsed it for now.
      this.railBeforeAI = undefined;
      try {
        localStorage.setItem(RAIL_KEY, this.rail ? '1' : '0');
      } catch {
        // It just won't be remembered.
      }
    },

    updateOpenedGroups(groups) {
      this.openedGroups = groups;
    },

    onNavigate() {
      if (this.isMobile) {
        this.drawer = false;
      }
    },

    emitPageRefresh() {
      events.$emit('page.refresh');
    },

    onAIPanel(open) {
      if (open) {
        if (this.railBeforeAI === undefined) {
          this.railBeforeAI = this.rail;
        }
        this.rail = true;
        return;
      }
      if (this.railBeforeAI !== undefined) {
        this.rail = this.railBeforeAI;
        this.railBeforeAI = undefined;
      }
    },

    reloadApp() {
      this.$api.reloadApp().then(async () => {
        this.$root.isRestarting = true;
        // Give the server a moment to go down before polling for it.
        await new Promise((r) => { setTimeout(r, 500); });
        if (await this.$api.waitForHealth({ intervalMs: 500 })) {
          document.location.reload();
          return;
        }
        this.$root.isRestarting = false;
        this.$utils.toast(this.$t('globals.messages.restartTimeout'), 'error', -1);
      });
    },

    retryInit() {
      if (this.$root && typeof this.$root.retryInit === 'function') {
        this.$root.retryInit();
      }
    },

    doLogout() {
      this.accountMenuOpen = false;
      this.$api.logout().then(() => {
        document.location.href = uris.root;
      });
    },

    goToUserProfile() {
      this.accountMenuOpen = false;
      this.$router.push({ name: 'userProfile' });
    },

    async listenEvents() {
      const reMatchLog = /(.+?)\.go:\d+:(.+?)$/im;
      let numEv = 0;
      try {
        await pb.realtime.subscribe(this.eventsTopic, (e) => {
          if (numEv > 50) {
            return;
          }
          numEv += 1;

          const d = e && e.data ? e.data : e;
          if (d && d.type === 'error' && d.message) {
            const msg = reMatchLog.exec(d.message.trim());
            this.$utils.toast(msg ? msg[2] : d.message.trim(), 'error', null, true);
          }
        });
      } catch (err) {
        const msg = err?.response?.message || err?.message || this.$t('globals.messages.realtimeFailed');
        this.$utils.toast(msg, 'error', 5000, false);
      }
    },
  },

  mounted() {
    events.$on('layout.aiPanel', this.onAIPanel);
    // Signed-out visitors (the login and password pages) can't load lists or
    // subscribe to events; login does a full page load, so this re-runs after.
    if (this.$api.isAuthenticated()) {
      this.$api.getLists({ minimal: true, per_page: 'all', status: 'active' });
      this.listenEvents();
    }
  },

  beforeUnmount() {
    pb.realtime.unsubscribe(this.eventsTopic);
    events.$off('layout.aiPanel', this.onAIPanel);
  },
};
</script>

<style lang="scss">
@use "assets/style.scss";
@use "assets/layout.scss";

.skip-link {
  position: absolute;
  left: -999px;
  top: 12px;
  z-index: 4000;
  padding: 8px 14px;
  background: rgb(var(--v-theme-primary));
  color: rgb(var(--v-theme-on-primary));
  border-radius: 8px;
}

.skip-link:focus {
  left: 16px;
}

.app-drawer .v-navigation-drawer__content {
  padding: 16px 12px;
}

.app-drawer.v-navigation-drawer--rail .v-navigation-drawer__content {
  padding: 12px 6px;
}

.app-drawer-brand {
  padding: 8px 8px 16px;
}

.app-drawer-foot {
  border-top: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  display: flex;
  justify-content: flex-end;
  padding: 4px 8px;
}

.app-drawer.v-navigation-drawer--rail .app-drawer-foot {
  justify-content: center;
}

.app-brand-link {
  display: inline-flex;
  align-items: center;
}

.app-brand-link img.full {
  height: 26px;
}

.app-brand-link img.favicon {
  display: none;
  height: 28px;
}

.app-drawer.v-navigation-drawer--rail .app-drawer-brand {
  display: flex;
  justify-content: center;
  padding: 8px 0 20px;
}

.app-drawer.v-navigation-drawer--rail .app-brand-link img.full {
  display: none;
}

.app-drawer.v-navigation-drawer--rail .app-brand-link img.favicon {
  display: block;
}

.app-bar {
  backdrop-filter: blur(14px);
}

.app-title {
  font-weight: 600;
  color: rgb(var(--v-theme-on-surface));
}

.app-user-trigger {
  text-transform: none;
}

.app-user-menu {
  position: relative;
}

.app-user-name {
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.app-main {
  background: linear-gradient(180deg, rgb(var(--v-theme-surface-light)) 0%, rgb(var(--v-theme-surface-light)) 100%);
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.app-main > .app-shell {
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  min-height: 0;
  min-width: 0;
}

.app-shell {
  padding: 24px;
  width: 100%;
}

.app-shell > :is(section, form, main) {
  min-width: 0;
  width: 100%;
}

/* Workflow builder root is <main>; let it fill the shell so inner grid gets height. */
.app-shell > main.workflow-route {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

@media (max-width: 960px) {
  .app-shell {
    padding: 16px;
  }
}

@media (max-width: 600px) {
  .app-shell {
    padding: 16px 12px 24px;
  }

  .app-user-name {
    display: none;
  }
}
</style>
