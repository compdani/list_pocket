<template>
  <v-form @submit.prevent="onSubmit">
    <div class="admin-dialog-card modal-card content user-dialog-card">
      <header class="admin-dialog-head modal-card-head">
        <div class="dialog-meta-row">
          <p v-if="isEditing" class="entity-meta has-text-grey is-size-7">
            {{ $t('globals.fields.id') }}:
            <span data-cy="id"><copy-text :text="`${identity}`" /></span>
          </p>
        </div>

        <h4 v-if="isEditing" class="dialog-title">
          {{ data.name || data.username }}
        </h4>
        <h4 v-else class="dialog-title">
          {{ $t('users.newUser') }}
        </h4>
      </header>

      <section class="admin-dialog-body modal-card-body user-dialog-body">
        <v-alert
          v-if="formError"
          type="error"
          variant="tonal"
          class="mb-4"
        >
          {{ formError }}
        </v-alert>

        <v-row class="mb-2">
          <v-col cols="12" md="7">
            <div class="field-label">{{ $t('users.type.user') }} / {{ $t('users.type.api') }}</div>
            <div class="type-toggle">
              <v-btn
                :variant="form.type === 'user' ? 'flat' : 'outlined'"
                :color="form.type === 'user' ? 'primary' : undefined"
                prepend-icon="mdi-account-outline"
                :disabled="isEditing"
                @click="form.type = 'user'"
              >
                {{ $t('users.type.user') }}
              </v-btn>
              <v-btn
                :variant="form.type === 'api' ? 'flat' : 'outlined'"
                :color="form.type === 'api' ? 'primary' : undefined"
                prepend-icon="mdi-code-tags"
                :disabled="isEditing"
                @click="form.type = 'api'"
              >
                {{ $t('users.type.api') }}
              </v-btn>
            </div>
          </v-col>

          <v-col cols="12" md="5">
            <v-select
              v-model="form.status"
              :items="statusOptions"
              item-title="title"
              item-value="value"
              :label="$t('globals.fields.status')"
              variant="outlined"
              density="comfortable"
            />
          </v-col>
        </v-row>

        <v-row>
          <v-col cols="12" md="6">
            <v-text-field
              ref="focus"
              v-model="form.username"
              :label="$t('users.username')"
              maxlength="200"
              :hint="$t('users.usernameHelp')"
              persistent-hint
              name="username"
              autocomplete="off"
              required
              variant="outlined"
              density="comfortable"
            />
          </v-col>

          <v-col cols="12" md="6">
            <v-text-field
              v-model="form.name"
              :label="$t('globals.fields.name')"
              maxlength="200"
              name="name"
              variant="outlined"
              density="comfortable"
            />
          </v-col>
        </v-row>

        <v-row v-if="form.type !== 'api'">
          <v-col cols="12">
            <v-text-field
              v-model="form.email"
              :label="$t('subscribers.email')"
              maxlength="200"
              name="email"
              required
              variant="outlined"
              density="comfortable"
            />
          </v-col>
        </v-row>

        <section v-if="form.type !== 'api'" class="panel-card">
          <div class="panel-card__head">
            <div>
              <h5 class="panel-card__title">{{ $t('users.password') }}</h5>
              <p class="panel-card__subtle">{{ $t('users.passwordEnable') }}</p>
            </div>
            <v-switch
              v-model="form.passwordLogin"
              color="primary"
              hide-details
              inset
            />
          </div>

          <v-row>
            <v-col cols="12" md="6">
              <password-field
                v-model="form.password"
                :disabled="!form.passwordLogin"
                :label="$t('users.password')"
                autocomplete="new-password"
                :rules="form.passwordLogin && !isEditing ? [(v) => Boolean(v) || $t('auth.requiredField')] : []"
              />
            </v-col>

            <v-col cols="12" md="6">
              <password-field
                v-model="form.password2"
                :disabled="!form.passwordLogin"
                :label="$t('users.passwordRepeat')"
                autocomplete="new-password"
              />
            </v-col>
          </v-row>
        </section>

        <section class="panel-card">
          <div class="panel-card__head">
            <div>
              <h5 class="panel-card__title">{{ $tc('users.roles') }}</h5>
              <p class="panel-card__subtle">User access and scoped list access.</p>
            </div>
          </div>

          <v-row>
            <v-col cols="12" md="6">
              <v-select
                v-model="form.userRoleId"
                :items="userRoles"
                item-title="name"
                item-value="id"
                :label="$tc('users.userRole')"
                required
                variant="outlined"
                density="comfortable"
              />
            </v-col>

            <v-col cols="12" md="6">
              <v-select
                v-model="form.listRoleId"
                :items="listRoleOptions"
                item-title="name"
                item-value="id"
                :label="$tc('users.listRole', 0)"
                variant="outlined"
                density="comfortable"
              />
            </v-col>
          </v-row>
        </section>

        <v-alert
          v-if="apiToken"
          type="success"
          variant="tonal"
          class="mt-4"
        >
          <div class="token-head">{{ $t('users.apiOneTimeToken') }}</div>
          <copy-text :text="apiToken" />
        </v-alert>
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
          v-if="$can('users:manage') && !apiToken"
          color="primary"
          variant="flat"
          class="dialog-action"
          data-cy="btn-save"
          :loading="loading.users"
          :disabled="loading.users"
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
import PasswordField from '../components/PasswordField.vue';

