import PocketBase from 'pocketbase';
import qs from 'qs';
import dayjs from 'dayjs';
import store from '../store';
import { models } from '../constants';
import Utils from '../utils';

const rootURL = (import.meta.env.VUE_APP_ROOT_URL || '/').trim();
const pbBaseURL = rootURL === '/' ? window.location.origin : rootURL.replace(/\/$/, '');
const pb = new PocketBase(pbBaseURL);
pb.autoCancellation(false);

pb.beforeSend = (url, sendOptions) => ({
  url,
  sendOptions: {
    ...sendOptions,
    credentials: 'omit',
  },
});

const utils = new Utils();

function setLoading(config, status) {
  if ('loading' in config) {
    store.commit('setLoading', { model: config.loading, status });
  }
}

function transformResponse(resp, config) {
  const payload = resp && typeof resp === 'object' && 'data' in resp ? resp.data : resp;

  let out = {};
  if (typeof payload === 'object') {
    if (payload && payload.constructor === Object) {
      out = { ...payload };
    } else if (payload) {
      out = [...payload];
    }

    switch (typeof config.camelCase) {
      case 'function':
        out = utils.camelKeys(out, config.camelCase);
        break;
      case 'boolean':
        if (config.camelCase) {
          out = utils.camelKeys(out);
        }
        break;
      default:
        out = utils.camelKeys(out);
        break;
    }
  } else {
    out = payload;
  }

  if ('store' in config) {
    store.commit('setModelResponse', { model: config.store, data: out });
  }

  return out;
}

function getErrorMessage(err) {
  if (err && err.response && err.response.message) {
    return err.response.message;
  }
  if (err && err.message) {
    return err.message;
  }
  return String(err);
}

async function send(method, url, data, config = {}) {
  setLoading(config, true);

  const baseRequestURL = url.startsWith('/api/')
    ? `/mailapi/${url.slice('/api/'.length)}`
    : url;
  const requestURL = config.params
    ? `${baseRequestURL}${baseRequestURL.includes('?') ? '&' : '?'}${qs.stringify(config.params, { arrayFormat: 'repeat' })}`
    : baseRequestURL;

  try {
    const requestConfig = {
      method,
    };

    if (data !== undefined) {
      requestConfig.body = data;
    }

    const response = await pb.send(requestURL, requestConfig);
    return transformResponse(response, config);
  } catch (err) {
    const msg = getErrorMessage(err);

    if (!config.disableToast) {
      utils.toast(msg, 'is-danger', 3000, false);
    }

    return Promise.reject(err);
  } finally {
    setLoading(config, false);
  }
}

function toUTCISOString(value) {
  if (value === null || typeof value === 'undefined' || value === '') {
    return null;
  }

  const parsed = dayjs(value);
  if (!parsed.isValid()) {
    return null;
  }

  return parsed.toDate().toISOString();
}

function getUserTimeZone() {
  try {
    const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
    return typeof tz === 'string' && tz.trim() ? tz : null;
  } catch {
    return null;
  }
}

function normalizeAnalyticsParams(params = {}) {
  const out = { ...params };
  ['from', 'to'].forEach((key) => {
    const normalized = toUTCISOString(out[key]);
    if (normalized) {
      // Always send analytics range bounds in UTC to the backend.
      out[key] = normalized;
    }
  });

  const userTimeZone = getUserTimeZone();
  if (userTimeZone) {
    out.tz = userTimeZone;
  }

  return out;
}

async function sendControlPlane(method, url, data, config = {}) {
  setLoading(config, true);

  try {
    const requestConfig = {
      method,
    };

    if (data !== undefined) {
      requestConfig.body = data;
    }

    const response = await pb.send(url, requestConfig);
    return transformResponse(response, config);
  } catch (err) {
    const msg = getErrorMessage(err);

    if (!config.disableToast) {
      utils.toast(msg, 'is-danger', 3000, false);
    }

    return Promise.reject(err);
  } finally {
    setLoading(config, false);
  }
}

