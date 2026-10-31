import { createRouter, createWebHistory } from 'vue-router';
import { isAuthenticated } from '../api';
import { isCurrentUserSuperAdmin } from '../utils/auth';
import { toRouterPath } from '../utils/authRoutes';
import { ensurePluginsLoaded } from '../pluginHost';

const routes = [
  {
    path: '/login',
    name: 'login',
    meta: { title: 'users.login', guestOnly: true, authPage: true },
    component: () => import('../views/AuthLogin.vue'),
  },
  {
    path: '/login/twofa',
    name: 'loginTwofa',
    meta: { title: 'users.twoFA', guestOnly: true, authPage: true },
    component: () => import('../views/AuthTwofa.vue'),
  },
  {
    path: '/forgot',
    name: 'forgotPassword',
    meta: { title: 'users.forgotPassword', guestOnly: true, authPage: true },
    component: () => import('../views/AuthForgotPassword.vue'),
  },
  {
    path: '/reset',
    name: 'resetPassword',
    meta: { title: 'users.resetPassword', guestOnly: true, authPage: true },
    component: () => import('../views/AuthResetPassword.vue'),
  },
  {
    path: '/404',
    name: '404_page',
    meta: { title: 'notFound.title' },
    component: () => import('../views/404.vue'),
  },
  {
    path: '/',
    name: 'dashboard',
    meta: { title: 'menu.overview' },
    component: () => import('../views/Dashboard.vue'),
  },
  {
    path: '/lists',
    name: 'lists',
    meta: { title: 'globals.terms.lists', group: 'lists' },
    component: () => import('../views/Lists.vue'),
  },
  {
    path: '/lists/forms',
    name: 'forms',
    meta: { title: 'forms.title', group: 'lists' },
    component: () => import('../views/Forms.vue'),
  },
  {
    path: '/lists/:id',
    name: 'list',
    meta: { nav: 'lists', title: 'globals.terms.lists', group: 'lists' },
    component: () => import('../views/Lists.vue'),
  },
  {
    path: '/subscribers',
    name: 'subscribers',
    meta: { title: 'globals.terms.subscribers', group: 'subscribers' },
    component: () => import('../views/Subscribers.vue'),
  },
  {
    path: '/subscribers/import',
    name: 'import',
    meta: { title: 'import.title', group: 'subscribers' },
    component: () => import('../views/Import.vue'),
  },
  {
    path: '/subscribers/bounces',
    name: 'bounces',
    meta: { title: 'globals.terms.bounces', group: 'subscribers' },
    component: () => import('../views/Bounces.vue'),
  },
  {
    path: '/subscribers/lists/:listID',
    name: 'subscribers_list',
    meta: { nav: 'subscribers', title: 'globals.terms.subscribers', group: 'subscribers' },
    component: () => import('../views/Subscribers.vue'),
  },
  {
    path: '/subscribers/:id',
    name: 'subscriber',
    meta: { nav: 'subscribers', title: 'globals.terms.subscribers', group: 'subscribers' },
    component: () => import('../views/SubscriberContact.vue'),
  },
  {
    path: '/campaigns',
    name: 'campaigns',
    meta: { title: 'globals.terms.campaigns', group: 'campaigns' },
    component: () => import('../views/Campaigns.vue'),
  },
  {
    path: '/campaigns/media',
    name: 'media',
    meta: { title: 'globals.terms.media', group: 'campaigns' },
    component: () => import('../views/Media.vue'),
  },
  {
    path: '/campaigns/whatsapp-channel',
    name: 'whatsappChannelPosts',
    meta: { title: 'whatsapp.posts.title', group: 'campaigns' },
    component: () => import('../views/WhatsAppChannelPosts.vue'),
  },
  {
    path: '/campaigns/templates',
    name: 'templates',
    meta: { title: 'globals.terms.templates', group: 'campaigns' },
    component: () => import('../views/Templates.vue'),
  },
  {
    path: '/campaigns/transactional',
    name: 'txMessages',
    meta: { title: 'menu.transactional', group: 'campaigns' },
    component: () => import('../views/TxMessages.vue'),
  },
  {
    path: '/campaigns/analytics',
    name: 'campaignAnalytics',
    meta: { title: 'analytics.title', group: 'campaigns' },
    component: () => import('../views/CampaignAnalytics.vue'),
  },
  {
    path: '/campaigns/:id',
    name: 'campaign',
    meta: { title: 'globals.terms.campaign', group: 'campaigns' },
    component: () => import('../views/Campaign.vue'),
  },
  {
    path: '/user/profile',
    name: 'userProfile',
    meta: { title: 'users.profile', group: 'settings' },
    component: () => import('../views/UserProfile.vue'),
  },
  {
    path: '/settings',
    name: 'settings',
    meta: { title: 'globals.terms.settings', group: 'settings' },
    component: () => import('../views/Settings.vue'),
  },
  {
    path: '/settings/logs',
    name: 'logs',
    meta: { title: 'logs.title', group: 'settings' },
    component: () => import('../views/Logs.vue'),
  },
  {
    path: '/users',
    name: 'users',
    meta: { title: 'globals.terms.users', group: 'users' },
    component: () => import('../views/Users.vue'),
  },
  {
    path: '/users/roles/users',
    name: 'userRoles',
    meta: { title: 'users.userRoles', group: 'users' },
    component: () => import('../views/Roles.vue'),
  },
  {
    path: '/users/roles/lists',
    name: 'listRoles',
    meta: { title: 'users.listRoles', group: 'users' },
    component: () => import('../views/Roles.vue'),
  },
  {
    path: '/settings/maintenance',
    name: 'maintenance',
    meta: { title: 'maintenance.title', group: 'settings' },
    component: () => import('../views/Maintenance.vue'),
  },
  {
    path: '/settings/admin-console',
    name: 'adminConsole',
    meta: { title: 'menu.adminConsole', group: 'settings', superAdmin: true },
    component: () => import('../views/AdminConsole.vue'),
  },
  {
    path: '/workflows',
    name: 'workflows',
    meta: { title: 'menu.workflows' },
    component: () => import('../views/Workflows.vue'),
  },
  {
    path: '/workflows/approvals',
    name: 'workflowApprovals',
    meta: { title: 'workflows.approvals' },
    component: () => import('../views/WorkflowApprovals.vue'),
  },
  {
    path: '/workflows/runs/:id',
    name: 'workflowRun',
    meta: { nav: 'workflows', title: 'menu.workflows' },
    component: () => import('../views/WorkflowRun.vue'),
  },
  {
    path: '/workflows/:id',
    name: 'workflowBuilder',
    meta: { nav: 'workflows', title: 'menu.workflows' },
    component: () => import('../views/WorkflowBuilder.vue'),
  },
  {
    path: '/settings/plugins',
    name: 'plugins',
    meta: { title: 'menu.plugins', group: 'settings', superAdmin: true },
    component: () => import('../views/Plugins.vue'),
  },
  {
    path: '/agent/:id?',
    name: 'agent',
    meta: { title: 'menu.agent' },
    component: () => import('../views/Agent.vue'),
  },
  {
    path: '/inbox',
    name: 'inbox',
    meta: { title: 'menu.inbox' },
    component: () => import('../views/Conversations.vue'),
  },
  {
    path: '/inbox/email',
    name: 'inboundEmail',
    meta: { nav: 'inbox', title: 'conversations.emailLog' },
    component: () => import('../views/InboundEmailInbox.vue'),
  },
  {
    path: '/crm/pipeline',
    redirect: { name: 'boards' },
  },
  {
    path: '/crm/boards',
    name: 'boards',
    meta: { title: 'menu.boards' },
    component: () => import('../views/Boards.vue'),
  },
  {
    path: '/crm/boards/:board',
    name: 'board',
    meta: { nav: 'boards', title: 'menu.boards' },
    component: () => import('../views/Board.vue'),
  },
  {
    path: '/crm/tasks',
    name: 'tasks',
    meta: { title: 'menu.tasks',  },
    component: () => import('../views/Tasks.vue'),
  },
];

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
  scrollBehavior(to) {
    if (to.hash) {
      const campaignTabHashes = new Set(['#campaign', '#content', '#attribs', '#archive']);

      if (to.name === 'campaign' && campaignTabHashes.has(to.hash)) {
        return false;
      }

      if (typeof document !== 'undefined' && document.querySelector(to.hash)) {
        return { el: to.hash };
      }

      return false;
    }
    return { top: 0 };
  },
});

router.beforeEach(async (to) => {
  const authenticated = isAuthenticated();
  if (authenticated) {
    // Plugin routes don't exist until here, so resolve deep links to them again.
    const justLoaded = await ensurePluginsLoaded();
    if (justLoaded && to.matched.length === 0) {
      return to.fullPath;
    }
  }
  if (to.matched.length === 0) {
    return '/404';
  }
  if (to.meta && to.meta.guestOnly) {
    if (authenticated) {
      const next = typeof to.query.next === 'string' && to.query.next ? to.query.next : '/';
      return toRouterPath(next);
    }
    return true;
  }
  if (!authenticated) {
    return {
      path: '/login',
      query: to.path === '/' && !to.query && !to.hash ? {} : { next: to.fullPath },
    };
  }
  if (to.meta && to.meta.superAdmin && !isCurrentUserSuperAdmin()) {
    return '/';
  }
  return true;
});

export default router;
