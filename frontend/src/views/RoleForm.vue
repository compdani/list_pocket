<template>
  <v-form @submit.prevent="onSubmit">
    <div class="admin-dialog-card modal-card content role-dialog-card">
      <header class="admin-dialog-head modal-card-head">
        <div class="dialog-meta-row">
          <p v-if="isEditing" class="entity-meta has-text-grey is-size-7">
            {{ $t('globals.fields.id') }}:
            <span data-cy="id"><copy-text :text="`${data.id}`" /></span>
          </p>
        </div>

        <h4 v-if="isEditing" class="dialog-title">
          {{ data.name }}
        </h4>
        <h4 v-else class="dialog-title">
          {{ type === 'user' ? $t('users.newUserRole') : $t('users.newListRole') }}
        </h4>
      </header>

      <section class="admin-dialog-body modal-card-body role-dialog-body">
        <v-alert
          v-if="formError"
          type="error"
          variant="tonal"
          class="mb-4"
        >
          {{ formError }}
        </v-alert>

        <v-text-field
          ref="focus"
          v-model="form.name"
          :disabled="disabled"
          :label="$t('globals.fields.name')"
          maxlength="200"
          required
          variant="outlined"
          density="comfortable"
        />

        <section v-if="type === 'list'" class="panel-card">
          <div class="panel-card__head">
            <div>
              <h5 class="panel-card__title">{{ $t('users.listPerms') }}</h5>
              <p class="panel-card__subtle">Grant list-specific access and management rights.</p>
            </div>
          </div>

          <div class="list-add-row">
            <v-select
              v-model="form.curList"
              :items="filteredLists"
              item-title="name"
              item-value="id"
              :disabled="disabled || filteredLists.length < 1"
              :label="$tc('globals.terms.list')"
              variant="outlined"
              density="comfortable"
              hide-details
            />
            <v-btn
              color="primary"
              :disabled="disabled || !form.curList"
              @click="onAddListPerm"
            >
              {{ $t('globals.buttons.add') }}
            </v-btn>
          </div>

          <div
            v-if="form.lists.length > 0 && form.permissions && (form.permissions.includes('lists:get_all') || form.permissions.includes('lists:manage_all'))"
            class="warning-row"
          >
            {{ $t('users.listPermsWarning') }}
          </div>

          <div v-if="form.lists.length > 0" class="list-perm-grid">
            <article v-for="item in form.lists" :key="item.id" class="list-perm-card">
              <div class="list-perm-card__head">
                <div>
                  <div class="list-perm-card__title">{{ item.name }}</div>
                  <div class="list-perm-card__meta">List ID: {{ item.id }}</div>
                </div>
                <v-btn
                  v-if="!disabled"
                  icon="mdi-trash-can-outline"
                  variant="text"
                  color="error"
                  @click="onDeleteListPerm(item.id)"
                />
              </div>

              <div class="checkbox-row">
                <v-checkbox
                  v-model="item.permissions"
                  value="list:get"
                  :disabled="disabled"
                  :label="$t('globals.buttons.view')"
                  hide-details
                  density="comfortable"
                />
                <v-checkbox
                  v-model="item.permissions"
                  value="list:manage"
                  :disabled="disabled"
                  :label="$t('globals.buttons.manage')"
                  hide-details
                  density="comfortable"
                />
              </div>
            </article>
          </div>

          <v-alert
            v-else
            type="info"
            variant="tonal"
            class="mt-3"
          >
            No list permissions configured yet.
          </v-alert>
        </section>

        <section v-if="type === 'user'" class="panel-card">
          <div class="panel-card__head">
            <div>
              <h5 class="panel-card__title">{{ $t('users.perms') }}</h5>
              <p class="panel-card__subtle">Global permissions attached to this role.</p>
            </div>
            <v-btn
              v-if="!disabled"
              variant="text"
              size="small"
              @click="onToggleSelect"
            >
              {{ $t('globals.buttons.toggleSelect') }}
            </v-btn>
          </div>

          <div class="permissions-group">
            <article
              v-for="group in serverConfig.permissions"
              :key="group.group"
              class="permissions-card"
            >
              <h6 class="permissions-card__title">
                {{ $tc(`globals.terms.${group.group}`) }}
              </h6>

              <div class="permissions-card__items">
                <label
                  v-for="perm in group.permissions"
                  :key="perm"
                  class="permission-chip"
                >
                  <v-checkbox
                    v-model="form.permissions"
                    :value="perm"
                    :disabled="disabled"
                    hide-details
                    density="comfortable"
                  />
                  <span>{{ perm }}</span>
                  <a
                    v-if="perm === 'subscribers:sql_query'"
                    :href="$docsUrl('roles-and-permissions/#subscriberssql_query')"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="perm-link"
                  >
                    <v-icon size="small" color="error">mdi-alert-outline</v-icon>
                  </a>
                </label>
              </div>
            </article>
          </div>
        </section>

        <a
          :href="$docsUrl('roles-and-permissions/')"
          target="_blank"
          rel="noopener noreferrer"
          class="learn-link"
        >
          <v-icon size="small">mdi-link-variant</v-icon>
          {{ $t('globals.buttons.learnMore') }}
        </a>
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
          v-if="!disabled"
          color="primary"
          variant="flat"
          class="dialog-action"
          data-cy="btn-save"
          :loading="isLoading"
          :disabled="isLoading"
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

