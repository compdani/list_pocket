<template>
  <div :class="['side-nav', { 'side-nav--rail': rail }]" data-cy="side-nav">
    <div v-if="!rail" class="side-nav-search">
      <v-text-field
        v-model="filter"
        :placeholder="$t('menu.jumpTo')"
        :aria-label="$t('menu.jumpTo')"
        prepend-inner-icon="mdi-magnify"
        density="compact"
        variant="solo-filled"
        flat
        hide-details
        clearable
        data-cy="nav-filter"
        @keydown.enter="openFirstMatch"
        @keydown.esc="filter = ''"
      />
    </div>

    <!-- Searching shows a flat list, so a match inside a closed group is still reachable. -->
    <v-list v-if="filter" nav density="compact" class="side-nav-list" data-cy="nav-results">
      <v-list-item
        v-for="item in matches"
        :key="item.key"
        :to="item.route"
        :prepend-icon="item.icon"
        :title="item.label"
        :subtitle="item.parent"
        rounded="lg"
        @click="onPick"
      />
      <p v-if="!matches.length" class="text-caption text-medium-emphasis px-3 py-2 mb-0">{{ $t('menu.noMatches') }}</p>
    </v-list>

    <v-list
      v-else
      :opened="rail ? [] : openedGroups"
      nav
      density="compact"
      class="side-nav-list"
      @update:opened="updateOpenedGroups"
    >
      <template v-for="section in sections" :key="section.key">
        <template v-if="section.label">
          <div v-if="!rail" class="side-nav-heading">{{ section.label }}</div>
          <v-divider v-else class="side-nav-rail-divider" />
        </template>

        <template v-for="item in section.items" :key="item.key">
          <!-- Collapsed: icons, with a tooltip, and a flyout for anything with children. -->
          <template v-if="rail">
            <v-menu v-if="item.children" open-on-hover location="end top" :open-delay="60" :close-delay="120" offset="6">
              <template #activator="{ props: menuProps }">
                <v-list-item
                  v-bind="menuProps"
                  :active="groupActive(item)"
                  :aria-label="item.label"
                  rounded="lg"
                  class="side-nav-rail-item"
                  :data-cy="`nav-${item.key}`"
                >
                  <template #prepend>
                    <v-badge :model-value="groupCount(item) > 0" dot color="error" offset-x="-2" offset-y="-2">
                      <v-icon :icon="item.icon" />
                    </v-badge>
                  </template>
                </v-list-item>
              </template>
              <v-list density="compact" nav class="side-nav-flyout">
                <v-list-subheader>{{ item.label }}</v-list-subheader>
                <v-list-item
                  v-for="child in item.children"
                  :key="child.key"
                  :to="child.route"
                  :active="Boolean(activeItem[child.key])"
                  :prepend-icon="child.icon"
                  :title="child.label"
                  rounded="lg"
                  @click="onItemClick"
                />
              </v-list>
            </v-menu>
            <v-tooltip v-else location="end" :text="item.label">
              <template #activator="{ props: tipProps }">
                <v-list-item
                  v-bind="tipProps"
                  :to="item.route"
                  :active="Boolean(activeItem[item.key])"
                  :aria-label="item.label"
                  rounded="lg"
                  class="side-nav-rail-item"
                  :data-cy="`nav-${item.key}`"
                  @click="onItemClick"
                >
                  <template #prepend>
                    <v-badge :model-value="countOf(item) > 0" dot color="error" offset-x="-2" offset-y="-2">
                      <v-icon :icon="item.icon" />
                    </v-badge>
                  </template>
                </v-list-item>
              </template>
            </v-tooltip>
          </template>

          <!-- Expanded. -->
          <v-list-item
            v-else-if="!item.children"
            :to="item.route"
            :active="Boolean(activeItem[item.key])"
            :prepend-icon="item.icon"
            :title="item.label"
            rounded="lg"
            :data-cy="`nav-${item.key}`"
            @click="onItemClick"
          >
            <template v-if="countOf(item) > 0" #append>
              <span class="side-nav-count" :data-cy="`nav-count-${item.key}`">{{ countOf(item) > 99 ? '99+' : countOf(item) }}</span>
            </template>
          </v-list-item>

          <v-list-group v-else :value="item.key">
            <template #activator="{ props: activatorProps }">
              <v-list-item
                v-bind="activatorProps"
                :active="false"
                :class="{ 'side-nav-parent--active': groupActive(item) }"
                :prepend-icon="item.icon"
                :title="item.label"
                rounded="lg"
                :data-cy="`nav-${item.key}`"
              />
            </template>
            <v-list-item
              v-for="child in item.children"
              :key="child.key"
              :to="child.route"
              :active="Boolean(activeItem[child.key])"
              :title="child.label"
              rounded="lg"
              class="side-nav-child"
              :data-cy="`nav-${child.key}`"
              @click="onItemClick"
            />
          </v-list-group>
        </template>
      </template>
    </v-list>
  </div>
