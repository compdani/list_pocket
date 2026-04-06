<template>
  <v-list
    :opened="rail ? [] : openedGroups"
    nav
    density="comfortable"
    class="workflow-nav"
    @update:opened="updateOpenedGroups"
  >
    <template v-if="rail">
      <template v-for="item in navItems">
        <v-tooltip v-if="!item.children" :key="`rail-${item.key}`" location="right">
          <template #activator="{ props: tooltipProps }">
            <v-list-item
              v-bind="tooltipProps"
              :active="Boolean(activeItem[item.key])"
              :prepend-icon="item.icon"
              rounded="lg"
              class="workflow-nav-rail-item"
              @click="navigate(item)"
            />
          </template>
          <span>{{ item.label }}</span>
        </v-tooltip>

        <v-menu
          v-else
          :key="`rail-${item.key}`"
          :activator="`#rail-nav-item-${item.key}`"
          location="end"
          origin="start top"
          offset="8"
        >
          <v-list-item
            :id="`rail-nav-item-${item.key}`"
            :active="item.children.some((child) => Boolean(activeItem[child.key]))"
            :prepend-icon="item.icon"
            rounded="lg"
            class="workflow-nav-rail-item"
          />

          <v-list min-width="220" density="comfortable">
            <v-list-subheader>{{ item.label }}</v-list-subheader>
            <v-list-item
              v-for="child in item.children"
              :key="`rail-child-${child.key}`"
              :active="Boolean(activeItem[child.key])"
              :prepend-icon="child.icon"
              :title="child.label"
              rounded="lg"
              @click="navigate(child)"
            />
          </v-list>
        </v-menu>
      </template>
    </template>

    <template v-else>
      <template v-for="item in navItems">
        <v-list-item
          v-if="!item.children"
          :key="`item-${item.key}`"
          :active="Boolean(activeItem[item.key])"
          :prepend-icon="item.icon"
          :title="item.label"
          rounded="lg"
          @click="navigate(item)"
        />

        <v-list-group
          v-else
          :key="`group-${item.key}`"
          :value="item.key"
        >
          <template #activator="{ props: activatorProps }">
            <v-list-item
              v-bind="activatorProps"
              :active="openedGroups.includes(item.key)"
              :prepend-icon="item.icon"
              :title="item.label"
              rounded="lg"
            />
          </template>

          <v-list-item
            v-for="child in item.children"
            :key="child.key"
            :active="Boolean(activeItem[child.key])"
            :prepend-icon="child.icon"
            :title="child.label"
            rounded="lg"
            class="workflow-nav-child"
            @click="navigate(child)"
          />
        </v-list-group>
      </template>
    </template>
  </v-list>
</template>

<script setup>
import { computed, getCurrentInstance } from 'vue';
import { useRouter } from 'vue-router';

defineProps({
  activeItem: { type: Object, default: () => ({}) },
  openedGroups: { type: Array, default: () => [] },
  rail: { type: Boolean, default: false },
});

const emit = defineEmits(['updateOpenedGroups', 'navigate']);
const router = useRouter();
const { proxy } = getCurrentInstance();

