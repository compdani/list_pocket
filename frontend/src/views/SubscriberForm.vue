<template>
  <form @submit.prevent="onSubmit">
    <div class="admin-dialog-card modal-card content">
      <header class="admin-dialog-head modal-card-head">
        <span v-if="isEditing" class="status-pill" :class="data.status">
          {{ $t(`subscribers.status.${data.status}`) }}
        </span>
        <h4 v-if="isEditing">
          {{ data.name }}
        </h4>
        <h4 v-else>
          {{ $t('subscribers.newSubscriber') }}
        </h4>

        <p v-if="isEditing" class="has-text-grey is-size-7">
          {{ $t('globals.fields.id') }}: <span data-cy="id"><copy-text :text="`${data.id}`" /></span>
          {{ $t('globals.fields.uuid') }}: <copy-text :text="data.uuid" />
        </p>
      </header>

      <section expanded class="admin-dialog-body modal-card-body">
        <div class="form-field">
          <label class="form-label" for="subscriber-email">{{ $t('subscribers.email') }}</label>
          <input
            id="subscriber-email"
            ref="focus"
            v-model="form.email"
            class="input"
            maxlength="200"
            name="email"
            :placeholder="$t('subscribers.email')"
            required
            type="email"
          >
        </div>

        <div class="columns">
          <div class="column is-8">
            <div class="form-field">
              <label class="form-label" for="subscriber-name">{{ $t('globals.fields.name') }}</label>
              <input
                id="subscriber-name"
                v-model="form.name"
                class="input"
                maxlength="200"
                name="name"
                :placeholder="$t('globals.fields.name')"
                type="text"
              >
            </div>
          </div>
          <div class="column is-4">
            <div class="form-field">
              <label class="form-label" for="subscriber-status">{{ $t('globals.fields.status') }}</label>
              <select
                id="subscriber-status"
                v-model="form.status"
                class="input"
                name="status"
                required
              >
                <option value="enabled">
                  {{ $t('subscribers.status.enabled') }}
                </option>
                <option value="blocklisted">
                  {{ $t('subscribers.status.blocklisted') }}
                </option>
              </select>
              <p class="form-help">{{ $t('subscribers.blocklistedHelp') }}</p>
            </div>
          </div>
        </div>

        <div class="settings-tabs">
          <button type="button" class="settings-tab" :class="{ 'is-active': activeTab === 'lists' }" @click="activeTab = 'lists'">
            {{ $t('globals.terms.lists') }}
          </button>
          <button
            type="button"
            class="settings-tab"
            :class="{ 'is-active': activeTab === 'subscriptions' }"
            :disabled="!data.lists || data.lists.length === 0"
            @click="activeTab = 'subscriptions'"
          >
            {{ `${$tc('globals.terms.subscriptions', 2)} (${data.lists ? data.lists.length : 0})` }}
          </button>
          <button
            type="button"
            class="settings-tab"
            :class="{ 'is-active': activeTab === 'bounces' }"
            :disabled="bounces.length === 0"
            @click="activeTab = 'bounces'"
          >
            {{ `${$t('globals.terms.bounces')} (${bounces.length})` }}
          </button>
          <button
            type="button"
            class="settings-tab"
            :class="{ 'is-active': activeTab === 'activity' }"
            :disabled="!isEditing"
            @click="activeTab = 'activity'"
          >
            {{ $t('subscribers.activity') }}
          </button>
        </div>

        <section v-if="activeTab === 'lists'">
          <div class="form-field">
            <label class="form-label" for="subscriber-lists">{{ $t('subscribers.lists') }}</label>
            <div class="select is-multiple is-fullwidth">
              <select
                id="subscriber-lists"
                :value="selectedListIds"
                :aria-label="$t('subscribers.lists')"
                class="multi-select"
                multiple
                size="6"
                @change="onListsChange($event)"
              >
                <option v-for="list in availableLists" :key="list.id" :value="String(list.id)">
                  {{ list.name }}
                </option>
              </select>
            </div>
            <p class="form-help">{{ $t('subscribers.listsHelp') }}</p>
          </div>
            <div class="columns">
              <div class="column is-7">
                <div class="form-field">
                  <label class="checkbox-row">
                    <input v-model="form.preconfirm" :disabled="!hasOptinList" type="checkbox">
                    <span>{{ $t('subscribers.preconfirm') }}</span>
                  </label>
                  <p class="form-help">{{ $t('subscribers.preconfirmHelp') }}</p>
                </div>
              </div>
              <div v-if="$can('subscribers:manage') && isEditing" class="column is-5 has-text-right">
                <a href="#" @click.prevent="sendOptinConfirmation" :class="{ 'is-disabled': !hasOptinList }">
                  <b-icon icon="email-outline" size="is-small" />
                  {{ $t('subscribers.sendOptinConfirm') }}</a>
              </div>
            </div>
        </section>

        <section v-if="activeTab === 'subscriptions' && data.lists && data.lists.length > 0" class="mt-5">
          <h5 class="mb-3">{{ `${$tc('globals.terms.subscriptions', 2)} (${data.lists.length})` }}</h5>
          <div class="table-wrap">
            <table class="dialog-table">
              <thead>
                <tr>
                  <th>{{ $tc('globals.terms.list', 1) }}</th>
                  <th>{{ $t('globals.fields.status') }}</th>
                  <th>{{ $t('globals.fields.createdAt') }}</th>
                  <th>{{ $t('globals.fields.updatedAt') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in data.lists" :key="item.id">
                  <td>
                    <router-link :to="`/lists/${item.id}`">
                      {{ item.name }}
                    </router-link>
                    <div class="subtle-row">
                      <span class="status-pill neutral">
                        {{ $t(`lists.optins.${item.optin}`) }}
                      </span>
                    </div>
                  </td>
                  <td>
                    <span class="status-pill neutral">
                      {{ $t(`subscribers.status.${item.subscriptionStatus}`) }}
                    </span>
                    <div v-if="item.optin === 'double' && item.subscriptionMeta.optinIp" class="subtle-row">
                      {{ item.subscriptionMeta.optinIp }}
                    </div>
                  </td>
                  <td>{{ $utils.niceDate(item.subscriptionCreatedAt, true) }}</td>
                  <td>{{ $utils.niceDate(item.subscriptionCreatedAt, true) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <section v-if="activeTab === 'bounces' && bounces.length > 0" class="bounces mt-5">
            <h5 class="mb-3">{{ `${$t('globals.terms.bounces')} (${bounces.length})` }}</h5>
            <a href="#" class="is-size-6 is-pulled-right" disabed="true" @click.prevent="deleteBounces"
              v-if="isBounceVisible">
              {{ $t('globals.buttons.delete') }}
            </a>

            <div class="table-wrap">
              <table class="dialog-table">
                <thead>
                  <tr>
                    <th>{{ $tc('globals.terms.campaign', 1) }}</th>
                    <th>{{ $t('globals.fields.createdAt') }}</th>
                    <th>{{ $t('globals.fields.type') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="bounce in bounces" :key="bounce.id">
                    <td>
                      <router-link
                        v-if="bounce.campaign"
                        :to="{ name: 'bounces', query: { campaign_id: bounce.campaign.id } }"
                      >
                        {{ bounce.campaign.name }}
                      </router-link>
                    </td>
                    <td>{{ $utils.niceDate(bounce.createdAt, true) }}</td>
                    <td>
                      <a href="#" @click.prevent="toggleMeta(bounce.id)">
                        {{ bounce.source }}
                      </a>
                      <pre v-if="visibleMeta[bounce.id]" class="meta-block">{{ bounce.meta }}</pre>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
        </section>

        <section v-if="activeTab === 'activity' && isEditing && data.id" class="activity mt-5">
          <h5 class="mb-3">{{ $t('subscribers.activity') }}</h5>
          <subscriber-activity :subscriber-id="data.id" />
        </section>

        <div class="form-field mt-6">
          <label class="form-label" for="subscriber-tags">{{ $t('globals.terms.tags') }}</label>
          <input
            id="subscriber-tags"
            :value="tagsInput"
            :aria-label="$t('globals.terms.tags')"
            class="input"
            :placeholder="$t('globals.terms.tags')"
            type="text"
            @input="tagsInput = $event.target.value"
          >
        </div>

        <div class="form-field mt-6">
          <label class="form-label" for="subscriber-attribs">{{ $t('globals.terms.attribs') }}</label>
          <textarea id="subscriber-attribs" v-model="form.strAttribs" class="input textarea-input" name="attribs" />
          <p class="form-help">{{ $t('subscribers.attribsHelp') }} {{ egAttribs }}</p>
          <a href="https://listmonk.app/docs/concepts" target="_blank" rel="noopener noreferrer" class="is-size-7">
            {{ $t('globals.buttons.learnMore') }}
          </a>
        </div>
      </section>
      <footer class="admin-dialog-foot modal-card-foot has-text-right">
        <button type="button" class="button secondary-button" @click="$emit('close')">
          {{ $t('globals.buttons.close') }}
        </button>
        <button
          v-if="$can('subscribers:manage')"
          class="button primary-button"
          :disabled="loading.subscribers"
          type="submit"
        >
          {{ $t('globals.buttons.save') }}
        </button>
      </footer>
    </div>
  </form>
</template>

<script>
import { mapState } from 'vuex';
import CopyText from '../components/CopyText.vue';
import SubscriberActivity from '../components/SubscriberActivity.vue';

export default {
  components: {
    CopyText,
    SubscriberActivity,
  },

  props: {
    data: {
      type: Object,
      default: () => ({ lists: [] }),
    },
    isEditing: Boolean,
  },

  data() {
    return {
      // Binds form input values. This is populated by subscriber props passed
      // from the parent component in mounted().
      form: {
        lists: [],
        strAttribs: '{}',
        status: 'enabled',
        preconfirm: false,
        tags: [],
      },
      isBounceVisible: false,
      bounces: [],
      visibleMeta: {},
      activeTab: 'lists',

      egAttribs: '{"job": "developer", "location": "Mars", "has_rocket": true}',
    };
  },

  methods: {
    initForm() {
      const baseForm = {
        id: null,
        email: '',
        name: '',
        lists: [],
        strAttribs: '{}',
        status: 'enabled',
        preconfirm: false,
        tags: [],
      };

      if (!this.$props.isEditing) {
        this.form = baseForm;
        this.bounces = [];
        this.visibleMeta = {};
        return;
      }

      this.form = {
        ...baseForm,
        ...this.$props.data,
        email: this.$props.data.email || '',
        name: this.$props.data.name || '',
        status: this.$props.data.status || 'enabled',
        lists: Array.isArray(this.$props.data.lists) ? [...this.$props.data.lists] : [],
        strAttribs: JSON.stringify(this.$props.data.attribs || {}, null, 4),
        tags: Array.isArray((this.$props.data.attribs || {}).tags) ? [...this.$props.data.attribs.tags] : [],
      };
      this.bounces = [];
      this.visibleMeta = {};
      this.activeTab = 'lists';

      if (this.form.id) {
        this.getBounces();
      }
    },

    toggleBounces() {
      this.isBounceVisible = !this.isBounceVisible;
    },

    toggleMeta(id) {
      this.visibleMeta = {
        ...this.visibleMeta,
        [id]: !this.visibleMeta[id],
      };
    },

    onListsChange(event) {
      const selectedIDs = Array.from(event.target.selectedOptions || [], (option) => Number(option.value));
      const listMap = new Map(this.availableLists.map((list) => [Number(list.id), list]));
      this.form.lists = selectedIDs
        .map((id) => listMap.get(id))
        .filter(Boolean);
    },

    deleteBounces(sub) {
      this.$utils.confirm(
        null,
        () => {
          this.$api.deleteSubscriberBounces(this.form.id).then(() => {
            this.getBounces();
            this.$utils.toast(this.$t('globals.messages.deleted', { name: sub.name }));
          });
        },
      );
    },

    getBounces() {
      this.$api.getSubscriberBounces(this.form.id).then((data) => {
        this.bounces = data;
      });
    },

    onSubmit() {
      if (this.isEditing) {
        this.updateSubscriber();
        return;
      }

      this.createSubscriber();
    },

    createSubscriber() {
      let attribs = {};
      if (this.form.strAttribs) {
        attribs = this.validateAttribs(this.form.strAttribs);
        if (!attribs) {
          return;
        }
      }

      if (this.form.tags.length > 0) {
        attribs.tags = [...this.form.tags];
      } else {
        delete attribs.tags;
      }

      const data = {
        email: this.form.email,
        name: this.form.name,
        status: this.form.status,
        attribs,
        preconfirm_subscriptions: this.form.preconfirm,

        // List IDs.
        lists: this.form.lists.map((l) => l.id),
      };

      this.$api.createSubscriber(data).then((d) => {
        this.$emit('finished');
        this.$emit('close');
        this.$utils.toast(this.$t('globals.messages.created', { name: d.name }));
      });
    },

    updateSubscriber() {
      let attribs = {};
      if (this.form.strAttribs) {
        attribs = this.validateAttribs(this.form.strAttribs);
        if (!attribs) {
          return;
        }
      }

      if (this.form.tags.length > 0) {
        attribs.tags = [...this.form.tags];
      } else {
        delete attribs.tags;
      }

      const data = {
        id: this.form.id,
        email: this.form.email,
        name: this.form.name,
        status: this.form.status,
        preconfirm_subscriptions: this.form.preconfirm,
        attribs,

        // List IDs.
        lists: this.form.lists.map((l) => l.id),
      };

      this.$api.updateSubscriber(data).then((d) => {
        this.$emit('finished');
        this.$emit('close');
        this.$utils.toast(this.$t('globals.messages.updated', { name: d.name }));
      });
    },

    sendOptinConfirmation() {
      this.$api.sendSubscriberOptin(this.form.id).then(() => {
        this.$utils.toast(this.$t('subscribers.sentOptinConfirm'));
      });
    },

    validateAttribs(str) {
      // Parse and validate attributes JSON.
      let attribs = {};
      try {
        attribs = JSON.parse(str);
      } catch (e) {
        this.$utils.toast(
          `${this.$t('subscribers.invalidJSON')}: ${e.toString()}`,
          'is-danger',

          3000,
        );
        return null;
      }
      if (attribs instanceof Array) {
        this.$utils.toast('Attributes should be a map {} and not an array []', 'is-danger', 3000);
        return null;
      }

      return attribs;
    },
  },

  computed: {
    ...mapState(['lists', 'loading']),

    availableLists() {
      return Array.isArray(this.lists && this.lists.results) ? this.lists.results : [];
    },

    selectedListIds() {
      return Array.isArray(this.form.lists) ? this.form.lists.map((list) => String(list.id)) : [];
    },

    tagsInput: {
      get() {
        return Array.isArray(this.form.tags) ? this.form.tags.join(', ') : '';
      },
      set(value) {
        this.form.tags = value
          .split(',')
          .map((tag) => tag.trim())
          .filter(Boolean)
          .filter((tag, index, all) => all.indexOf(tag) === index);
      },
    },

    hasOptinList() {
      return this.form.lists.some((l) => l.optin === 'double');
    },
  },

  watch: {
    data: {
      deep: true,
      handler() {
        this.initForm();
      },
    },

    isEditing() {
      this.initForm();
    },
  },

  mounted() {
    this.initForm();

    this.$nextTick(() => {
      if (this.$refs.focus && typeof this.$refs.focus.focus === 'function') {
        this.$refs.focus.focus();
      }
    });
  },
};
</script>

<style scoped>
.form-field {
  margin-bottom: 18px;
}

.form-label {
  display: block;
  font-size: 0.95rem;
  font-weight: 600;
  margin-bottom: 8px;
}

.form-help {
  color: #667085;
  font-size: 0.9rem;
  margin-top: 8px;
}

.textarea-input {
  min-height: 120px;
  resize: vertical;
}

.multi-select {
  min-height: 180px;
  padding: 10px;
  width: 100%;
}

.checkbox-row {
  align-items: center;
  display: flex;
  gap: 10px;
}

.dialog-table {
  border-collapse: collapse;
  width: 100%;
}

.dialog-table th,
.dialog-table td {
  border-bottom: 1px solid #eaecf0;
  padding: 12px 10px;
  text-align: left;
  vertical-align: top;
}

.subtle-row {
  color: #667085;
  font-size: 0.85rem;
  margin-top: 6px;
}

.meta-block {
  background: #f8fafc;
  border: 1px solid #e4e7ec;
  border-radius: 8px;
  margin-top: 8px;
  padding: 10px;
  white-space: pre-wrap;
}

.status-pill {
  background: #eff6ff;
  border-radius: 999px;
  color: #0f5bd8;
  display: inline-block;
  float: right;
  font-size: 0.85rem;
  font-weight: 600;
  padding: 6px 10px;
}

.status-pill.neutral {
  float: none;
}

.button {
  border: 1px solid #d0d5dd;
  border-radius: 10px;
  cursor: pointer;
  font-weight: 600;
  min-height: 44px;
  padding: 0 16px;
}

.primary-button {
  background: #0f5bd8;
  border-color: #0f5bd8;
  color: #fff;
}

.primary-button:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.secondary-button {
  background: #fff;
  color: #1d2939;
}

.admin-dialog-card {
  background: #fff;
  border: 1px solid #ddd;
  border-radius: 12px;
  box-shadow: 0 18px 45px rgba(15, 23, 42, 0.18);
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 48px);
  overflow: hidden;
  width: min(920px, calc(100vw - 32px));
}

.admin-dialog-head {
  border-bottom: 0;
  display: block;
  padding: 20px 20px 0;
}

.admin-dialog-body {
  overflow: auto;
  padding: 24px 20px;
}

.admin-dialog-foot {
  background: #fff;
  border-top: 0;
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  padding: 0 20px 20px;
}

.admin-dialog-foot button {
  flex: 1 1 0;
}

.settings-tabs {
  border-bottom: 1px solid #d8dfec;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 20px;
}

.settings-tab {
  background: #fff;
  border: 1px solid #d8dfec;
  border-bottom: 0;
  border-radius: 12px 12px 0 0;
  color: #667085;
  cursor: pointer;
  font-size: 0.95rem;
  padding: 10px 16px;
}

.settings-tab.is-active {
  background: #f8fbff;
  color: #0f5bd8;
  font-weight: 600;
}
</style>