</template>

<script setup>
import {
  computed, getCurrentInstance, onBeforeUnmount, onMounted, ref, watch,
} from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { agentGet } from '../agent/client';
import { useStore } from 'vuex';
import { pluginRegistry } from '../pluginHost';
import { mergeNavItems } from '../pluginHost/registry.mjs';

const props = defineProps({
  activeItem: { type: Object, default: () => ({}) },
  openedGroups: { type: Array, default: () => [] },
  rail: { type: Boolean, default: false },
});

const emit = defineEmits(['updateOpenedGroups', 'navigate']);
const { proxy } = getCurrentInstance();
const store = useStore();
const route = useRoute();
const router = useRouter();

const COUNTS_REFRESH_MS = 60000;
const filter = ref('');
const counts = ref({});
let countsTimer = null;

const navItems = computed(() => {
  const { $can, $t, $isSuperAdmin } = proxy;
  const isSuper = typeof $isSuperAdmin === 'function' && $isSuperAdmin();
  const when = (allowed, item) => (allowed ? item : null);
  const group = (key, label, icon, section, children) => {
    const shown = children.filter(Boolean);
    return shown.length ? {
      key, label, icon, section, children: shown,
    } : null;
  };

  const items = [
    {
      key: 'dashboard', label: $t('menu.dashboard'), icon: 'mdi-view-dashboard-outline', route: { name: 'dashboard' }, section: 'main',
    },
    {
      key: 'agent', label: $t('menu.agent'), icon: 'mdi-creation-outline', route: { name: 'agent' }, section: 'main',
    },
    when($can('inbox:get', 'inbox:manage'), {
      key: 'inbox', label: $t('menu.inbox'), icon: 'mdi-inbox-outline', route: { name: 'inbox' }, section: 'main', count: 'inbox',
    }),

    group('subscribers', $t('globals.terms.subscribers'), 'mdi-account-multiple-outline', 'audience', [
      when($can('subscribers:get_all', 'subscribers:get'), {
        key: 'subscribers', label: $t('menu.allSubscribers'), icon: 'mdi-account-multiple-outline', route: { name: 'subscribers' },
      }),
      when($can('subscribers:import'), {
        key: 'import', label: $t('menu.import'), icon: 'mdi-file-upload-outline', route: { name: 'import' },
      }),
      when($can('bounces:get'), {
        key: 'bounces', label: $t('globals.terms.bounces'), icon: 'mdi-email-fast-outline', route: { name: 'bounces' },
      }),
    ]),
    group('lists', $t('globals.terms.lists'), 'mdi-format-list-bulleted-square', 'audience', [
      when($can('lists:get_all', 'lists:get', 'lists:manage_all', 'lists:manage'), {
        key: 'lists', label: $t('menu.allLists'), icon: 'mdi-format-list-bulleted-square', route: { name: 'lists' },
      }),
      when($can('forms:get', 'forms:manage'), {
        key: 'forms', label: $t('menu.forms'), icon: 'mdi-newspaper-variant-outline', route: { name: 'forms' },
      }),
    ]),
    when($can('boards:get'), {
      key: 'boards', label: $t('menu.boards'), icon: 'mdi-view-column-outline', route: { name: 'boards' }, section: 'audience',
    }),
    when($can('subscribers:get_all'), {
      key: 'tasks', label: $t('menu.tasks'), icon: 'mdi-clipboard-text-outline', route: { name: 'tasks' }, section: 'audience', count: 'tasks',
    }),

    group('campaigns', $t('globals.terms.campaigns'), 'mdi-rocket-launch-outline', 'marketing', [
      when($can('campaigns:get'), {
        key: 'campaigns', label: $t('menu.allCampaigns'), icon: 'mdi-rocket-launch-outline', route: { name: 'campaigns' },
      }),
      when($can('campaigns:manage'), {
        key: 'campaign', label: $t('menu.newCampaign'), icon: 'mdi-plus', route: { name: 'campaign', params: { id: 'new' } },
      }),
      when($can('campaigns:get_analytics'), {
        key: 'campaignAnalytics', label: $t('globals.terms.analytics'), icon: 'mdi-chart-bar', route: { name: 'campaignAnalytics' },
      }),
      // Only offered when the active WhatsApp provider can post to channels.
      when(store.state.serverConfig?.whatsapp?.capabilities?.channel_posts && $can('whatsapp:post'), {
        key: 'whatsappChannelPosts', label: $t('whatsapp.posts.title'), icon: 'mdi-whatsapp', route: { name: 'whatsappChannelPosts' },
      }),
    ]),
    when($can('templates:get', 'templates:manage'), {
      key: 'templates', label: $t('globals.terms.templates'), icon: 'mdi-file-image-outline', route: { name: 'templates' }, section: 'marketing',
    }),
    when($can('media:get') || $can('media:manage'), {
      key: 'media', label: $t('menu.media'), icon: 'mdi-image-outline', route: { name: 'media' }, section: 'marketing',
    }),
    when($can('tx:get'), {
      key: 'txMessages', label: $t('menu.transactional'), icon: 'mdi-email-outline', route: { name: 'txMessages' }, section: 'marketing',
    }),

    when($can('workflows:get', 'workflows:manage'), {
      key: 'workflows', label: $t('menu.workflows'), icon: 'mdi-source-branch', route: { name: 'workflows' }, section: 'automation',
    }),
    when($can('workflows:approve', 'workflows:get', 'workflows:manage'), {
      key: 'workflowApprovals', label: $t('workflows.approvals'), icon: 'mdi-account-check-outline', route: { name: 'workflowApprovals' }, section: 'automation', count: 'approvals',
    }),

    group('users', $t('globals.terms.users'), 'mdi-account-cog-outline', 'admin', [
      when($can('users:get'), {
        key: 'users', label: $t('globals.terms.users'), icon: 'mdi-account-multiple-outline', route: { name: 'users' },
      }),
      when($can('roles:get'), {
        key: 'userRoles', label: $t('users.userRoles'), icon: 'mdi-shield-account-outline', route: { name: 'userRoles' },
      }),
      when($can('roles:get'), {
        key: 'listRoles', label: $t('users.listRoles'), icon: 'mdi-format-list-bulleted-square', route: { name: 'listRoles' },
      }),
    ]),
    group('settings', $t('menu.settings'), 'mdi-cog-outline', 'admin', [
      when($can('settings:get'), {
        key: 'settings', label: $t('menu.settings'), icon: 'mdi-cog-outline', route: { name: 'settings' },
      }),
      when($can('settings:maintain'), {
        key: 'maintenance', label: $t('menu.maintenance'), icon: 'mdi-wrench-outline', route: { name: 'maintenance' },
      }),
      when($can('settings:get'), {
        key: 'logs', label: $t('menu.logs'), icon: 'mdi-format-list-bulleted', route: { name: 'logs' },
      }),
      when(isSuper, {
        key: 'plugins', label: $t('menu.plugins'), icon: 'mdi-puzzle-outline', route: { name: 'plugins' },
      }),
      when(isSuper, {
        key: 'adminConsole', label: $t('menu.adminConsole'), icon: 'mdi-console', route: { name: 'adminConsole' },
      }),
    ]),
  ].filter(Boolean);

  return mergeNavItems(items, pluginRegistry.nav, $can);
});

