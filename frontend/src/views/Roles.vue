<template>
  <section class="roles">
    <header class="page-header">
      <div class="header-content">
        <h1 class="text-h4">
          {{ $t(isUser ? 'users.userRoles' : 'users.listRoles') }}
          <span v-if="!isNaN(roles.length)">({{ roles.length }})</span>
        </h1>
      </div>
      <div class="header-actions">
        <v-btn
          v-if="$can('roles:manage')"
          color="primary"
          prepend-icon="mdi-plus"
          data-cy="btn-new"
          @click="showNewForm"
        >
          {{ $t('globals.buttons.new') }}
        </v-btn>
      </div>
    </header>

    <v-data-table
      :headers="tableHeaders"
      :items="roles"
      :loading="isLoading"
      class="admin-data-table roles-table"
      item-value="id"
    >
      <template #[`item.name`]="{ item }">
        <div>
          <button
            type="button"
            class="link-button"
            @click="showEditForm(item)"
          >
            {{ item.name }}
          </button>
          <div v-if="item.name === 'Super Admin' && isUser" class="mt-2">
            <v-chip size="small" color="success" variant="tonal">
              {{ item.name }}
            </v-chip>
          </div>
        </div>
      </template>

      <template #[`item.createdAt`]="{ item }">
        {{ $utils.niceDate(item.createdAt) }}
      </template>

      <template #[`item.updatedAt`]="{ item }">
        {{ $utils.niceDate(item.updatedAt) }}
      </template>

      <template #[`item.actions`]="{ item }">
        <div class="action-group">
          <v-tooltip :text="$t('globals.buttons.clone')" location="top">
            <template #activator="{ props }">
              <v-btn
                v-if="$can('roles:manage')"
                v-bind="props"
                icon="mdi-file-multiple-outline"
                size="x-small"
                variant="text"
                data-cy="btn-clone"
                @click="promptClone(item)"
              />
            </template>
          </v-tooltip>

          <template v-if="!(item.name === 'Super Admin' && isUser)">
            <v-tooltip :text="$t('globals.buttons.edit')" location="top">
              <template #activator="{ props }">
                <v-btn
                  v-if="$can('roles:manage')"
                  v-bind="props"
                  icon="mdi-pencil-outline"
                  size="x-small"
                  variant="text"
                  data-cy="btn-edit"
                  @click="showEditForm(item)"
                />
              </template>
            </v-tooltip>

            <v-tooltip :text="$t('globals.buttons.delete')" location="top">
              <template #activator="{ props }">
                <v-btn
                  v-if="$can('roles:manage')"
                  v-bind="props"
                  icon="mdi-trash-can-outline"
                  size="x-small"
                  variant="text"
                  color="error"
                  data-cy="btn-delete"
                  @click="onDeleteRole(item)"
                />
              </template>
            </v-tooltip>
          </template>
        </div>
      </template>

      <template #no-data>
        <empty-placeholder v-if="!isLoading" />
      </template>
    </v-data-table>

    <v-overlay
      :model-value="isFormVisible"
      class="admin-overlay align-center justify-center"
      scrim="rgba(15, 23, 42, 0.45)"
      @update:model-value="handleDialogModelUpdate"
    >
      <div class="admin-dialog-frame role-dialog-frame">
        <role-form
          v-if="isFormVisible"
          :data="curItem"
          :type="curType"
          :is-editing="isEditing"
          @finished="formFinished"
          @close="closeForm"
        />
      </div>
    </v-overlay>
  </section>
</template>

<script>
import { mapState } from 'vuex';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';
import RoleForm from './RoleForm.vue';

export default {
  components: {
    EmptyPlaceholder,
    RoleForm,
  },

  data() {
    return {
      curItem: null,
      curType: null,
      isEditing: false,
      isFormVisible: false,
    };
  },

  computed: {
    ...mapState(['loading', 'userRoles', 'listRoles']),

    isUser() {
      return this.curType === 'user';
    },

    isLoading() {
      return this.isUser ? this.loading.userRoles : this.loading.listRoles;
    },

    roles() {
      return this.isUser ? this.userRoles : this.listRoles;
    },

    tableHeaders() {
      return [
        { title: this.$tc('users.role'), key: 'name' },
        { title: this.$t('globals.fields.createdAt'), key: 'createdAt' },
        { title: this.$t('globals.fields.updatedAt'), key: 'updatedAt' },
        { title: '', key: 'actions', align: 'end', sortable: false, width: 110 },
      ];
    },
  },

  methods: {
    fetchRoles() {
      if (this.isUser) {
        this.$api.getUserRoles();
      } else {
        this.$api.getListRoles();
      }
    },

    showEditForm(item) {
      this.curItem = item;
      this.curType = this.isUser ? 'user' : 'list';
      this.isFormVisible = true;
      this.isEditing = true;
    },

    showNewForm() {
      this.curItem = {};
      this.isEditing = false;
      this.isFormVisible = true;
    },

    closeForm() {
      this.isFormVisible = false;
      this.curItem = null;
      this.isEditing = false;

      if (this.$route.params.id) {
        this.$router.push({ name: 'users' });
      }
    },

    handleDialogModelUpdate(value) {
      if (!value) {
        this.closeForm();
      }
    },

    formFinished() {
      this.fetchRoles();
    },

    promptClone(item) {
      this.$utils.prompt(
        this.$t('globals.buttons.clone'),
        {
          placeholder: this.$t('globals.fields.name'),
          value: this.$t('campaigns.copyOf', { name: item.name }),
        },
        (name) => this.onCloneRole(name, item),
      );
    },

    onCloneRole(name, item) {
      const form = { name };
      let fn;
      if (this.isUser) {
        fn = this.$api.createUserRole;
        form.permissions = item.permissions;
      } else {
        fn = this.$api.createListRole;
        form.lists = item.lists;
      }

      fn(form).then(() => {
        this.fetchRoles();
        this.$utils.toast(this.$t('globals.messages.created', { name }));
      });
    },

    onDeleteRole(item) {
      this.$utils.confirm(this.$t('globals.messages.confirm'), () => {
        this.$api.deleteRole(item.id).then(() => {
          this.fetchRoles();
          this.$utils.toast(this.$t('globals.messages.deleted', { name: item.name }));
        });
      });
    },
  },

  mounted() {
    this.curType = this.$route.name === 'userRoles' ? 'user' : 'list';
    this.fetchRoles();
  },
};
</script>

<style scoped>
.roles {
  --roles-border: #dce5f2;
  --roles-border-strong: #c7d5ea;
  --roles-surface-soft: #f6f9ff;
}

.page-header {
  align-items: center;
  display: flex;
  justify-content: space-between;
  margin-bottom: 24px;
}

.roles-table {
  border: 1px solid var(--roles-border);
  border-radius: 20px;
  overflow: hidden;
}

.roles-table :deep(thead th) {
  background: var(--roles-surface-soft);
  border-bottom: 1px solid var(--roles-border-strong) !important;
  font-weight: 700;
}

.roles-table :deep(tbody td) {
  padding-top: 18px !important;
  padding-bottom: 18px !important;
  vertical-align: top;
}

.link-button {
  color: rgb(var(--v-theme-primary));
  cursor: pointer;
  font: inherit;
  font-weight: 700;
  text-align: left;
}

.action-group {
  display: flex;
  justify-content: flex-end;
}

.role-dialog-frame {
  max-width: min(980px, calc(100vw - 32px));
  width: 100%;
}

@media (max-width: 959px) {
  .page-header {
    align-items: stretch;
    flex-direction: column;
    gap: 16px;
  }
}
</style>