const http = {
  get(url, config = {}) {
    return send('GET', url, undefined, config);
  },
  post(url, data = {}, config = {}) {
    return send('POST', url, data, config);
  },
  put(url, data = {}, config = {}) {
    return send('PUT', url, data, config);
  },
  delete(url, config = {}) {
    return send('DELETE', url, config.data, config);
  },
};

export const getAuthToken = () => pb.authStore.token;
export const clearAuthToken = () => pb.authStore.clear();
export const getAuthRecord = () => pb.authStore.record;

function normalizeAuthProfile(profile) {
  if (!profile) {
    return profile;
  }

  const normalized = utils.camelKeys(profile);
  if (!normalized.userRole) {
    normalized.userRole = { id: 0, permissions: [] };
  }
  if (!Array.isArray(normalized.userRole.permissions)) {
    normalized.userRole.permissions = [];
  }
  if (!normalized.listRole) {
    normalized.listRole = { lists: [] };
  }
  if (!Array.isArray(normalized.listRole.lists)) {
    normalized.listRole.lists = [];
  }

  if (normalized.role && (!normalized.userRole || !normalized.userRole.id)) {
    normalized.userRole = {
      ...normalized.userRole,
      id: Number(normalized.role) || 0,
      permissions: Array.isArray(normalized.userRole && normalized.userRole.permissions)
        ? normalized.userRole.permissions
        : [],
    };
  }

  return normalized;
}

function syncAuthProfile(profile) {
  const normalized = normalizeAuthProfile(profile);
  if (!normalized || !pb.authStore.record) {
    return normalized;
  }

  pb.authStore.save(pb.authStore.token, {
    ...pb.authStore.record,
    profile: normalized,
  });

  store.commit('setModelResponse', { model: models.profile, data: normalized });
  return normalized;
}

export const getStoredUserProfile = () => syncAuthProfile(pb.authStore.record && pb.authStore.record.profile
  ? pb.authStore.record.profile
  : null);

// Authenticate with PocketBase using username/password
export const login = async (username, password) => {
  try {
    // Use PocketBase SDK's native authentication
    const authData = await pb.collection('users').authWithPassword(username, password);
    return authData;
  } catch (err) {
    pb.authStore.clear();
    throw err;
  }
};

// Check if user is authenticated
export const isAuthenticated = () => pb.authStore.isValid;

// Export pb instance for direct access to PocketBase SDK
export { pb };

// API calls accept the following config keys.
// loading: modelName (set's the loading status in the global store: eg: store.loading.lists = true)
// store: modelName (set's the API response in the global store. eg: store.lists: { ... } )

// Health check endpoint that does not throw a toast.
export const getHealth = () => http.get(
  '/api/health',
  { disableToast: true },
);

export const reloadApp = () => http.post('/api/admin/reload');

// Dashboard
export const getDashboardCounts = () => http.get(
  '/api/dashboard/counts',
  { loading: models.dashboard },
);

export const getDashboardCharts = () => http.get(
  '/api/dashboard/charts',
  { loading: models.dashboard },
);

export const getWorkflowDashboard = (workflowId) => sendControlPlane(
  'GET',
  `/api/control-plane/dashboard${workflowId ? `?${qs.stringify({ workflowId })}` : ''}`,
  undefined,
  { disableToast: true },
);

export const createWorkflow = (data = {}) => sendControlPlane(
  'POST',
  '/api/control-plane/workflows',
  data,
);

export const deleteWorkflow = (workflowId) => sendControlPlane(
  'DELETE',
  `/api/control-plane/workflows/${workflowId}`,
);

export const saveWorkflowGraph = (workflowId, data) => sendControlPlane(
  'POST',
  `/api/control-plane/workflows/${workflowId}/save`,
  data,
);

export const publishWorkflow = (workflowId) => sendControlPlane(
  'POST',
  `/api/control-plane/workflows/${workflowId}/publish`,
);

export const validateWorkflow = (workflowId) => sendControlPlane(
  'POST',
  `/api/control-plane/workflows/${workflowId}/validate`,
);

