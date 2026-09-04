import { createStore } from 'vuex';
import { models } from '../constants';
import aiCampaignBuilderThreads from './modules/aiCampaignBuilderThreads';

export default createStore({
  modules: {
    aiCampaignBuilderThreads,
  },

  state: {
    ...Object.keys(models).reduce((obj, cur) => ({
      ...obj,
      [cur]: cur === models.profile ? {} : [],
    }), {}),
    loading: Object.keys(models).reduce((obj, cur) => ({ ...obj, [cur]: false }), {}),
  },

  mutations: {
    setModelResponse(state, { model, data }) {
      state[model] = data;
    },

    setLoading(state, { model, status }) {
      state.loading[model] = status;
    },
  },

  getters: {
    [models.lists]: (state) => state[models.lists],
    [models.subscribers]: (state) => state[models.subscribers],
    [models.txMessages]: (state) => state[models.txMessages],
    [models.campaigns]: (state) => state[models.campaigns],
    [models.media]: (state) => state[models.media],
    [models.templates]: (state) => state[models.templates],
    [models.users]: (state) => state[models.users],
    [models.profile]: (state) => state[models.profile],
    [models.userRoles]: (state) => state[models.userRoles],
    [models.listRoles]: (state) => state[models.listRoles],
    [models.settings]: (state) => state[models.settings],
    [models.serverConfig]: (state) => state[models.serverConfig],
    [models.logs]: (state) => state[models.logs],
  },
});
