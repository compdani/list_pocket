<template>
  <section class="users">
    <header class="page-header">
      <div class="header-content">
        <h1 class="text-h4">
          {{ $t('globals.terms.users') }}
          <span v-if="!isNaN(users.length)">({{ users.length }})</span>
        </h1>
      </div>
      <div class="header-actions">
        <v-btn
          v-if="$can('users:manage')"
          color="primary"
          prepend-icon="mdi-plus"
          data-cy="btn-new"
          @click="showNewForm"
        >
          {{ $t('globals.buttons.new') }}
        </v-btn>
      </div>
    </header>

    <v-card class="mb-4 query-card" elevation="0">
      <v-card-text class="query-card-body">
        <form class="query-form" @submit.prevent="getUsers">
          <v-text-field
            v-model="queryParams.query"
            class="query-input"
            name="query"
            :placeholder="$t('users.username')"
            prepend-inner-icon="mdi-magnify"
            variant="outlined"
            density="comfortable"
            hide-details
            ref="query"
            data-cy="query"
          />
          <v-btn
            type="submit"
            class="query-submit"
            color="primary"
            icon="mdi-magnify"
            data-cy="btn-query"
          />
        </form>
      </v-card-text>
    </v-card>

    <v-data-table
      :headers="tableHeaders"
      :items="filteredUsers"
      :loading="loading.users"
      class="admin-data-table users-table"
      item-value="id"
    >
      <template #[`item.username`]="{ item }">
        <div>
          <button
            type="button"
            class="link-button"
            @click="showEditForm(item)"
          >
            {{ item.username }}
          </button>
          <div class="user-meta mt-2">
            <v-chip v-if="item.status === 'disabled'" size="small" variant="tonal" color="warning">
              {{ $t(`users.status.${item.status}`) }}
            </v-chip>
            <v-chip v-if="item.type === 'api'" size="small" variant="tonal">
              <v-icon start size="small">mdi-code-tags</v-icon>
              {{ $t(`users.type.${item.type}`) }}
            </v-chip>
          </div>
          <div class="text-caption text-medium-emphasis mt-2">
            {{ item.name || '—' }}
          </div>
        </div>
      </template>

      <template #[`item.role`]="{ item }">
        <div class="role-stack">
          <router-link :to="{ name: 'userRoles' }" class="role-link">
            <v-chip size="small" :color="item.userRole && item.userRole.id === 1 ? 'success' : 'primary'" variant="tonal">
              <v-icon start size="small">mdi-account-outline</v-icon>
              {{ item.userRole ? item.userRole.name : '—' }}
            </v-chip>
          </router-link>
          <router-link v-if="item.listRole" :to="{ name: 'listRoles' }" class="role-link">
            <v-chip size="small" variant="tonal">
              <v-icon start size="small">mdi-format-list-bulleted-square</v-icon>
              {{ item.listRole.name }}
            </v-chip>
          </router-link>
        </div>
      </template>

      <template #[`item.email`]="{ item }">
        <span>{{ item.email || '—' }}</span>
      </template>

      <template #[`item.createdAt`]="{ item }">
        {{ $utils.niceDate(item.createdAt) }}
      </template>

      <template #[`item.updatedAt`]="{ item }">
        {{ $utils.niceDate(item.updatedAt) }}
      </template>

      <template #[`item.loggedinAt`]="{ item }">
        {{ item.loggedinAt ? $utils.niceDate(item.loggedinAt, true) : '—' }}
      </template>

      <template #[`item.actions`]="{ item }">
        <div class="action-group">
          <v-tooltip :text="$t('globals.buttons.edit')" location="top">
            <template #activator="{ props }">
              <v-btn
                v-if="$can('users:manage')"
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
                v-if="$can('users:manage')"
                v-bind="props"
                icon="mdi-trash-can-outline"
                size="x-small"
                variant="text"
                color="error"
                data-cy="btn-delete"
                @click="deleteUser(item)"
              />
            </template>
          </v-tooltip>
        </div>
      </template>

      <template #no-data>
        <empty-placeholder v-if="!loading.users" />
      </template>
    </v-data-table>

    <v-overlay
      :model-value="isFormVisible"
      class="admin-overlay align-center justify-center"
      scrim="rgba(15, 23, 42, 0.45)"
      @update:model-value="handleDialogModelUpdate"
    >
      <div class="admin-dialog-frame user-dialog-frame">
        <user-form
          v-if="isFormVisible"
          :data="curItem"
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
import UserForm from './UserForm.vue';

