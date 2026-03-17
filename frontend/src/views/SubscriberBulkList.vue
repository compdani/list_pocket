<template>
  <form @submit.prevent="onSubmit">
    <div class="admin-dialog-card modal-card">
      <header class="admin-dialog-head modal-card-head">
        <h4 class="title is-size-5">
          {{ $t('subscribers.manageLists') }}
        </h4>
      </header>

      <section expanded class="admin-dialog-body modal-card-body">
        <b-field label="Action">
          <div>
            <b-radio v-model="form.action" name="action" native-value="add" data-cy="check-list-add">
              {{ $t('globals.buttons.add') }}
            </b-radio>
            <b-radio v-model="form.action" name="action" native-value="remove" data-cy="check-list-remove">
              {{ $t('globals.buttons.remove') }}
            </b-radio>
            <b-radio v-model="form.action" name="action" native-value="unsubscribe" data-cy="check-list-unsubscribe">
              {{ $t('subscribers.markUnsubscribed') }}
            </b-radio>
          </div>
        </b-field>

        <list-selector label="Target lists" placeholder="Lists to apply to" v-model="form.lists" :selected="form.lists"
          :all="lists.results" />

        <b-field :message="$t('subscribers.preconfirmHelp')">
          <b-checkbox v-model="form.preconfirm" data-cy="preconfirm" :native-value="true" :disabled="!hasOptinList">
            {{ $t('subscribers.preconfirm') }}
          </b-checkbox>
        </b-field>
      </section>

      <footer class="admin-dialog-foot modal-card-foot has-text-right">
        <b-button @click="$emit('close')">
          {{ $t('globals.buttons.close') }}
        </b-button>
        <b-button native-type="submit" type="is-primary" :disabled="form.lists.length === 0">
          {{ $t('globals.buttons.save') }}
        </b-button>
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

  props: {
    numSubscribers: { type: Number, default: 0 },
  },

  data() {
    return {
      // Binds form input values.
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
</style>
