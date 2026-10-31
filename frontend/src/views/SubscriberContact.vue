<template>
  <section class="contact-page">
    <div v-if="isLoading && !subscriber.id" class="state-wrap">
      <v-progress-circular indeterminate color="primary" />
    </div>

    <div v-else-if="loadError" class="state-wrap">
      <v-alert type="error" variant="tonal" class="mb-3">
        {{ loadError }}
      </v-alert>
      <v-btn color="primary" variant="flat" @click="loadSubscriber">
        {{ $t('globals.buttons.retry') }}
      </v-btn>
    </div>

    <template v-else>
      <v-btn
        variant="text"
        size="small"
        prepend-icon="mdi-arrow-left"
        class="text-none px-1 mb-2"
        @click="goBack"
      >
        {{ $t('globals.terms.subscribers') }}
      </v-btn>

      <div class="contact-layout">
        <aside class="contact-card" data-cy="contact-summary">
          <div class="contact-identity">
            <contact-avatar :name="subscriber.name" :fallback="email" :size="64" />
            <h1 class="text-h6 mt-2 mb-0">{{ subscriber.name || email || subscriber.phone }}</h1>
            <p v-if="profile.company" class="text-body-2 text-medium-emphasis mb-1">{{ profile.company }}</p>
            <div class="d-flex flex-wrap justify-center ga-1 mt-1">
              <v-chip
                v-for="place in contactBoards"
                :key="place.board"
                size="small"
                label
                :color="place.color"
                :title="place.boardName"
              >
                {{ place.stageName }}
              </v-chip>
              <v-chip
                v-if="subscriber.status && subscriber.status !== 'enabled'"
                size="small"
                label
                color="error"
                variant="tonal"
              >
                {{ subscriber.status }}
              </v-chip>
            </div>
          </div>

          <div class="contact-quick">
            <v-btn
              v-if="canConverse"
              size="small"
              variant="tonal"
              class="text-none"
              prepend-icon="mdi-message-text-outline"
              data-cy="contact-message"
              @click="tab = 'conversation'"
            >
              {{ $t('crm.tab.conversation') }}
            </v-btn>
            <v-btn
              size="small"
              variant="tonal"
              class="text-none"
              prepend-icon="mdi-note-text-outline"
              @click="tab = 'notes'"
            >
              {{ $t('crm.tab.notes') }}
            </v-btn>
            <v-btn
              size="small"
              variant="tonal"
              class="text-none"
              prepend-icon="mdi-clipboard-text-outline"
              @click="tab = 'tasks'"
            >
              {{ $t('crm.tab.tasks') }}
            </v-btn>
            <v-btn
              v-if="canEnroll"
              size="small"
              variant="tonal"
              class="text-none"
              prepend-icon="mdi-source-branch"
              :disabled="subscriber.status === 'blocklisted'"
              data-cy="contact-add-to-workflow"
              @click="isEnrollVisible = true"
            >
              {{ $t('workflows.enroll.addToWorkflow') }}
            </v-btn>
          </div>
          <flow-enroll-dialog v-if="canEnroll" v-model="isEnrollVisible" :preset-subscribers="[subscriber]" />

          <dl class="contact-facts">
            <div v-if="email">
              <dt><v-icon icon="mdi-email-outline" size="16" /><span class="sr-only">{{ $t('crm.email') }}</span></dt>
              <dd><a :href="`mailto:${email}`">{{ email }}</a></dd>
            </div>
            <div v-if="subscriber.phone">
              <dt><v-icon icon="mdi-phone-outline" size="16" /><span class="sr-only">{{ $t('crm.phone') }}</span></dt>
              <dd>{{ subscriber.phone }}</dd>
            </div>
          </dl>

          <contact-crm-fields
            v-if="subscriber.id"
            :subscriber-id="recordId"
            @updated="profile = $event"
          />

          <contact-boards
            v-if="subscriber.id && canSeeBoards"
            :subscriber-id="recordId"
            class="mt-4"
            @updated="contactBoards = $event"
          />

          <div v-if="listNames.length" class="mt-4">
            <div class="text-caption text-medium-emphasis mb-1">{{ $t('crm.lists') }}</div>
            <div class="d-flex flex-wrap ga-1">
              <v-chip v-for="name in listNames" :key="name" size="x-small" label variant="outlined">{{ name }}</v-chip>
            </div>
          </div>
        </aside>

        <div class="contact-main">
          <v-tabs v-model="tab" density="comfortable" color="primary" show-arrows>
            <v-tab value="activity" class="text-none" prepend-icon="mdi-timeline-text-outline">{{ $t('crm.tab.activity') }}</v-tab>
            <v-tab v-if="canConverse" value="conversation" class="text-none" prepend-icon="mdi-forum-outline" data-cy="tab-conversation">{{ $t('crm.tab.conversation') }}</v-tab>
            <v-tab value="notes" class="text-none" prepend-icon="mdi-note-text-outline" data-cy="tab-notes">{{ $t('crm.tab.notes') }}</v-tab>
            <v-tab value="tasks" class="text-none" prepend-icon="mdi-clipboard-text-outline" data-cy="tab-tasks">{{ $t('crm.tab.tasks') }}</v-tab>
            <v-tab value="details" class="text-none" prepend-icon="mdi-account-outline" data-cy="tab-details">{{ $t('crm.tab.details') }}</v-tab>
          </v-tabs>
          <v-divider />

          <div v-if="subscriber.id" :class="['contact-panel', { 'contact-panel--thread': tab === 'conversation' }]">
            <subscriber-activity v-if="tab === 'activity'" :subscriber-id="subscriber.id" />
            <conversation-thread v-else-if="tab === 'conversation'" :conversation-key="recordId" />
            <contact-notes v-else-if="tab === 'notes'" :subscriber-id="recordId" />
            <contact-tasks v-else-if="tab === 'tasks'" :subscriber-id="recordId" />
            <subscriber-form
              v-else
              :data="subscriber"
              :is-editing="true"
              :is-page-mode="true"
              :hide-activity-tab="true"
              :close-on-save="false"
              :close-label="$t('globals.buttons.close')"
              @finished="loadSubscriber"
              @close="goBack"
            />
          </div>
        </div>
      </div>
    </template>
  </section>