export default {
  components: {
    EmptyPlaceholder,
    UserForm,
  },

  data() {
    return {
      curItem: null,
      isEditing: false,
      isFormVisible: false,
      users: [],
      queryParams: {
        query: '',
      },
    };
  },

  computed: {
    ...mapState(['loading']),

    filteredUsers() {
      const q = this.queryParams.query.trim().toLowerCase();
      if (!q) {
        return this.users;
      }

      return this.users.filter((item) => [item.username, item.name, item.email]
        .filter(Boolean)
        .some((value) => value.toLowerCase().includes(q)));
    },

    tableHeaders() {
      return [
        { title: this.$t('users.username'), key: 'username' },
        { title: this.$tc('users.role'), key: 'role', sortable: false },
        { title: this.$t('subscribers.email'), key: 'email' },
        { title: this.$t('globals.fields.createdAt'), key: 'createdAt' },
        { title: this.$t('globals.fields.updatedAt'), key: 'updatedAt' },
        { title: this.$t('users.lastLogin'), key: 'loggedinAt' },
        { title: '', key: 'actions', align: 'end', sortable: false, width: 100 },
      ];
    },
  },

  methods: {
    showEditForm(item) {
      this.curItem = item;
      this.isFormVisible = true;
      this.isEditing = true;
    },

    showNewForm() {
      this.curItem = {};
      this.isFormVisible = true;
      this.isEditing = false;
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
      this.getUsers();
    },

    getUsers() {
      this.$api.queryUsers().then((resp) => {
        this.users = resp;
      });
    },

    deleteUser(item) {
      this.$utils.confirm(this.$t('globals.messages.confirm'), () => {
        this.$api.deleteUser(item.recordId || item.record_id || item.id).then(() => {
          this.getUsers();
          this.$utils.toast(this.$t('globals.messages.deleted', { name: item.name || item.username }));
        });
      });
    },
  },

  created() {
    this.$events.$on('page.refresh', this.getUsers);
  },

  destroyed() {
    this.$events.$off('page.refresh', this.getUsers);
  },

  mounted() {
    if (this.$route.params.id) {
      this.$api.getUser(this.$route.params.id).then((data) => {
        this.showEditForm(data);
      });
    } else {
      this.getUsers();
    }
  },
};
</script>

<style scoped>
.users {
  --users-border: #dce5f2;
  --users-border-strong: #c7d5ea;
  --users-surface-soft: #f6f9ff;
}

.page-header {
  align-items: center;
  display: flex;
  justify-content: space-between;
  margin-bottom: 24px;
}

.query-card {
  border: 1px solid var(--users-border);
  border-radius: 18px;
  overflow: hidden;
}

.query-card-body {
  padding: 18px 20px !important;
}

.query-form {
  display: flex;
  gap: 12px;
}

.query-input {
  flex: 1 1 auto;
}

.users-table {
  border: 1px solid var(--users-border);
  border-radius: 20px;
  overflow: hidden;
}

.users-table :deep(thead th) {
  background: var(--users-surface-soft);
  border-bottom: 1px solid var(--users-border-strong) !important;
  font-weight: 700;
}

.users-table :deep(tbody td) {
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

.role-stack {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.role-link {
  text-decoration: none;
}

.action-group {
  display: flex;
  justify-content: flex-end;
}

.user-dialog-frame {
  max-width: min(860px, calc(100vw - 32px));
  width: 100%;
}

@media (max-width: 959px) {
  .page-header {
    align-items: stretch;
    flex-direction: column;
    gap: 16px;
  }

  .query-form {
    flex-direction: column;
  }
}
</style>
