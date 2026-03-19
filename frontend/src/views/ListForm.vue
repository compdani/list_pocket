<template>
  <v-form @submit.prevent="onSubmit">
    <div class="admin-dialog-card modal-card content">
      <header class="admin-dialog-head modal-card-head">
        <div class="dialog-meta-row">
          <p v-if="isEditing" class="entity-meta has-text-grey-light is-size-7">
            {{ $t('globals.fields.id') }}: <copy-text :text="`${data.id}`" />
            Record ID: <copy-text :text="data.record_id" />
            {{ $t('globals.fields.uuid') }}: <copy-text :text="data.uuid" />
          </p>
          <span v-if="isEditing" class="type-pill" :class="data.type">
            {{ $t(`lists.types.${data.type}`) }}
          </span>
        </div>

        <h4 v-if="isEditing" class="dialog-title">
          {{ data.name }}
        </h4>
        <h4 v-else class="dialog-title">
          {{ $t('lists.newList') }}
        </h4>
      </header>

      <section class="admin-dialog-body modal-card-body">
        <v-text-field
          ref="focus"
          v-model="form.name"
          :label="$t('globals.fields.name')"
          maxlength="200"
          name="name"
          :placeholder="$t('globals.fields.name')"
          required
          type="text"
          variant="outlined"
          density="comfortable"
          class="mb-2"
        />

        <v-row class="mb-1">
          <v-col cols="12" md="6">
            <v-select
              v-model="form.type"
              :items="listTypeOptions"
              item-title="title"
              item-value="value"
              :label="$t('lists.type')"
              name="type"
              required
              variant="outlined"
              density="comfortable"
            />
            <p class="form-help">{{ $t('lists.typeHelp') }}</p>
          </v-col>

          <v-col cols="12" md="6">
            <v-select
              v-model="form.optin"
              :items="optinOptions"
              item-title="title"
              item-value="value"
              :label="$t('lists.optin')"
              name="optin"
              required
              variant="outlined"
              density="comfortable"
            />
            <p class="form-help">{{ $t('lists.optinHelp') }}</p>
          </v-col>
        </v-row>

        <v-text-field
          v-model="tagsInput"
          :aria-label="$t('globals.terms.tags')"
          :label="$t('globals.terms.tags')"
          :placeholder="$t('globals.terms.tags')"
          type="text"
          variant="outlined"
          density="comfortable"
          class="mb-2"
        />

        <v-textarea
          v-model="form.description"
          :label="$t('globals.fields.description')"
          maxlength="2000"
          name="description"
          :placeholder="$t('globals.fields.description')"
          variant="outlined"
          auto-grow
          rows="4"
          class="mb-2"
        />

        <v-card class="settings-box mt-2" variant="tonal">
          <v-card-text class="pa-4">
            <v-checkbox
              v-model="isArchived"
              :label="$t('lists.archived')"
              name="status"
              density="comfortable"
              hide-details
              class="mb-1"
            />
            <p class="form-help">{{ $t('lists.archivedHelp') }}</p>
          </v-card-text>
        </v-card>
      </section>

      <footer class="admin-dialog-foot modal-card-foot">
        <v-btn
          type="button"
          variant="outlined"
          class="dialog-action"
          @click="$emit('close')"
        >
          {{ $t('globals.buttons.close') }}
        </v-btn>
        <v-btn
          v-if="$can('lists:manage_all') || $canList(data.id, 'list:manage')"
          color="primary"
          variant="flat"
          class="dialog-action"
          data-cy="btn-save"
          :disabled="loading.lists"
          :loading="loading.lists"
          type="submit"
        >
          {{ $t('globals.buttons.save') }}
        </v-btn>
      </footer>
    </div>
  </v-form>
</template>

<script>
import { mapState } from 'vuex';
import CopyText from '../components/CopyText.vue';