</template>

<script>
import ContactAvatar from '../components/ContactAvatar.vue';
import ContactBoards from '../components/ContactBoards.vue';
import ContactCrmFields from '../components/ContactCrmFields.vue';
import ContactNotes from '../components/ContactNotes.vue';
import ContactTasks from '../components/ContactTasks.vue';
import ConversationThread from '../components/ConversationThread.vue';
import SubscriberActivity from '../components/SubscriberActivity.vue';
import FlowEnrollDialog from '../components/flow/FlowEnrollDialog.vue';
import SubscriberForm from './SubscriberForm.vue';
import { visibleEmail } from '../utils/crm.mjs';

const TABS = ['activity', 'conversation', 'notes', 'tasks', 'details'];

export default {
  name: 'SubscriberContact',
  components: {
    ContactAvatar,
    ContactBoards,
    ContactCrmFields,
    ContactNotes,
    ContactTasks,
    ConversationThread,
    FlowEnrollDialog,
    SubscriberActivity,
    SubscriberForm,
  },
  data() {
    return {
      isLoading: false,
      isEnrollVisible: false,
      loadError: '',
      subscriber: {
        lists: [],
        attribs: {},
      },
      profile: {},
      contactBoards: [],
    };
  },
  computed: {
    tab: {
      get() {
        const hash = String(this.$route.hash || '').replace('#', '');
        return TABS.includes(hash) ? hash : 'activity';
      },
      set(value) {
        this.$router.replace({ hash: value === 'activity' ? '' : `#${value}` });
      },
    },
    canConverse() {
      return this.$can('inbox:get');
    },
    // Choosing a workflow needs the list of them, which workflows:run alone can't read.
    canEnroll() {
      return this.$can('workflows:manage') || (this.$can('workflows:run') && this.$can('workflows:get'));
    },
    email() {
      return visibleEmail(this.subscriber.email);
    },
    // Conversations and CRM data are keyed by the stored record ID. Older links can
    // still use the numeric ID, so this comes from the loaded subscriber, not the URL.
    recordId() {
      return String(this.subscriber.id);
    },
    canSeeBoards() {
      return this.$can('boards:get');
    },
    listNames() {
      return (this.subscriber.lists || []).map((l) => l.name).filter(Boolean);
    },
  },
  methods: {
    async loadSubscriber() {
      const id = String(this.$route.params.id || '').trim();
      if (!id) {
        this.$router.replace({ name: 'subscribers' });
        return;
      }

      this.isLoading = true;
      this.loadError = '';
      try {
        const data = await this.$api.getSubscriber(id);
        this.subscriber = {
          ...data,
          lists: Array.isArray(data.lists) ? data.lists : [],
          attribs: data.attribs && typeof data.attribs === 'object' ? data.attribs : {},
        };
      } catch (e) {
        this.loadError = e?.message || this.$t('globals.messages.errorFetching');
      } finally {
        this.isLoading = false;
      }
    },

    goBack() {
      if (window.history.length > 1) {
        this.$router.back();
        return;
      }
      this.$router.push({ name: 'subscribers' });
    },
  },
  mounted() {
    this.loadSubscriber();
  },
  watch: {
    '$route.params.id'() {
      this.loadSubscriber();
    },
  },
};
</script>

<style scoped>
.contact-page {
  --contact-border: rgba(var(--v-border-color), var(--v-border-opacity));
}

.contact-layout {
  align-items: start;
  display: grid;
  gap: 16px;
  grid-template-columns: minmax(260px, 320px) minmax(0, 1fr);
}

.contact-card,
.contact-main {
  background: rgb(var(--v-theme-surface));
  border: 1px solid var(--contact-border);
  border-radius: 12px;
}

.contact-card {
  padding: 20px 16px;
}

.contact-identity {
  overflow-wrap: anywhere;
  text-align: center;
}

.contact-quick {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  justify-content: center;
  margin: 14px 0;
}

.contact-facts {
  border-top: 1px solid var(--contact-border);
  margin: 0 0 14px;
  padding-top: 12px;
}

.contact-facts > div {
  align-items: center;
  display: flex;
  font-size: 14px;
  gap: 8px;
  margin-bottom: 6px;
}

.contact-facts dd {
  margin: 0;
  min-width: 0;
  overflow-wrap: anywhere;
}

.sr-only {
  border: 0;
  clip: rect(0 0 0 0);
  height: 1px;
  overflow: hidden;
  position: absolute;
  width: 1px;
}

.contact-main {
  min-width: 0;
}

.contact-panel {
  padding: 16px;
}

.contact-panel--thread {
  display: flex;
  height: calc(100vh - 210px);
  min-height: 480px;
  padding: 0;
}

.state-wrap {
  align-items: center;
  display: flex;
  flex-direction: column;
  gap: 12px;
  justify-content: center;
  min-height: 240px;
}

@media (max-width: 960px) {
  .contact-layout {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