const SECTION_ORDER = ['main', 'audience', 'marketing', 'automation', 'admin', 'plugins'];

// Groups items under headings. Plugin items have no section, so they get their own.
const sections = computed(() => {
  const byKey = {};
  navItems.value.forEach((item) => {
    const key = item.section || 'plugins';
    (byKey[key] ||= []).push(item);
  });
  return SECTION_ORDER.filter((key) => byKey[key]).map((key) => ({
    key,
    label: key === 'main' ? '' : proxy.$t(`menu.section.${key}`),
    items: byKey[key],
  }));
});

const matches = computed(() => {
  const q = String(filter.value || '').trim().toLowerCase();
  if (!q) return [];
  const out = [];
  navItems.value.forEach((item) => {
    (item.children || [item]).forEach((entry) => {
      const parent = item.children ? item.label : '';
      if (`${entry.label} ${parent}`.toLowerCase().includes(q)) {
        out.push({ ...entry, parent });
      }
    });
  });
  return out;
});

function countOf(item) {
  return item.count ? (counts.value[item.count] || 0) : 0;
}

function groupCount(item) {
  return (item.children || []).reduce((sum, child) => sum + countOf(child), 0);
}

function groupActive(item) {
  return (item.children || []).some((child) => props.activeItem[child.key]);
}

