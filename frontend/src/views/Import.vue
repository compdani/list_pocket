<template>
  <section class="import import-page">
    <div class="import-header">
      <h1 class="text-h4 font-weight-bold">
        {{ $t('import.title') }}
      </h1>
    </div>

    <v-overlay
      :model-value="isLoading"
      class="align-center justify-center"
      contained
    >
      <v-progress-circular indeterminate color="primary" size="56" />
    </v-overlay>

    <section v-if="isFree()" class="wrap import-content">
      <v-form @submit.prevent="onUpload">
        <v-card variant="outlined">
          <v-card-text class="pa-6">
            <v-row>
              <v-col cols="12" md="4">
                <p class="text-subtitle-2 mb-2">{{ $t('import.mode') }}</p>
                <v-radio-group v-model="form.mode" hide-details>
                  <v-radio
                    :label="$t('import.subscribe')"
                    value="subscribe"
                    name="mode"
                    data-cy="check-subscribe"
                  />
                  <v-radio
                    :label="$t('import.blocklist')"
                    value="blocklist"
                    name="mode"
                    data-cy="check-blocklist"
                  />
                </v-radio-group>
              </v-col>

              <v-col cols="12" md="4">
                <p class="text-subtitle-2 mb-2">{{ $t('globals.fields.status') }}</p>
                <v-radio-group v-model="form.subStatus" hide-details>
                  <template v-if="form.mode === 'subscribe'">
                    <v-radio
                      :label="$t('subscribers.status.unconfirmed')"
                      value="unconfirmed"
                      name="subStatus"
                      data-cy="check-unconfirmed"
                    />
                    <v-radio
                      :label="$t('subscribers.status.confirmed')"
                      value="confirmed"
                      name="subStatus"
                      data-cy="check-confirmed"
                    />
                  </template>
                  <v-radio
                    v-else
                    :label="$t('subscribers.status.unsubscribed')"
                    value="unsubscribed"
                    name="subStatus"
                    data-cy="check-unsubscribed"
                  />
                </v-radio-group>
              </v-col>

              <v-col cols="12" md="4">
                <v-text-field
                  v-model="form.delim"
                  :label="$t('import.csvDelim')"
                  :hint="$t('import.csvDelimHelp')"
                  maxlength="1"
                  name="delim"
                  placeholder=","
                  persistent-hint
                  required
                  variant="outlined"
                  class="delimiter"
                />
              </v-col>
            </v-row>

            <v-row v-if="form.mode === 'subscribe'" class="mt-2">
              <v-col cols="12" md="4">
                <v-switch
                  v-model="form.overwriteUserInfo"
                  :label="$t('import.overwriteUserInfo')"
                  :hint="$t('import.overwriteUserInfoHelp')"
                  name="overwriteUserInfo"
                  persistent-hint
                  color="primary"
                  data-cy="overwrite-user-info"
                />
              </v-col>

              <v-col cols="12" md="8">
                <v-switch
                  v-model="form.overwriteSubStatus"
                  :label="$t('import.overwriteSubStatus')"
                  :hint="$t('import.overwriteSubStatusHelp')"
                  name="overwriteSubStatus"
                  persistent-hint
                  color="primary"
                  data-cy="overwrite-sub-status"
                />
              </v-col>
            </v-row>

            <div v-if="form.mode === 'subscribe'" class="mt-4">
              <list-selector
                :label="$t('globals.terms.lists')"
                :placeholder="$t('import.listSubHelp')"
                :message="$t('import.listSubHelp')"
                v-model="form.lists"
                :selected="form.lists"
                :all="lists.results"
              />
            </div>

            <v-divider class="my-6" />

            <v-file-input
              v-model="form.file"
              :label="$t('import.csvFile')"
              :hint="$t('import.csvFileHelp')"
              accept=".csv,.zip"
              clearable
              persistent-hint
              prepend-icon="mdi-file-upload-outline"
              variant="outlined"
              show-size
            />

            <v-chip
              v-if="selectedFile"
              class="my-3"
              size="small"
              closable
              @click:close="clearFile"
            >
              {{ selectedFile.name }}
            </v-chip>

            <div class="mt-2">
              <v-btn
                type="submit"
                color="primary"
                :disabled="!selectedFile || (form.mode === 'subscribe' && form.lists.length === 0)"
                :loading="isProcessing"
              >
                {{ $t('import.upload') }}
              </v-btn>
            </div>
          </v-card-text>
        </v-card>
      </v-form>

      <div class="import-help mt-8">
        <h2 class="text-h6 mb-2">
          {{ $t('import.instructions') }}
        </h2>
        <p class="text-body-2 mb-4">{{ $t('import.instructionsHelp') }}</p>

        <blockquote class="csv-example mb-4">
          <code class="csv-headers">
            <span>email,</span>
            <span>phone,</span>
            <span>first_name,</span>
            <span>last_name,</span>
            <span>attributes</span>
          </code>
        </blockquote>

        <h3 class="text-subtitle-1 font-weight-bold mb-2">
          {{ $t('import.csvExample') }}
        </h3>

        <pre class="csv-example" v-text="example" />
      </div>
    </section>

    <section v-if="isRunning() || isDone()" class="wrap import-status-wrap">
      <v-card variant="outlined" class="import-status">
        <v-card-text class="pa-6 text-center">
          <v-progress-linear
            :model-value="progress"
            color="success"
            height="16"
            rounded
          >
            <template #default="{ value }">
              {{ Math.ceil(value || 0) }}%
            </template>
          </v-progress-linear>

          <p :class="['text-h6 text-capitalize mt-6', statusClass]">
            {{ status.status }}
          </p>

          <p class="mt-2">
            {{ $t('import.recordsCount', { num: status.imported, total: status.total }) }}
          </p>

          <v-btn
            class="mt-4"
            @click="stopImport"
            :loading="isProcessing"
            prepend-icon="mdi-file-upload-outline"
            color="primary"
          >
            {{ isDone() ? $t('import.importDone') : $t('import.stopImport') }}
          </v-btn>

          <div class="import-logs mt-6">
            <log-view :lines="logs" :loading="false" />
          </div>
        </v-card-text>
      </v-card>
    </section>
  </section>