const baseForm = () => ({
  curList: null,
  lists: [],
  name: '',
  permissions: [],
});

export default {
  name: 'RoleForm',

  components: {
    CopyText,
  },

  emits: ['close', 'finished'],

  props: {
    data: { type: Object, default: () => ({}) },
    isEditing: { type: Boolean, default: false },
    type: { type: String, default: 'user' },
  },

  data() {
    return {
      availableLists: [],
      disabled: false,
      form: baseForm(),
      formError: '',
      hasToggle: false,
    };
  },

  computed: {
    ...mapState(['loading', 'serverConfig']),

    filteredLists() {
      const selected = this.form.lists.reduce((acc, item) => ({ ...acc, [item.id]: true }), {});
      return this.availableLists.filter((item) => !selected[item.id]);
    },

    isLoading() {
      return this.type === 'user' ? this.loading.userRoles : this.loading.listRoles;
    },
  },

  methods: {
    extractErrorMessage(err) {
      if (err && err.response && err.response.message) {
        return err.response.message;
      }

      if (err && err.message) {
        return err.message;
      }

      return 'Something went wrong while processing your request.';
    },

    async loadLists() {
      if (this.type !== 'list') {
        return;
      }

      const resp = await this.$api.getLists({ minimal: true, per_page: 'all', status: 'active' });
      this.availableLists = resp.results || resp || [];
    },

    onAddListPerm() {
      const list = this.availableLists.find((item) => item.id === this.form.curList);
      if (!list) {
        return;
      }

      this.form.lists.push({
        id: list.id,
        name: list.name,
        permissions: ['list:get', 'list:manage'],
      });
      this.form.curList = this.filteredLists.length > 0 ? this.filteredLists[0].id : null;
    },

    onDeleteListPerm(id) {
      this.form.lists = this.form.lists.filter((item) => item.id !== id);
      this.form.curList = this.filteredLists.length > 0 ? this.filteredLists[0].id : null;
    },

    onSubmit() {
      this.formError = '';

      if (this.isEditing) {
        this.updateRole();
        return;
      }

      this.createRole();
    },

    onToggleSelect() {
      if (this.hasToggle) {
        this.form.permissions = [];
      } else {
        this.form.permissions = this.serverConfig.permissions.reduce((acc, item) => {
          item.permissions.forEach((perm) => {
            acc.push(perm);
          });
          return acc;
        }, []);
      }

      this.hasToggle = !this.hasToggle;
    },

    createRole() {
      let fn;
      const form = { name: this.form.name };

      if (this.type === 'user') {
        fn = this.$api.createUserRole;
        form.permissions = this.form.permissions;
      } else {
        fn = this.$api.createListRole;
        form.lists = this.form.lists.map((item) => ({ id: item.id, permissions: item.permissions }));
      }

      fn(form).then((data) => {
        this.$emit('finished');
        this.$utils.toast(this.$t('globals.messages.created', { name: data.name }));
        this.$emit('close');
      }).catch((err) => {
        this.formError = this.extractErrorMessage(err);
      });
    },

    updateRole() {
      let fn;
      const form = { id: this.data.id, name: this.form.name };

      if (this.type === 'user') {
        fn = this.$api.updateUserRole;
        form.permissions = this.form.permissions;
      } else {
        fn = this.$api.updateListRole;
        form.lists = this.form.lists.map((item) => ({ id: item.id, permissions: item.permissions }));
      }

      fn(form).then((data) => {
        this.$emit('finished');
        this.$utils.toast(this.$t('globals.messages.updated', { name: data.name }));
        this.$emit('close');
      }).catch((err) => {
        this.formError = this.extractErrorMessage(err);
      });
    },
  },

  async mounted() {
    await this.loadLists();

    if (this.isEditing) {
      this.form = {
        ...baseForm(),
        ...this.data,
        permissions: Array.isArray(this.data.permissions) ? [...this.data.permissions] : [],
        lists: Array.isArray(this.data.lists)
          ? this.data.lists.map((item) => ({ ...item, permissions: [...item.permissions] }))
          : [],
      };

      if (this.data.id === 1 || !this.$can('roles:manage')) {
        this.disabled = true;
      }
    } else if (this.type === 'user') {
      const skip = ['admin', 'users'];
      this.form.permissions = this.serverConfig.permissions.reduce((acc, item) => {
        if (skip.includes(item.group)) {
          return acc;
        }
        item.permissions.forEach((perm) => {
          if (perm !== 'subscribers:sql_query' && !perm.startsWith('lists:') && !perm.startsWith('settings:')) {
            acc.push(perm);
          }
        });
        return acc;
      }, []);
    }

    this.$nextTick(() => {
      this.form.curList = this.filteredLists.length > 0 ? this.filteredLists[0].id : null;
      if (this.$refs.focus?.focus) {
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
  width: min(980px, calc(100vw - 32px));
}

.admin-dialog-head {
  background: linear-gradient(180deg, #ffffff 0%, #f8fbff 100%);
  border-bottom: 1px solid #ebf1fb;
  display: block;
  padding: 18px 20px;
}

.dialog-meta-row {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
}

.dialog-title {
  margin: 8px 0 0;
}

.role-dialog-body {
  overflow: auto;
  padding: 24px 20px;
}

.panel-card {
  background: linear-gradient(180deg, #fbfdff 0%, #f6f9ff 100%);
  border: 1px solid #dce5f2;
  border-radius: 14px;
  margin-top: 16px;
  padding: 16px;
}

.panel-card__head {
  align-items: flex-start;
  display: flex;
  justify-content: space-between;
  margin-bottom: 14px;
}

.panel-card__title {
  font-size: 1rem;
  font-weight: 700;
  margin: 0;
}

.panel-card__subtle {
  color: #64748b;
  font-size: 0.92rem;
  margin: 4px 0 0;
}

.list-add-row {
  display: grid;
  gap: 12px;
  grid-template-columns: minmax(0, 1fr) auto;
}

.warning-row {
  color: #b45309;
  font-size: 0.92rem;
  margin-top: 12px;
}

.list-perm-grid {
  display: grid;
  gap: 12px;
  margin-top: 16px;
}

.list-perm-card {
  background: #fff;
  border: 1px solid #d7e1f0;
  border-radius: 12px;
  padding: 14px;
}

.list-perm-card__head {
  align-items: flex-start;
  display: flex;
  justify-content: space-between;
}

.list-perm-card__title {
  font-weight: 700;
}

.list-perm-card__meta {
  color: #64748b;
  font-size: 0.85rem;
  margin-top: 2px;
}

.checkbox-row {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  margin-top: 10px;
}

.permissions-group {
  display: grid;
  gap: 14px;
}

.permissions-card {
  background: #fff;
  border: 1px solid #d7e1f0;
  border-radius: 12px;
  padding: 14px;
}

.permissions-card__title {
  color: #1e293b;
  font-size: 0.95rem;
  font-weight: 700;
  margin: 0 0 10px;
}

.permissions-card__items {
  display: grid;
  gap: 8px;
}

.permission-chip {
  align-items: center;
  background: #f8fbff;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  display: flex;
  gap: 8px;
  min-height: 44px;
  padding: 0 12px 0 4px;
}

.permission-chip span {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, Liberation Mono, monospace;
  font-size: 0.88rem;
}

.perm-link {
  margin-left: auto;
}

.learn-link {
  align-items: center;
  color: #2563eb;
  display: inline-flex;
  gap: 6px;
  margin-top: 16px;
  text-decoration: none;
}

.admin-dialog-foot {
  border-top: 1px solid #ebf1fb;
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  padding: 16px 20px;
}

@media (max-width: 959px) {
  .list-add-row {
    grid-template-columns: 1fr;
  }

  .checkbox-row {
    flex-direction: column;
    gap: 4px;
  }
}
</style>