const baseForm = () => ({
  username: '',
  email: '',
  name: '',
  password: '',
  password2: '',
  passwordLogin: false,
  type: 'user',
  status: 'enabled',
  userRoleId: null,
  listRoleId: null,
});

export default {
  name: 'UserForm',

  components: {
    CopyText,
    PasswordField,
  },

  emits: ['close', 'finished'],

  props: {
    data: { type: Object, default: () => ({}) },
    isEditing: { type: Boolean, default: false },
  },

  data() {
    return {
      apiToken: null,
      form: baseForm(),
      formError: '',
    };
  },

  computed: {
    ...mapState(['loading', 'userRoles', 'listRoles']),

    identity() {
      return this.data.recordId || this.data.record_id || this.data.id || '';
    },

    listRoleOptions() {
      return [{ id: null, name: `-- ${this.$t('globals.terms.none')} --` }, ...this.listRoles];
    },

    statusOptions() {
      return [
        { title: this.$t('users.status.enabled'), value: 'enabled' },
        { title: this.$t('users.status.disabled'), value: 'disabled' },
      ];
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

    normalizeForm(data = {}) {
      return {
        ...baseForm(),
        ...data,
        password: '',
        password2: '',
        passwordLogin: !!data.passwordLogin,
        userRoleId: data.userRole ? data.userRole.id : null,
        listRoleId: data.listRole ? data.listRole.id : null,
      };
    },

    onSubmit() {
      this.formError = '';

      if (!this.form.passwordLogin) {
        this.form.password = null;
        this.form.password2 = null;
      }

      if (this.form.type !== 'api' && this.form.passwordLogin && this.form.password !== this.form.password2) {
        this.formError = this.$t('users.passwordMismatch');
        return;
      }

      if (this.isEditing) {
        this.updateUser();
        return;
      }

      this.createUser();
    },

    createUser() {
      const form = {
        ...this.form,
        password_login: this.form.passwordLogin,
        user_role_id: this.form.userRoleId,
        list_role_id: this.form.listRoleId || null,
      };

      this.$api.createUser(form).then((data) => {
        this.$emit('finished');
        this.$utils.toast(this.$t('globals.messages.created', { name: data.name }));

        if (form.type === 'api') {
          this.apiToken = data.password;
          return;
        }

        this.$emit('close');
      }).catch((err) => {
        this.formError = this.extractErrorMessage(err);
      });
    },

    updateUser() {
      const form = {
        ...this.form,
        id: this.identity,
        password_login: this.form.passwordLogin,
        user_role_id: this.form.userRoleId,
        list_role_id: this.form.listRoleId || null,
      };

      this.$api.updateUser(form).then((data) => {
        this.$emit('finished');
        this.$emit('close');
        this.$utils.toast(this.$t('globals.messages.updated', { name: data.name }));
      }).catch((err) => {
        this.formError = this.extractErrorMessage(err);
      });
    },
  },

  mounted() {
    this.form = this.normalizeForm(this.data);

    this.$api.getUserRoles();
    this.$api.getListRoles();

    this.$nextTick(() => {
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
  width: min(860px, calc(100vw - 32px));
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

.user-dialog-body {
  display: flex;
  flex: 1;
  flex-direction: column;
  overflow: auto;
  padding: 24px 20px;
}

.field-label {
  color: #475569;
  font-size: 0.9rem;
  font-weight: 600;
  margin-bottom: 0.65rem;
}

.type-toggle {
  display: flex;
  gap: 0.75rem;
}

.type-toggle :deep(.v-btn) {
  flex: 1 1 0;
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
  margin-bottom: 12px;
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

.token-head {
  font-weight: 700;
  margin-bottom: 8px;
}

.admin-dialog-foot {
  border-top: 1px solid #ebf1fb;
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  padding: 16px 20px;
}

@media (max-width: 959px) {
  .type-toggle {
    flex-direction: column;
  }
}
</style>
