<template>
  <form @submit.prevent="onSubmit">
    <div class="admin-dialog-card modal-card">
      <header class="admin-dialog-head modal-card-head">
        <h4 class="dialog-title">
          {{ $t('subscribers.manageLists') }}
        </h4>
      </header>

      <section class="admin-dialog-body modal-card-body">
        <div class="field-block">
          <div class="field-label">Action</div>
          <v-btn-toggle
            v-model="form.action"
            mandatory
            divided
            variant="outlined"
            density="comfortable"
            class="action-toggle"
          >
            <v-btn value="add" data-cy="check-list-add">
              {{ $t('globals.buttons.add') }}
            </v-btn>
            <v-btn value="remove" data-cy="check-list-remove">
              {{ $t('globals.buttons.remove') }}
            </v-btn>
            <v-btn value="unsubscribe" data-cy="check-list-unsubscribe">
              {{ $t('subscribers.markUnsubscribed') }}
            </v-btn>
          </v-btn-toggle>
        </div>

        <list-selector
          class="mt-4"
          label="Target lists"
          placeholder="Lists to apply to"
          v-model="form.lists"
          :selected="form.lists"
          :all="lists.results"
        />

        <div class="preconfirm-block mt-4">
          <v-checkbox
            v-model="form.preconfirm"
            data-cy="preconfirm"
            :disabled="!hasOptinList"
            :label="$t('subscribers.preconfirm')"
            density="comfortable"
            hide-details
          />
          <p class="form-help mt-1">{{ $t('subscribers.preconfirmHelp') }}</p>
        </div>
      </section>

      <footer class="admin-dialog-foot modal-card-foot">
        <v-btn type="button" variant="outlined" class="dialog-action" @click="$emit('close')">
          {{ $t('globals.buttons.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          class="dialog-action"
          type="submit"
          data-cy="btn-save"
          :disabled="form.lists.length === 0"
        >
          {{ $t('globals.buttons.save') }}
        </v-btn>
      </footer>
    </div>
  </form>
</template>

<script>
import { mapState } from 'vuex';
import ListSelector from '../components/ListSelector.vue';

export default {
  components: {
    ListSelector,
  },

  emits: ['finished', 'close'],

  props: {
    numSubscribers: { type: Number, default: 0 },
  },

  data() {
    return {
      form: {
        action: 'add',
        lists: [],
        preconfirm: false,
      },
    };
  },

  methods: {
    onSubmit() {
      this.$emit('finished', this.form.action, this.form.preconfirm, this.form.lists);
      this.$emit('close');
    },
  },

  computed: {
    ...mapState(['lists', 'loading']),

    hasOptinList() {
      return this.form.lists.some((l) => l.optin === 'double');
    },
  },
};
</script>

<style scoped>
.admin-dialog-card {
  background: #fff;
  border: 1px solid #ddd;
  border-radius: 12px;
  box-shadow: 0 18px 45px rgba(15, 23, 42, 0.18);
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 48px);
  overflow: hidden;
  width: min(560px, calc(100vw - 32px));
}

.admin-dialog-head {
  border-bottom: 0;
  display: block;
  padding: 20px 20px 0;
}

.dialog-title {
  font-size: 1.15rem;
  font-weight: 700;
  line-height: 1.3;
  margin: 0;
}

.admin-dialog-body {
  overflow: auto;
  padding: 24px 20px;
}

.field-block {
  margin-bottom: 4px;
}

.field-label {
  color: rgba(0, 0, 0, 0.6);
  font-size: 0.875rem;
  font-weight: 500;
  margin-bottom: 8px;
}

.action-toggle {
  display: flex;
  flex-wrap: wrap;
  width: 100%;
}

.action-toggle :deep(.v-btn) {
  flex: 1 1 auto;
  text-transform: none;
}

.form-help {
  color: #64748b;
  font-size: 0.8rem;
  line-height: 1.4;
  margin: 0;
}

.admin-dialog-foot {
  background: #fff;
  border-top: 0;
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  padding: 0 20px 20px;
}

.dialog-action {
  flex: 1 1 0;
}
</style>