export const runWorkflow = (workflowId, data = {}) => sendControlPlane(
  'POST',
  `/api/control-plane/workflows/${workflowId}/run`,
  data,
);

export const armWorkflowWebhookCapture = (workflowId, mode) => sendControlPlane(
  'POST',
  `/api/control-plane/workflows/${workflowId}/webhook-capture`,
  { mode },
);

export const getWorkflowWebhookCapture = (sessionId) => sendControlPlane(
  'GET',
  `/api/control-plane/webhook-captures/${sessionId}`,
  undefined,
  { disableToast: true },
);

export const getWorkflowRunDetail = (runId) => sendControlPlane(
  'GET',
  `/api/control-plane/runs/${runId}`,
  undefined,
  { disableToast: true },
);

export const cancelWorkflowRun = (runId) => sendControlPlane(
  'POST',
  `/api/control-plane/runs/${runId}/cancel`,
);

// Lists.
export const getLists = (params) => http.get(
  '/api/lists',
  {
    params: (!params ? { per_page: 'all' } : params),
    loading: models.lists,
    store: models.lists,
  },
);

export const queryLists = (params) => http.get(
  '/api/lists',
  {
    params: (!params ? { per_page: 'all' } : params),
    loading: models.listsFull,
  },
);

export const getList = async (idOrRecordId) => http.get(
  `/api/lists/${idOrRecordId}`,
  { loading: models.list },
);

export const createList = (data) => http.post(
  '/api/lists',
  data,
  { loading: models.lists },
);

export const updateList = (data) => http.put(
  `/api/lists/${data.id}`,
  data,
  { loading: models.lists },
);

export const deleteList = (idOrRecordId) => http.delete(
  `/api/lists/${idOrRecordId}`,
  { loading: models.lists },
);

export const deleteLists = (params) => http.delete(
  '/api/lists',
  { params, loading: models.lists },
);

// Subscribers.
export const getSubscribers = async (params) => http.get(
  '/api/subscribers',
  {
    params,
    loading: models.subscribers,
    store: models.subscribers,
    camelCase: (keyPath) => !keyPath.startsWith('.results.*.attribs'),
  },
);

export const searchSubscribers = async (params) => http.get(
  '/api/subscribers',
  {
    params,
    disableToast: true,
    camelCase: (keyPath) => !keyPath.startsWith('.results.*.attribs'),
  },
);

export const getSubscriber = async (idOrRecordId) => http.get(
  `/api/subscribers/${idOrRecordId}`,
  { loading: models.subscribers },
);

export const getSubscriberActivity = async (idOrRecordId) => http.get(
  `/api/subscribers/${idOrRecordId}/activity`,
  { loading: models.subscribers },
);

export const getSubscriberBounces = async (idOrRecordId) => http.get(
  `/api/subscribers/${idOrRecordId}/bounces`,
  { loading: models.bounces },
);

export const deleteSubscriberBounces = async (idOrRecordId) => http.delete(
  `/api/subscribers/${idOrRecordId}/bounces`,
  { loading: models.bounces },
);

export const deleteBounce = async (id) => http.delete(
  `/api/bounces/${id}`,
  { loading: models.bounces },
);

export const deleteBounces = async (params) => http.delete(
  '/api/bounces',
  { params, loading: models.bounces },
);

export const blocklistBouncedSubscribers = async () => http.put(
  '/api/bounces/blocklist',
  { loading: models.bounces },
);

export const createSubscriber = (data) => http.post(
  '/api/subscribers',
  data,
  { loading: models.subscribers },
);

export const updateSubscriber = (data) => http.put(
  `/api/subscribers/${data.id}`,
  data,
  { loading: models.subscribers },
);

export const sendSubscriberOptin = (idOrRecordId) => http.post(
  `/api/subscribers/${idOrRecordId}/optin`,
  {},
  { loading: models.subscribers },
);

export const getTxMessages = async (params) => http.get(
  '/api/tx',
  {
    params,
    loading: models.txMessages,
    store: models.txMessages,
  },
);

