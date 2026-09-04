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
        :expand-on-hover="!isMobile && rail"
        :temporary="isMobile"
        :permanent="!isMobile"
        width="280"
        color="surface"
        border="end"
        class="app-drawer"
      >
        <div class="app-drawer-brand">
          <router-link :to="{ name: 'dashboard' }" class="app-brand-link">
            <img class="full" src="@/assets/logo.svg" :alt="$t('menu.dashboard')" />
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
      </v-navigation-drawer>

      <v-app-bar v-if="$root.isLoaded" flat color="surface" border="b" class="app-bar">
        <v-app-bar-nav-icon
          v-if="isMobile"
          :aria-label="$t('globals.buttons.openMenu')"
          @click="drawer = !drawer"
        />
        <v-btn
          v-else
          icon="mdi-dock-left"
          variant="text"
          :aria-label="rail ? $t('globals.buttons.expandNav') : $t('globals.buttons.collapseNav')"
          @click="rail = !rail"
        />

        <v-toolbar-title class="app-title">
          <span>{{ pageTitle }}</span>
        </v-toolbar-title>

        <v-spacer />

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

            <v-list width="240">
              <v-list-item :title="profile.username" :subtitle="profile.name" />
              <v-divider />
              <v-list-item prepend-icon="mdi-account-outline" @click="goToUserProfile">
                <v-list-item-title>{{ $t('users.profile') }}</v-list-item-title>
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

          <router-view :key="$route.name" />
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
import { computed } from 'vue';
import { mapState } from 'vuex';
import { useDisplay } from 'vuetify';
import { uris } from './constants';
import { pb } from './api';
import Navigation from './components/Navigation.vue';
import ConfirmDialog from './components/ConfirmDialog.vue';
import { toastQueue } from './utils/toast';
import { events } from './utils/events';

export default {
  name: 'App',

  components: {
    Navigation,
    ConfirmDialog,
  },

  setup() {
    const { width } = useDisplay();
    const isMobile = computed(() => width.value <= 960);
    return { isMobile };
  },

  data() {
    return {
      activeItem: {},
      openedGroups: [],
      accountMenuOpen: false,
      drawer: true,
      rail: false,
      railBeforeAI: undefined,
      eventsTopic: 'events/error',
      toastQueue,
    };
  },

  computed: {
    ...mapState(['serverConfig', 'profile']),

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
        this.activeItem = { [to.name]: true };
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
      this.$api.reloadApp().then(() => {
        this.$utils.toast(this.$t('globals.messages.reloading'));
        const pollId = setInterval(() => {
          this.$api.getHealth().then(() => {
            clearInterval(pollId);
            document.location.reload();
          });
        }, 500);
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
    this.$api.getLists({ minimal: true, per_page: 'all', status: 'active' });
    events.$on('layout.aiPanel', this.onAIPanel);
    this.listenEvents();
  },

  beforeUnmount() {
    pb.realtime.unsubscribe(this.eventsTopic);
    events.$off('layout.aiPanel', this.onAIPanel);
  },
};
</script>

<style lang="scss">
@use "assets/style.scss";

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
  padding: 8px 8px 20px;
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
  background: linear-gradient(180deg, #f6f7fb 0%, #eef2f9 100%);
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.app-main > .app-shell {
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  min-height: 0;
}

.app-shell {
  padding: 24px;
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
</style>