export default {
  name: 'ListForm',

  components: {
    CopyText,
  },

  props: {
    data: { type: Object, default: () => ({}) },
    isEditing: { type: Boolean, default: false },
  },

  data() {
    return {
      // Binds form input values.
      form: {
        name: '',
        type: 'private',
        optin: 'single',
        description: '',
        status: 'active',
        tags: [],
      },
    };
  },

  methods: {
    onSubmit() {
      if (this.isEditing) {
        this.updateList();
        return;
      }

      this.createList();
    },

    createList() {
      this.$api.createList(this.form).then((data) => {
        this.$emit('finished');
        this.$emit('close');
        this.$utils.toast(this.$t('globals.messages.created', { name: data.name }));
      });
    },

    updateList() {
      this.$api.updateList({ id: this.data.id, record_id: this.data.record_id, ...this.form }).then((data) => {
        this.$emit('finished');
        this.$emit('close');
        this.$utils.toast(this.$t('globals.messages.updated', { name: data.name }));
      });
    },
  },

  computed: {
    ...mapState(['loading', 'profile']),

    listTypeOptions() {
      return [
        { title: this.$t('lists.types.private'), value: 'private' },
        { title: this.$t('lists.types.public'), value: 'public' },
      ];
    },

    optinOptions() {
      return [
        { title: this.$t('lists.optins.single'), value: 'single' },
        { title: this.$t('lists.optins.double'), value: 'double' },
      ];
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

    isArchived: {
      get() {
        return this.form.status === 'archived';
      },
      set(v) {
        this.form.status = v ? 'archived' : 'active';
      },
    },
  },

  mounted() {
    this.form = { ...this.form, ...this.$props.data };

    this.$nextTick(() => {
      if (this.$refs.focus && typeof this.$refs.focus.focus === 'function') {
        this.$refs.focus.focus();
      }
    });
  },
};
</script>

<style scoped>
.admin-dialog-card {
  background: #fff;
  border: 1px solid #dce5f2;
  border-radius: 16px;
  box-shadow: 0 18px 45px rgba(15, 23, 42, 0.18);
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 48px);
  overflow: hidden;
  width: min(680px, calc(100vw - 32px));
}

.admin-dialog-head {
  background: linear-gradient(180deg, #ffffff 0%, #f8fbff 100%);
  border-bottom: 1px solid #ebf1fb;
  display: block;
  padding: 18px 20px;
}

.admin-dialog-body {
  overflow: auto;
  padding: 24px 20px;
}

.admin-dialog-foot {
  background: #fff;
  border-top: 1px solid #ebf1fb;
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  padding: 16px 20px 20px;
}

.dialog-meta-row {
  align-items: flex-start;
  display: flex;
  justify-content: space-between;
}

.entity-meta {
  margin: 0;
}

.dialog-title {
  margin: 8px 0 0;
}

.form-field {
  margin-bottom: 18px;
}

.form-help {
  color: #667085;
  font-size: 0.9rem;
  margin-top: 4px;
}

.settings-box {
  background: #f8fbff;
  border: 1px solid #e7eefb;
  border-radius: 12px;
}

:deep(.v-field) {
  border-radius: 12px;
}

.dialog-action {
  height: 44px;
  min-width: 120px;
}

.type-pill {
  background: #eff6ff;
  border-radius: 999px;
  color: #0f5bd8;
  display: inline-block;
  float: right;
  font-size: 0.85rem;
  font-weight: 600;
  padding: 6px 10px;
}

@media (max-width: 640px) {
  .admin-dialog-head {
    padding: 16px;
  }

  .admin-dialog-body {
    padding: 16px;
  }

  .admin-dialog-foot {
    flex-direction: column-reverse;
    padding: 12px 16px 16px;
  }

  .dialog-action {
    min-width: 100%;
  }

  .dialog-meta-row {
    align-items: flex-start;
    flex-direction: column;
    gap: 10px;
  }
}
</style>