export const getTxMessage = async (idOrRecordId) => http.get(
  `/api/tx/${idOrRecordId}`,
  { loading: models.txMessages },
);

export const deleteSubscriber = (idOrRecordId) => http.delete(
  `/api/subscribers/${idOrRecordId}`,
  { loading: models.subscribers },
);

export const addSubscribersToLists = (data) => http.put(
  '/api/subscribers/lists',
  data,
  { loading: models.subscribers },
);

export const addSubscribersToListsByQuery = (data) => http.put(
  '/api/subscribers/query/lists',
  data,

  { loading: models.subscribers },
);

export const blocklistSubscribers = (data) => http.put(
  '/api/subscribers/blocklist',
  data,
  { loading: models.subscribers },
);

export const blocklistSubscribersByQuery = (data) => http.put(
  '/api/subscribers/query/blocklist',
  data,
  { loading: models.subscribers },
);

export const deleteSubscribers = (params) => http.delete(
  '/api/subscribers',
  { params, loading: models.subscribers },
);

export const deleteSubscribersByQuery = (data) => http.post(
  '/api/subscribers/query/delete',
  data,
  { loading: models.subscribers },
);

// Subscriber import.
export const importSubscribers = (data) => http.post('/api/import/subscribers', data);

export const getImportStatus = () => http.get('/api/import/subscribers');

export const getImportLogs = async () => http.get(
  '/api/import/subscribers/logs',
  { camelCase: false },
);

export const stopImport = () => http.delete('/api/import/subscribers');

// Bounces.
export const getBounces = async (params) => http.get(
  '/api/bounces',
  { params, loading: models.bounces },
);

// Campaigns.
export const getCampaigns = async (params) => http.get('/api/campaigns', {
  params,
  loading: models.campaigns,
  store: models.campaigns,
  camelCase: (keyPath) => !keyPath.startsWith('.results.*.headers'),
});

export const getCampaign = async (idOrRecordId) => http.get(`/api/campaigns/${idOrRecordId}`, {
  loading: models.campaigns,
  camelCase: (keyPath) => !keyPath.startsWith('.headers'),
});

export const getCampaignStats = async () => http.get('/api/campaigns/running/stats', {});

export const createCampaign = async (data) => http.post(
  '/api/campaigns',
  data,
  { loading: models.campaigns },
);

export const getCampaignViewCounts = async (params) => http.get(
  '/api/campaigns/analytics/views',
  { params: normalizeAnalyticsParams(params), loading: models.campaigns },
);

export const getCampaignUniqueViewCounts = async (params) => http.get(
  '/api/campaigns/analytics/views_unique',
  { params: normalizeAnalyticsParams(params), loading: models.campaigns },
);

export const getCampaignUniqueViewTotalCounts = async (params) => http.get(
  '/api/campaigns/analytics/views_unique_total',
  { params: normalizeAnalyticsParams(params), loading: models.campaigns },
);

export const getCampaignClickCounts = async (params) => http.get(
  '/api/campaigns/analytics/clicks',
  { params: normalizeAnalyticsParams(params), loading: models.campaigns },
);

export const getCampaignBounceCounts = async (params) => http.get(
  '/api/campaigns/analytics/bounces',
  { params: normalizeAnalyticsParams(params), loading: models.campaigns },
);

export const getCampaignUnsubscribeCounts = async (params) => http.get(
  '/api/campaigns/analytics/unsubscribes',
  { params: normalizeAnalyticsParams(params), loading: models.campaigns },
);

export const getCampaignLinkCounts = async (params) => http.get(
  '/api/campaigns/analytics/links',
  { params: normalizeAnalyticsParams(params), loading: models.campaigns },
);

export const getCampaignRawViewCounts = async (params) => http.get(
  '/api/campaigns/analytics/views_raw',
  { params: normalizeAnalyticsParams(params), loading: models.campaigns },
);

export const getCampaignRawViewTotalCounts = async (params) => http.get(
  '/api/campaigns/analytics/views_raw_total',
  { params: normalizeAnalyticsParams(params), loading: models.campaigns },
);