async function loadCounts() {
  try {
    counts.value = await agentGet('nav-counts');
  } catch {
    // Counts are a convenience, so a failure just leaves the last ones showing.
  }
}

function openFirstMatch() {
  if (matches.value[0]?.route) {
    router.push(matches.value[0].route);
    onPick();
  }
}

function onPick() {
  filter.value = '';
  onItemClick();
}

// Reading an inbox or finishing a task changes the numbers, so refresh on the way past.
watch(() => route.fullPath, loadCounts);

onMounted(() => {
  loadCounts();
  countsTimer = setInterval(loadCounts, COUNTS_REFRESH_MS);
  window.addEventListener('assistant-applied', loadCounts);
});

onBeforeUnmount(() => {
  clearInterval(countsTimer);
  window.removeEventListener('assistant-applied', loadCounts);
});

function updateOpenedGroups(groups) {
  emit('updateOpenedGroups', groups.slice(-1));
}

function onItemClick() {
  emit('navigate');
}
</script>

<style scoped>
.side-nav-search {
  padding: 0 4px 10px;
}

.side-nav-list {
  background: transparent;
  padding: 0;
}

.side-nav-heading {
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.05em;
  margin: 14px 0 4px;
  padding: 0 12px;
  text-transform: uppercase;
}

.side-nav-list :deep(.v-list-item--active) {
  background: rgba(var(--v-theme-primary), 0.14);
  color: rgb(var(--v-theme-primary));
}

.side-nav-list :deep(.v-list-item--active .v-icon) {
  color: rgb(var(--v-theme-primary));
}

.side-nav-list :deep(.v-list-item__prepend > .v-icon) {
  margin-inline-end: 12px;
}

.side-nav-list :deep(.v-list-item-title) {
  font-size: 14px;
}

.side-nav-parent--active :deep(.v-list-item-title) {
  color: rgb(var(--v-theme-primary));
  font-weight: 500;
}

/* Children line up under the parent's label, so they don't need icons of their own. */
.side-nav-list :deep(.v-list-group__items .side-nav-child) {
  padding-inline-start: 42px !important;
}

.side-nav-count {
  background: rgba(var(--v-theme-primary), 0.16);
  border-radius: 999px;
  color: rgb(var(--v-theme-primary));
  font-size: 11px;
  font-weight: 600;
  line-height: 18px;
  min-width: 18px;
  padding: 0 6px;
  text-align: center;
}

.side-nav-rail-divider {
  margin: 8px 10px;
}

.side-nav--rail .side-nav-rail-item {
  justify-content: center;
  padding-inline: 0 !important;
}

.side-nav--rail :deep(.v-list-item__prepend) {
  margin-inline-end: 0;
}

.side-nav--rail :deep(.v-list-item__spacer) {
  display: none;
}

.side-nav-flyout {
  min-width: 200px;
}
</style>
