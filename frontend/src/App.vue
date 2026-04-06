<template>
  <v-app>
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
          <img class="full" src="@/assets/logo.svg" alt="" />
          <img class="favicon" src="@/assets/favicon.png" alt="" />
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
      <v-app-bar-nav-icon v-if="isMobile" @click="drawer = !drawer" />
      <v-btn v-else icon="mdi-dock-left" variant="text" @click="rail = !rail" />

      <v-toolbar-title class="app-title">
        <span>{{ pageTitle }}</span>
      </v-toolbar-title>

      <v-spacer />

      <v-btn
        icon="mdi-refresh"
        variant="text"
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
            >
              <v-avatar size="32" color="primary" class="mr-2">
                <img v-if="profile.avatar" :src="profile.avatar" alt="" />
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

    <v-main class="app-main">
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
              <a :href="serverConfig.update.update.url" target="_blank" rel="noopener noreferer">View</a>
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
              <a v-if="m.url" :href="m.url" target="_blank" rel="noopener noreferer">View</a>
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
            <a :href="$docsUrl('upgrade/')" target="_blank" rel="noopener noreferrer">Learn more.</a>
          </v-alert>
        </div>

        <router-view :key="$route.fullPath" />
      </div>

      <v-overlay
        :model-value="!$root.isLoaded"
        class="align-center justify-center"
        persistent
      >
        <v-progress-circular indeterminate size="56" width="5" color="primary" />
      </v-overlay>
    </v-main>
  </v-app>
</template>

<script>
import { mapState } from 'vuex';
import { uris } from './constants';
import { pb } from './api';
import Navigation from './components/Navigation.vue';

export default {
  name: 'App',

  components: {
    Navigation,
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
      windowWidth: window.innerWidth,
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

    isMobile() {
      return this.windowWidth <= 960;
    },

    pageTitle() {
      if (this.$route.name === 'dashboard') {
        return 'Overview';
      }
      if (this.$route.meta && typeof this.$route.meta.title === 'string' && this.$route.meta.title.startsWith('Workflow')) {
        return this.$route.meta.title;
      }
      if (this.$route.name === 'workflows') {
        return 'Workflows';
      }
      return typeof this.$route.meta.title === 'string' && this.$route.meta.title
        ? this.$t(this.$route.meta.title)
        : 'Admin';
    },

    userInitial() {
      return this.profile.username ? this.profile.username[0].toUpperCase() : '?';
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
      this.$events.$emit('page.refresh');
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
        this.$utils.toast('Reloading app ...');
        const pollId = setInterval(() => {
          this.$api.getHealth().then(() => {
            clearInterval(pollId);
            document.location.reload();
          });
        }, 500);
      });
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
            this.$utils.toast(msg ? msg[2] : d.message.trim(), 'is-danger', null, true);
          }
        });
      } catch (err) {
        const msg = err?.response?.message || err?.message || 'Realtime connection failed';
        this.$utils.toast(msg, 'is-danger', 5000, false);
      }
    },
  },

  mounted() {
    this.$api.getLists({ minimal: true, per_page: 'all', status: 'active' });
    window.addEventListener('resize', this.onResize);
    this.$events.$on('layout.aiPanel', this.onAIPanel);
    this.listenEvents();
  },

  beforeUnmount() {
    pb.realtime.unsubscribe(this.eventsTopic);
    window.removeEventListener('resize', this.onResize);
    this.$events.$off('layout.aiPanel', this.onAIPanel);
  },

  created() {
    this.onResize = () => {
      this.windowWidth = window.innerWidth;
      if (!this.isMobile) {
        this.drawer = true;
      }
    };
  },
};
</script>

<style lang="scss">
@use "assets/style.scss";
@import "assets/icons/fontello.css";

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
  color: #0f172a;
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
}

.app-shell {
  padding: 24px;
}

@media (max-width: 960px) {
  .app-shell {
    padding: 16px;
  }
}
</style>