export const getCampaignUniqueRawViewCounts = async (params) => http.get(
  '/api/campaigns/analytics/views_unique_raw',
  { params: normalizeAnalyticsParams(params), loading: models.campaigns },
);

export const getCampaignUniqueRawViewTotalCounts = async (params) => http.get(
  '/api/campaigns/analytics/views_unique_raw_total',
  { params: normalizeAnalyticsParams(params), loading: models.campaigns },
);

export const getCampaignSuspectedViewCounts = async (params) => http.get(
  '/api/campaigns/analytics/views_suspected',
  { params: normalizeAnalyticsParams(params), loading: models.campaigns },
);

export const getCampaignUniqueSuspectedViewCounts = async (params) => http.get(
  '/api/campaigns/analytics/views_unique_suspected',
  { params: normalizeAnalyticsParams(params), loading: models.campaigns },
);

export const getCampaignUniqueSuspectedViewTotalCounts = async (params) => http.get(
  '/api/campaigns/analytics/views_unique_suspected_total',
  { params: normalizeAnalyticsParams(params), loading: models.campaigns },
);

export const convertCampaignContent = async (data) => http.post(
  `/api/campaigns/${data.id}/content`,
  data,
  { loading: models.campaigns },
);

export const testCampaign = async (data) => http.post(
  `/api/campaigns/${data.id}/test`,
  data,
  { loading: models.campaigns },
);

export const updateCampaign = async (idOrRecordId, data) => http.put(
  `/api/campaigns/${idOrRecordId}`,
  data,
  { loading: models.campaigns },
);

export const changeCampaignStatus = async (idOrRecordId, status) => http.put(
  `/api/campaigns/${idOrRecordId}/status`,
  { status },

  { loading: models.campaigns },
);

export const updateCampaignArchive = async (idOrRecordId, data) => http.put(
  `/api/campaigns/${idOrRecordId}/archive`,
  data,
  { loading: models.campaigns },
);

export const deleteCampaign = async (idOrRecordId) => http.delete(
  `/api/campaigns/${idOrRecordId}`,
  { loading: models.campaigns },
);

export const deleteCampaigns = (params) => http.delete(
  '/api/campaigns',
  { params, loading: models.campaigns },
);

// Media.
export const getMedia = async (params) => http.get(
  '/api/media',
  { params, loading: models.media, store: models.media },
);

export const uploadMedia = (data) => http.post(
  '/api/media',
  data,
  { loading: models.media },
);

export const deleteMedia = (id) => http.delete(
  `/api/media/${id}`,
  { loading: models.media },
);

export const listPictures = async ({ page = 1, perPage = 20, query = '' } = {}) => {
  const options = {
    sort: '-created',
  };

  if (query && query.trim()) {
    options.filter = pb.filter('original_name ~ {:query} || file ~ {:query}', {
      query: query.trim(),
    });
  }

  return pb.collection('pictures').getList(page, perPage, options);
};

export const uploadPicture = async (file) => {
  const data = new FormData();
  data.set('file', file);
  data.set('original_name', file.name);
  data.set('content_type', file.type || '');
  return pb.collection('pictures').create(data);
};

export const deletePicture = async (id) => pb.collection('pictures').delete(id);

// Templates.
export const createTemplate = async (data) => http.post(
  '/api/templates',
  data,
  { loading: models.templates },
);

export const getTemplates = async () => http.get(
  '/api/templates',
  { loading: models.templates, store: models.templates },
);

export const getTemplate = async (id) => http.get(
  `/api/templates/${id}`,
  { loading: models.templates },
);

export const updateTemplate = async (data) => http.put(
  `/api/templates/${data.id}`,
  data,
  { loading: models.templates },
);

export const makeTemplateDefault = async (id) => http.put(
  `/api/templates/${id}/default`,
  {},
  { loading: models.templates },
);

export const deleteTemplate = async (id) => http.delete(
  `/api/templates/${id}`,
  { loading: models.templates },
);