</template>

<script>
import { mapState } from 'vuex';
import ListSelector from '../components/ListSelector.vue';
import LogView from '../components/LogView.vue';

export default {
  components: {
    ListSelector,
    LogView,
  },

  props: {
    data: { type: Object, default: () => { } },
    isEditing: { type: Boolean, default: false },
  },

  data() {
    return {
      form: {
        mode: 'subscribe',
        subStatus: 'unconfirmed',
        delim: ',',
        lists: [],
        overwriteUserInfo: false,
        overwriteSubStatus: false,
        file: null,
      },
      example: '',

      // Initial page load still has to wait for the status API to return
      // to either show the form or the status box.
      isLoading: true,

      isProcessing: false,
      status: { status: '' },
      logs: [],
      pollID: null,
    };
  },

  watch: {
    'form.mode': function formMode() {
      // Select the appropriate status radio whenever mode changes.
      this.$nextTick(() => {
        if (this.form.mode === 'subscribe') {
          this.form.subStatus = 'unconfirmed';
        } else {
          this.form.subStatus = 'unsubscribed';
        }
      });
    },
  },

  methods: {
    clearFile() {
      this.form.file = null;
    },

    // Returns true if we're free to do an upload.
    isFree() {
      if (this.status.status === 'none') {
        return true;
      }
      return false;
    },

    // Returns true if an import is running.
    isRunning() {
      if (this.status.status === 'importing'
        || this.status.status === 'stopping') {
        return true;
      }
      return false;
    },

    isSuccessful() {
      return this.status.status === 'finished';
    },

    isFailed() {
      return (
        this.status.status === 'stopped'
        || this.status.status === 'failed'
      );
    },

    // Returns true if an import has finished (failed or successful).
    isDone() {
      if (this.status.status === 'finished'
        || this.status.status === 'stopped'
        || this.status.status === 'failed'
      ) {
        return true;
      }
      return false;
    },

    pollStatus() {
      // Clear any running status polls.
      clearInterval(this.pollID);

      // Poll for the status as long as the import is running.
      this.pollID = setInterval(() => {
        this.$api.getImportStatus().then((data) => {
          this.isProcessing = false;
          this.isLoading = false;
          this.status = data;
          this.getLogs();

          if (!this.isRunning()) {
            clearInterval(this.pollID);
          }
        }, () => {
          this.isProcessing = false;
          this.isLoading = false;
          this.status = { status: 'none' };
          clearInterval(this.pollID);
        });
        return true;
      }, 250);
    },

    getLogs() {
      this.$api.getImportLogs().then((data) => {
        this.logs = data.split('\n').map((line) => line.replace(/\s+importer\.go:\d+:\s*/, ' *: '));
        this.$nextTick(() => {
          // vue.$refs doesn't work as the logs textarea is rendered dynamically.
          const ref = document.getElementById('import-log');
          if (ref) {
            ref.scrollTop = ref.scrollHeight;
          }
        });
      });
    },

    // Cancel a running import or clears a finished import.
    stopImport() {
      this.isProcessing = true;
      this.$api.stopImport().then(() => {
        this.pollStatus();
        this.form.file = null;
      });
    },

    renderExample() {
      const h = 'email,phone,first_name,last_name,attributes\n'
        + 'user1@mail.com,"+15555550123","User","One","{""age"": 42, ""planet"": ""Mars""}"\n'
        + 'user2@mail.com,"+15555550124","User","Two","{""age"": 24, ""job"": ""Time Traveller""}"';

      this.example = h;
    },

    resetForm() {
      this.form.mode = 'subscribe';
      this.form.overwriteUserInfo = false;
      this.form.overwriteSubStatus = false;
      this.form.file = null;
      this.form.lists = [];
      this.form.subStatus = 'unconfirmed';
      this.form.delim = ',';
    },

    onUpload() {
      if (this.form.mode === 'subscribe' && this.form.overwriteSubStatus) {
        this.$utils.confirm(this.$t('import.subscribeWarning'), this.onSubmit, this.resetForm);
        return;
      }

      this.onSubmit();
    },

    onSubmit() {
      this.isProcessing = true;

      const file = this.selectedFile;
      if (!file) {
        this.isProcessing = false;
        return;
      }

      // Prepare the upload payload.
      const params = new FormData();
      params.set('params', JSON.stringify({
        mode: this.form.mode,
        subscription_status: this.form.subStatus,
        delim: this.form.delim,
        lists: this.form.lists.map((l) => l.id),
        list_record_ids: this.form.lists
          .map((l) => l.record_id)
          .filter((id) => typeof id === 'string' && id.length > 0),
        overwrite_userinfo: this.form.overwriteUserInfo,
        overwrite_subscription_status: this.form.overwriteSubStatus,
      }));
      params.set('file', file);

      // Post.
      this.$api.importSubscribers(params).then(() => {
        // On file upload, show a confirmation.
        this.$utils.toast(this.$t('import.importStarted'));

        // Start polling status.
        this.pollStatus();
      }, () => {
        this.isProcessing = false;
        this.form.file = null;
      });
    },
  },

  computed: {
    ...mapState(['lists']),

    // Import progress bar value.
    progress() {
      if (!this.status || this.status.total <= 0) {
        return 0;
      }
      return Math.ceil((this.status.imported / this.status.total) * 100);
    },

    selectedFile() {
      if (Array.isArray(this.form.file)) {
        return this.form.file[0] || null;
      }
      return this.form.file;
    },

    statusClass() {
      if (this.status.status === 'finished') {
        return 'text-success';
      }

      if (this.status.status === 'failed' || this.status.status === 'stopped') {
        return 'text-error';
      }

      return '';
    },
  },

  mounted() {
    this.renderExample();
    this.pollStatus();

    const ids = this.$utils.parseQueryIDs(this.$route.query.list_id);
    const recordIDs = typeof this.$route.query.list_record_id === 'object'
      ? this.$route.query.list_record_id
      : (this.$route.query.list_record_id ? [this.$route.query.list_record_id] : []);
    if ((ids.length > 0 || recordIDs.length > 0) && this.lists.results) {
      this.$nextTick(() => {
        this.form.lists = this.lists.results.filter((l) => (
          ids.indexOf(l.id) > -1 || recordIDs.indexOf(l.record_id) > -1
        ));
      });
    }
  },

  beforeUnmount() {
    clearInterval(this.pollID);
  },
};
</script>

<style scoped>
.import-page {
  position: relative;
}

.import-header {
  margin-bottom: 24px;
}

.delimiter {
  max-width: 120px;
}

.import-help,
.import-status-wrap {
  margin-top: 24px;
}

.csv-headers {
  display: inline-flex;
  gap: 6px;
}

.csv-example {
  background: rgb(var(--v-theme-surface-variant));
  border-radius: 8px;
  font-family: SFMono-Regular, Consolas, "Liberation Mono", Menlo, monospace;
  font-size: 0.84rem;
  line-height: 1.55;
  overflow-x: auto;
  padding: 12px 14px;
}

.import-status :deep(.log-view .lines) {
  max-height: 240px;
  text-align: left;
}
</style>
