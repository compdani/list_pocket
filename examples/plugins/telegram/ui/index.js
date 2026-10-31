// A plugin UI with no build step: plain ES module, render functions, and the host's
// Vue and Vuetify through the import map. An SFC built with `vue` external works too.
import {
  defineComponent, h, onMounted, ref, resolveComponent,
} from 'vue';

export default function setup(host) {
  const Contacts = defineComponent({
    name: 'TelegramContacts',
    setup() {
      const contacts = ref([]);
      const loading = ref(true);
      const error = ref('');

      onMounted(async () => {
        try {
          contacts.value = (await host.fetch('contacts')).contacts;
        } catch (err) {
          error.value = err.message;
        } finally {
          loading.value = false;
        }
      });

      return () => h('section', { 'data-cy': 'telegram-contacts' }, [
        h('h1', { class: 'text-h5 mb-4' }, host.t('telegram.contacts')),
        loading.value ? h(resolveComponent('v-progress-linear'), { indeterminate: true }) : null,
        error.value ? h(resolveComponent('v-alert'), { type: 'error', text: error.value }) : null,
        !loading.value && !error.value && contacts.value.length === 0
          ? h(resolveComponent('v-alert'), { type: 'info', variant: 'tonal', text: host.t('telegram.contactsEmpty') })
          : null,
        contacts.value.length > 0 ? h(resolveComponent('v-table'), null, () => [
          h('thead', h('tr', [
            h('th', host.t('telegram.chatId')),
            h('th', host.t('telegram.name')),
            h('th', host.t('telegram.subscribed')),
          ])),
          h('tbody', contacts.value.map((c) => h('tr', { key: c.chat_id }, [
            h('td', c.chat_id),
            h('td', c.name),
            h('td', c.first_seen),
          ]))),
        ]) : null,
      ]);
    },
  });

  host.routes.add({
    name: 'contacts',
    path: 'contacts',
    title: 'telegram.contacts',
    component: Contacts,
    permission: 'plugin_telegram:manage',
  });
  host.nav.add({
    route: 'contacts',
    label: host.t('telegram.contacts'),
    icon: 'M9.78 18.65l.28-4.23 7.68-6.92c.34-.31-.07-.46-.52-.19L7.74 13.3 3.64 12c-.88-.25-.89-.86.2-1.3l15.97-6.16c.73-.33 1.43.18 1.15 1.3l-2.72 12.81c-.19.91-.74 1.13-1.5.71L12.6 16.3l-1.99 1.93c-.23.23-.42.42-.83.42z',
    permission: 'plugin_telegram:manage',
  });

  // Used by telegram.get_chat's chat_id. People can still type an ID or a {{ }} expression.
  host.fields.add('chat', defineComponent({
    name: 'TelegramChatField',
    props: ['modelValue', 'label', 'hint', 'readonly'],
    emits: ['update:modelValue'],
    setup(props, { emit }) {
      const chats = ref([]);
      onMounted(async () => {
        try {
          chats.value = (await host.fetch('contacts')).contacts;
        } catch {
          // Without access to the list, this is still a text input.
        }
      });
      return () => h(resolveComponent('v-combobox'), {
        modelValue: props.modelValue,
        label: props.label,
        hint: props.hint,
        persistentHint: true,
        readonly: props.readonly,
        items: chats.value.map((c) => ({ title: `${c.name} (${c.chat_id})`, value: c.chat_id })),
        returnObject: false,
        density: 'compact',
        variant: 'outlined',
        class: 'mb-3',
        'data-cy': 'telegram-chat-field',
        'onUpdate:modelValue': (value) => emit('update:modelValue', value),
      });
    },
  }));

  host.slots.add('subscriber.form.after', defineComponent({
    name: 'TelegramSubscriberNote',
    setup() {
      return () => h(resolveComponent('v-alert'), {
        type: 'info',
        variant: 'tonal',
        density: 'comfortable',
        class: 'my-3',
        'data-cy': 'telegram-note',
        text: host.t('telegram.note'),
      });
    },
  }));
}
