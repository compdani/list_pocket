import { createRouter, createWebHistory } from 'vue-router';
import { isAuthenticated } from '../api';
import { isCurrentUserSuperAdmin } from '../utils/auth';
import { toRouterPath } from '../utils/authRoutes';

const routes = [
  {
    path: '/login',
    name: 'login',
    meta: { title: 'Login', guestOnly: true, authPage: true },
    component: () => import('../views/AuthLogin.vue'),
  },
  {
    path: '/login/twofa',
    name: 'loginTwofa',
    meta: { title: 'Two-Factor Authentication', guestOnly: true, authPage: true },
    component: () => import('../views/AuthTwofa.vue'),
  },
  {
    path: '/forgot',
    name: 'forgotPassword',
    meta: { title: 'Forgot Password', guestOnly: true, authPage: true },
    component: () => import('../views/AuthForgotPassword.vue'),
  },
  {
    path: '/reset',
    name: 'resetPassword',
    meta: { title: 'Reset Password', guestOnly: true, authPage: true },
    component: () => import('../views/AuthResetPassword.vue'),
  },
  {
    path: '/404',
    name: '404_page',
    meta: { title: '404' },
    component: () => import('../views/404.vue'),
  },
  {
    path: '/',
    name: 'dashboard',
    meta: { title: '' },
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
    meta: { title: 'globals.terms.lists', group: 'lists' },
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
    meta: { title: 'globals.terms.subscribers', group: 'subscribers' },
    component: () => import('../views/Subscribers.vue'),
  },
  {
    path: '/subscribers/:id',
    name: 'subscriber',
    meta: { title: 'globals.terms.subscribers', group: 'subscribers' },
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
    path: '/campaigns/templates',
    name: 'templates',
    meta: { title: 'globals.terms.templates', group: 'campaigns' },
    component: () => import('../views/Templates.vue'),
  },
  {
    path: '/campaigns/transactional',
    name: 'txMessages',
    meta: { title: 'Transactional Messages', group: 'campaigns' },
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
    meta: { title: 'Admin Console', group: 'settings', superAdmin: true },
    component: () => import('../views/AdminConsole.vue'),
  },
  {
    path: '/workflows',
    name: 'workflows',
    meta: { title: 'Workflows', group: 'workflows' },
    component: () => import('../views/Workflow.vue'),
  },
  {
    path: '/workflows/builder/:workflowId?',
    name: 'workflowBuilder',
    meta: { title: 'Workflow Builder', group: 'workflows' },
    component: () => import('../views/WorkflowBuilder.vue'),
  },
  {
    path: '/inbox',
    name: 'inbox',
    meta: { title: 'menu.inbox', group: 'inbox' },
    component: () => import('../views/InboundEmailInbox.vue'),
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

router.beforeEach((to) => {
  if (to.matched.length === 0) {
    return '/404';
  }
  const authenticated = isAuthenticated();
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
