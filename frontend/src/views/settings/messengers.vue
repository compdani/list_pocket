<template>
  <div class="settings-section">
    <v-card
      v-for="(item, n) in data.messengers"
      :key="n"
      variant="outlined"
      class="messenger-card"
    >
      <div class="card-head">
        <div class="toggle-field">
          <div class="text-subtitle-2">{{ $t('globals.buttons.enabled') }}</div>
          <v-switch v-model="item.enabled" color="primary" hide-details inset name="enabled" />
        </div>
        <v-btn
          variant="text"
          color="error"
          prepend-icon="mdi-trash-can-outline"
          @click.prevent="$utils.confirm(null, () => removeMessenger(n))"
        >
          {{ $t('globals.buttons.delete') }}
        </v-btn>
      </div>

      <div :class="{ 'section-disabled': !item.enabled }">
        <v-row>
          <v-col cols="12" md="4">
            <v-text-field
              :ref="`messengerName${n}`"
              v-model="item.name"
              :hint="$t('settings.messengers.nameHelp')"
              :label="$t('globals.fields.name')"
              maxlength="200"
              name="name"
              persistent-hint
              placeholder="mymessenger"
            />
          </v-col>
          <v-col cols="12" md="8">
            <v-text-field
              v-model="item.root_url"
              :hint="$t('settings.messengers.urlHelp')"
              :label="$t('settings.messengers.url')"
              maxlength="200"
              name="root_url"
              persistent-hint
              placeholder="https://postback.messenger.net/path"
              type="url"
            />
          </v-col>
        </v-row>

        <v-row>
          <v-col cols="12" md="6">
            <v-text-field
              v-model="item.username"
              :label="$t('settings.messengers.username')"
              maxlength="200"
              name="username"
            />
          </v-col>
          <v-col cols="12" md="6">
            <v-text-field
              v-model="item.password"
              :hint="$t('globals.messages.passwordChange')"
              :label="$t('settings.messengers.password')"
              maxlength="200"
              name="password"
              persistent-hint
              :placeholder="$t('globals.messages.passwordChange')"
              type="password"
            />
          </v-col>
        </v-row>

        <v-divider class="my-4" />

        <v-row>
          <v-col cols="12" md="4">
            <v-text-field
              v-model.number="item.max_conns"
              :hint="$t('settings.messengers.maxConnsHelp')"
              :label="$t('settings.messengers.maxConns')"
              max="65535"
              min="1"
              name="max_conns"
              persistent-hint
              placeholder="25"
              type="number"
            />
          </v-col>
          <v-col cols="12" md="4">
            <v-text-field
              v-model.number="item.max_msg_retries"
              :hint="$t('settings.messengers.retriesHelp')"
              :label="$t('settings.messengers.retries')"
              max="1000"
              min="1"
              name="max_msg_retries"
              persistent-hint
              placeholder="2"
              type="number"
            />
          </v-col>
          <v-col cols="12" md="4">
            <v-text-field
              v-model="item.timeout"
              :hint="$t('settings.messengers.timeoutHelp')"
              :label="$t('settings.messengers.timeout')"
              :maxlength="10"
              name="timeout"
              persistent-hint
              placeholder="5s"
            />
          </v-col>
        </v-row>
      </div>
    </v-card>

    <v-btn color="primary" prepend-icon="mdi-plus" @click="addMessenger">
      {{ $t('globals.buttons.addNew') }}
    </v-btn>
  </div>
</template>

<script>
export default {
  props: {
    form: {
      type: Object, default: () => {},
    },
  },

  data() {
    return {
      data: this.form,
    };
  },

  methods: {
    addMessenger() {
      this.data.messengers.push({
        enabled: true,
        root_url: '',
        name: '',
        username: '',
        password: '',
        max_conns: 25,
        max_msg_retries: 2,
        timeout: '5s',
      });

      this.$nextTick(() => {
        const latest = this.$refs[`messengerName${this.data.messengers.length - 1}`];
        latest?.focus?.();
      });
    },

    removeMessenger(i) {
      this.data.messengers.splice(i, 1);
    },
  },
};
</script>

<style scoped>
.settings-section {
  display: grid;
  gap: 20px;
}

.messenger-card {
  padding: 20px;
}

.card-head,
.toggle-field {
  align-items: start;
  display: flex;
  gap: 16px;
  justify-content: space-between;
}

.section-disabled {
  opacity: 0.65;
}

@media (max-width: 768px) {
  .card-head {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
