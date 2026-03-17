<template>
  <form @submit.prevent="onSubmit">
    <div class="admin-dialog-card modal-card content">
      <header class="admin-dialog-head modal-card-head">
        <p v-if="isEditing" class="has-text-grey-light is-size-7">
          {{ $t('globals.fields.id') }}: <copy-text :text="`${data.id}`" />
          {{ $t('globals.fields.uuid') }}: <copy-text :text="data.uuid" />
        </p>
        <span v-if="isEditing" class="type-pill" :class="data.type">
          {{ $t(`lists.types.${data.type}`) }}
        </span>
        <h4 v-if="isEditing">
          {{ data.name }}
        </h4>
        <h4 v-else>
          {{ $t('lists.newList') }}
        </h4>
      </header>
      <section expanded class="admin-dialog-body modal-card-body">
        <div class="form-field">
          <label class="form-label" for="list-name">{{ $t('globals.fields.name') }}</label>
          <input
            id="list-name"
            ref="focus"
            v-model="form.name"
            class="input"
            maxlength="200"
            name="name"
            :placeholder="$t('globals.fields.name')"
            required
            type="text"
          >
        </div>

        <div class="form-field">
          <label class="form-label" for="list-type">{{ $t('lists.type') }}</label>
          <select id="list-type" v-model="form.type" class="input" name="type" required>
            <option value="private">
              {{ $t('lists.types.private') }}
            </option>
            <option value="public">
              {{ $t('lists.types.public') }}
            </option>
          </select>
          <p class="form-help">{{ $t('lists.typeHelp') }}</p>
        </div>

        <div class="form-field">
          <label class="form-label" for="list-optin">{{ $t('lists.optin') }}</label>
          <select id="list-optin" v-model="form.optin" class="input" name="optin" required>
            <option value="single">
              {{ $t('lists.optins.single') }}
            </option>
            <option value="double">
              {{ $t('lists.optins.double') }}
            </option>
          </select>
          <p class="form-help">{{ $t('lists.optinHelp') }}</p>
        </div>

        <div class="form-field">
          <label class="form-label" for="list-tags">{{ $t('globals.terms.tags') }}</label>
          <input
            id="list-tags"
            :value="tagsInput"
            :aria-label="$t('globals.terms.tags')"
            class="input"
            :placeholder="$t('globals.terms.tags')"
            type="text"
            @input="tagsInput = $event.target.value"
          >
        </div>

        <div class="form-field">
          <label class="form-label" for="list-description">{{ $t('globals.fields.description') }}</label>
          <textarea
            id="list-description"
            v-model="form.description"
            class="input textarea-input"
            maxlength="2000"
            name="description"
            :placeholder="$t('globals.fields.description')"
          />
        </div>

        <div class="form-field">
          <label class="checkbox-row">
            <input v-model="isArchived" name="status" type="checkbox">
            <span>{{ $t('lists.archived') }}</span>
          </label>
          <p class="form-help">{{ $t('lists.archivedHelp') }}</p>
        </div>
      </section>
      <footer class="admin-dialog-foot modal-card-foot has-text-right">
        <button type="button" class="button secondary-button" @click="$emit('close')">
          {{ $t('globals.buttons.close') }}
        </button>
        <button
          v-if="$can('lists:manage_all') || $canList(data.id, 'list:manage')"
          class="button primary-button"
          data-cy="btn-save"
          :disabled="loading.lists"
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
      this.$api.updateList({ id: this.data.id, ...this.form }).then((data) => {
        this.$emit('finished');
        this.$emit('close');
        this.$utils.toast(this.$t('globals.messages.updated', { name: data.name }));
      });
    },
  },

  computed: {
    ...mapState(['loading', 'profile']),

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
  border: 1px solid #ddd;
  border-radius: 12px;
  box-shadow: 0 18px 45px rgba(15, 23, 42, 0.18);
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 48px);
  overflow: hidden;
  width: min(680px, calc(100vw - 32px));
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

.checkbox-row {
  align-items: center;
  display: flex;
  gap: 10px;
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
</style>