const navItems = computed(() => {
  const { $can, $t } = proxy;

  const items = [
    {
      key: 'dashboard',
      label: $t('menu.dashboard'),
      icon: 'mdi-view-dashboard-outline',
      route: { name: 'dashboard' },
    },
  ];
  const workflowChildren = [
    {
      key: 'workflows',
      label: 'Overview',
      icon: 'mdi-view-grid-outline',
      route: { name: 'workflows' },
    },
    {
      key: 'workflowBuilder',
      label: 'Builder',
      icon: 'mdi-vector-polyline',
      route: { name: 'workflowBuilder' },
    },
  ];

  items.push({
    key: 'workflows',
    label: 'Workflows',
    icon: 'mdi-source-branch',
    children: workflowChildren,
  });

  const listsChildren = [
    $can('lists:get_all', 'lists:get', 'lists:manage_all', 'lists:manage') ? {
      key: 'lists',
      label: $t('menu.allLists'),
      icon: 'mdi-format-list-bulleted-square',
      route: { name: 'lists' },
    } : null,
    $can('forms:get', 'forms:manage') ? {
      key: 'forms',
      label: $t('menu.forms'),
      icon: 'mdi-newspaper-variant-outline',
      route: { name: 'forms' },
    } : null,
  ].filter(Boolean);

  if (listsChildren.length > 0) {
    items.push({
      key: 'lists',
      label: $t('globals.terms.lists'),
      icon: 'mdi-format-list-bulleted-square',
      children: listsChildren,
    });
  }

  const subscriberChildren = [
    $can('subscribers:get_all', 'subscribers:get') ? {
      key: 'subscribers',
      label: $t('menu.allSubscribers'),
      icon: 'mdi-account-multiple-outline',
      route: { name: 'subscribers' },
    } : null,
    $can('subscribers:import') ? {
      key: 'import',
      label: $t('menu.import'),
      icon: 'mdi-file-upload-outline',
      route: { name: 'import' },
    } : null,
    $can('bounces:get') ? {
      key: 'bounces',
      label: $t('globals.terms.bounces'),
      icon: 'mdi-email-fast-outline',
      route: { name: 'bounces' },
    } : null,
  ].filter(Boolean);

  if (subscriberChildren.length > 0) {
    items.push({
      key: 'subscribers',
      label: $t('globals.terms.subscribers'),
      icon: 'mdi-account-multiple-outline',
      children: subscriberChildren,
    });
  }

  const campaignChildren = [
    $can('campaigns:get') ? {
      key: 'campaigns',
      label: $t('menu.allCampaigns'),
      icon: 'mdi-rocket-launch-outline',
      route: { name: 'campaigns' },
    } : null,
    $can('campaigns:manage') ? {
      key: 'campaign',
      label: $t('menu.newCampaign'),
      icon: 'mdi-plus',
      route: { name: 'campaign', params: { id: 'new' } },
    } : null,
    $can('media:get') || $can('media:manage') ? {
      key: 'media',
      label: $t('menu.media'),
      icon: 'mdi-image-outline',
      route: { name: 'media' },
    } : null,
    $can('templates:get', 'templates:manage') ? {
      key: 'templates',
      label: $t('globals.terms.templates'),
      icon: 'mdi-file-image-outline',
      route: { name: 'templates' },
    } : null,
    $can('tx:get') ? {
      key: 'txMessages',
      label: 'Transactional',
      icon: 'mdi-email-outline',
      route: { name: 'txMessages' },
    } : null,
    $can('campaigns:get_analytics') ? {
      key: 'campaignAnalytics',
      label: $t('globals.terms.analytics'),
      icon: 'mdi-chart-bar',
      route: { name: 'campaignAnalytics' },
    } : null,
  ].filter(Boolean);

  if (campaignChildren.length > 0) {
    items.push({
      key: 'campaigns',
      label: $t('globals.terms.campaigns'),
      icon: 'mdi-rocket-launch-outline',
      children: campaignChildren,
    });
  }

  const userChildren = [
    $can('users:get') ? {
      key: 'users',
      label: $t('globals.terms.users'),
      icon: 'mdi-account-multiple-outline',
      route: { name: 'users' },
    } : null,
    $can('roles:get') ? {
      key: 'userRoles',
      label: $t('users.userRoles'),
      icon: 'mdi-shield-account-outline',
      route: { name: 'userRoles' },
    } : null,
    $can('roles:get') ? {
      key: 'listRoles',
      label: $t('users.listRoles'),
      icon: 'mdi-format-list-bulleted-square',
      route: { name: 'listRoles' },
    } : null,
  ].filter(Boolean);

  if (userChildren.length > 0) {
    items.push({
      key: 'users',
      label: $t('globals.terms.users'),
      icon: 'mdi-account-cog-outline',
      children: userChildren,
    });
  }

  const settingsChildren = [
    $can('settings:get') ? {
      key: 'settings',
      label: $t('menu.settings'),
      icon: 'mdi-cog-outline',
      route: { name: 'settings' },
    } : null,
    $can('settings:maintain') ? {
      key: 'maintenance',
      label: $t('menu.maintenance'),
      icon: 'mdi-wrench-outline',
      route: { name: 'maintenance' },
    } : null,
    $can('settings:get') ? {
      key: 'logs',
      label: $t('menu.logs'),
      icon: 'mdi-format-list-bulleted',
      route: { name: 'logs' },
    } : null,
  ].filter(Boolean);

  if (settingsChildren.length > 0) {
    items.push({
      key: 'settings',
      label: $t('menu.settings'),
      icon: 'mdi-cog-outline',
      children: settingsChildren,
    });
  }

  return items;
});

function updateOpenedGroups(groups) {
  emit('updateOpenedGroups', groups.slice(-1));
}

function navigate(item) {
  if (!item.route) {
    return;
  }
  router.push(item.route);
  emit('navigate');
}
</script>

<style scoped>
.workflow-nav {
  background: transparent;
}

.workflow-nav-child {
  margin-left: 12px;
}

.workflow-nav-rail-item {
  justify-content: center;
  margin-inline: 4px;
}

:deep(.workflow-nav-rail-item .v-list-item__prepend) {
  width: auto;
}

:deep(.workflow-nav-rail-item .v-list-item__spacer),
:deep(.workflow-nav-rail-item .v-list-item__content),
:deep(.workflow-nav-rail-item .v-list-item__append) {
  display: none;
}
</style>