// Settings.
export const getServerConfig = async () => http.get(
  '/api/config',
  { loading: models.serverConfig, store: models.serverConfig, camelCase: false },
);

export const getSettings = async () => http.get(
  '/api/settings',
  { loading: models.settings, store: models.settings, camelCase: false },
);

export const updateSettings = async (data) => http.put(
  '/api/settings',
  data,
  { loading: models.settings },
);

export const updateSettingsByKey = async (key, data) => http.put(
  `/api/settings/${key}`,
  data,
  { loading: models.settings },
);

export const testSMTP = async (data) => http.post(
  '/api/settings/smtp/test',
  data,
  { loading: models.settings, disableToast: true },
);

export const getLogs = async () => http.get(
  '/api/logs',
  { loading: models.logs, camelCase: false },
);

export const getLang = async (lang) => http.get(
  `/api/lang/${lang}`,
  { loading: models.lang, camelCase: false },
);

export const logout = async () => {
  pb.authStore.clear();
};

export const deleteGCCampaignAnalytics = async (typ, beforeDate) => http.delete(
  `/api/maintenance/analytics/${typ}`,
  {
    loading: models.maintenance,
    params: { before_date: toUTCISOString(beforeDate) || beforeDate },
  },
);

export const deleteGCSubscribers = async (typ) => http.delete(
  `/api/maintenance/subscribers/${typ}`,
  { loading: models.maintenance },
);

export const deleteGCSubscriptions = async (beforeDate) => http.delete(
  '/api/maintenance/subscriptions/unconfirmed',
  {
    loading: models.maintenance,
    params: { before_date: toUTCISOString(beforeDate) || beforeDate },
  },
);

// Users.
export const getUsers = () => http.get(
  '/api/users',
  {
    loading: models.users,
    store: models.users,
  },
);

export const queryUsers = () => http.get(
  '/api/users',
  {
    loading: models.users,
    store: models.users,
  },
);

export const getUser = async (idOrRecordId) => http.get(
  `/api/users/${idOrRecordId}`,
  { loading: models.users },
);

export const createUser = (data) => http.post(
  '/api/users',
  data,
  { loading: models.users },
);

export const updateUser = (data) => http.put(
  `/api/users/${data.id}`,
  data,
  { loading: models.users },
);

export const deleteUser = (idOrRecordId) => http.delete(
  `/api/users/${idOrRecordId}`,
  { loading: models.users },
);

export const getUserProfile = () => Promise.resolve(getStoredUserProfile());

export const refreshUserProfile = () => http.get(
  '/api/profile',
  { loading: models.users, store: models.profile },
).then((profile) => syncAuthProfile(profile));

export const updateUserProfile = (data) => http.put(
  '/api/profile',
  data,
  { loading: models.users, store: models.profile },
).then((profile) => syncAuthProfile(profile));

export const getUserRoles = async () => http.get(
  '/api/roles/users',
  { loading: models.userRoles, store: models.userRoles },
);

export const getListRoles = async () => http.get(
  '/api/roles/lists',
  { loading: models.listRoles, store: models.listRoles },
);

export const createUserRole = (data) => http.post(
  '/api/roles/users',
  data,
  { loading: models.userRoles },
);

export const createListRole = (data) => http.post(
  '/api/roles/lists',
  data,
  { loading: models.listRoles },
);

export const updateUserRole = (data) => http.put(
  `/api/roles/users/${data.id}`,
  data,
  { loading: models.userRoles },
);

export const updateListRole = (data) => http.put(
  `/api/roles/lists/${data.id}`,
  data,
  { loading: models.userRoles },
);

export const deleteRole = (id) => http.delete(
  `/api/roles/${id}`,
  { loading: models.userRoles },
);

// TOTP 2FA APIs
export const getTOTPQR = (id) => http.get(
  `/api/users/${id}/twofa/totp`,
  { camelCase: true },
);

export const enableTOTP = (id, data) => http.put(
  `/api/users/${id}/twofa`,
  data,
);

export const disableTOTP = (id, data) => http.delete(
  `/api/users/${id}/twofa`,
  { data },
);
