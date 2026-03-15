import { ToastProgrammatic as Toast } from 'buefy';
import PocketBase from 'pocketbase';
import qs from 'qs';
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

  const requestURL = url.startsWith('/api/')
    ? `/mailapi/${url.slice('/api/'.length)}`
    : url;

  try {
    const requestConfig = {
      method,
      query: config.params,
      paramsSerializer: (params) => qs.stringify(params, { arrayFormat: 'repeat' }),
    };

    if (data !== undefined) {
      requestConfig.body = data;
    }

    const response = await pb.send(requestURL, requestConfig);
    return transformResponse(response, config);
  } catch (err) {
    const msg = getErrorMessage(err);

    if (!config.disableToast) {
      Toast.open({
        message: msg,
        type: 'is-danger',
        queue: false,
        position: 'is-top',
        pauseOnHover: true,
      });
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

export const getList = async (id) => http.get(
  `/api/lists/${id}`,
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

export const deleteList = (id) => http.delete(
  `/api/lists/${id}`,
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

export const getSubscriber = async (id) => http.get(
  `/api/subscribers/${id}`,
  { loading: models.subscribers },
);

export const getSubscriberActivity = async (id) => http.get(
  `/api/subscribers/${id}/activity`,
  { loading: models.subscribers },
);

export const getSubscriberBounces = async (id) => http.get(
  `/api/subscribers/${id}/bounces`,
  { loading: models.bounces },
);

export const deleteSubscriberBounces = async (id) => http.delete(
  `/api/subscribers/${id}/bounces`,
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

export const sendSubscriberOptin = (id) => http.post(
  `/api/subscribers/${id}/optin`,
  {},
  { loading: models.subscribers },
);

export const deleteSubscriber = (id) => http.delete(
  `/api/subscribers/${id}`,
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

export const getCampaign = async (id) => http.get(`/api/campaigns/${id}`, {
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
  { params, loading: models.campaigns },
);

export const getCampaignClickCounts = async (params) => http.get(
  '/api/campaigns/analytics/clicks',
  { params, loading: models.campaigns },
);

export const getCampaignBounceCounts = async (params) => http.get(
  '/api/campaigns/analytics/bounces',
  { params, loading: models.campaigns },
);

export const getCampaignLinkCounts = async (params) => http.get(
  '/api/campaigns/analytics/links',
  { params, loading: models.campaigns },
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

export const updateCampaign = async (id, data) => http.put(
  `/api/campaigns/${id}`,
  data,
  { loading: models.campaigns },
);

export const changeCampaignStatus = async (id, status) => http.put(
  `/api/campaigns/${id}/status`,
  { status },

  { loading: models.campaigns },
);

export const updateCampaignArchive = async (id, data) => http.put(
  `/api/campaigns/${id}/archive`,
  data,
  { loading: models.campaigns },
);

export const deleteCampaign = async (id) => http.delete(
  `/api/campaigns/${id}`,
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
  { loading: models.maintenance, params: { before_date: beforeDate } },
);

export const deleteGCSubscribers = async (typ) => http.delete(
  `/api/maintenance/subscribers/${typ}`,
  { loading: models.maintenance },
);

export const deleteGCSubscriptions = async (beforeDate) => http.delete(
  '/api/maintenance/subscriptions/unconfirmed',
  { loading: models.maintenance, params: { before_date: beforeDate } },
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

export const getUser = async (id) => http.get(
  `/api/users/${id}`,
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

export const deleteUser = (id) => http.delete(
  `/api/users/${id}`,
  { loading: models.users },
);

export const getUserProfile = () => http.get(
  '/api/profile',
  { loading: models.users, store: models.profile },
);

export const updateUserProfile = (data) => http.put(
  '/api/profile',
  data,
  { loading: models.users, store: models.profile },
);

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
