<template>
  <section v-if="channels.length > 0" class="subscriber-channels mt-4" data-cy="subscriber-channels">
    <h5 class="text-subtitle-1 mb-2">{{ $t('plugins.channelsTitle') }}</h5>
    <v-card v-for="info in channels" :key="info.channel" variant="outlined" class="mb-3">
      <v-card-title class="text-subtitle-2">{{ labelFor(info.channel) }}</v-card-title>
      <v-card-text>
        <template v-if="usesOwnAddress(info.channel)">
          <div class="d-flex flex-wrap ga-2 mb-3">
            <v-chip
              v-for="item in info.addresses"
              :key="item.address"
              :color="item.status === 'active' ? undefined : 'error'"
              :closable="canManage"
              size="small"
              @click:close="removeAddress(info.channel, item.address)"
            >
              {{ item.address }}
            </v-chip>
            <span v-if="info.addresses.length === 0" class="text-body-2 text-medium-emphasis">
              {{ $t('plugins.noAddress') }}
            </span>
          </div>
          <div v-if="canManage" class="d-flex ga-2 mb-3">
            <v-text-field
              v-model="drafts[info.channel]"
              :label="addressLabelFor(info.channel)"
              hide-details="auto"
              variant="outlined"
              density="compact"
              :data-cy="`channel-address-${info.channel}`"
            />
            <v-btn
              variant="tonal"
              :disabled="!drafts[info.channel]"
              :data-cy="`channel-address-add-${info.channel}`"
              @click="addAddress(info.channel)"
            >
              {{ $t('globals.buttons.add') }}
            </v-btn>
          </div>
        </template>

        <p v-if="info.consent.length === 0" class="text-body-2 text-medium-emphasis mb-0">
          {{ $t('plugins.noLists') }}
        </p>
        <v-switch
          v-for="row in info.consent"
          :key="row.list_id"
          :model-value="row.status === 'confirmed'"
          :label="row.list_name"
          :disabled="!canManage"
          density="compact"
          hide-details
          color="primary"
          :data-cy="`channel-consent-${info.channel}-${row.list_id}`"
          @update:model-value="setConsent(info.channel, row.list_id, $event)"
        />
      </v-card-text>
    </v-card>
  </section>
</template>

<script setup>
import {
  computed, getCurrentInstance, onMounted, ref, watch,
} from 'vue';
import { useStore } from 'vuex';
import {
  addSubscriberChannelAddress,
  getSubscriberChannels,
  removeSubscriberChannelAddress,
  setSubscriberChannelConsent,
} from '../api';
import { findChannel } from '../utils/channels';

const props = defineProps({
  subscriberId: { type: String, required: true },
});

const { proxy } = getCurrentInstance();
const store = useStore();
const channels = ref([]);
const drafts = ref({});

const canManage = computed(() => proxy.$can('subscribers:manage'));
const config = computed(() => store.state.serverConfig?.channels || []);

function labelFor(id) {
  return findChannel(config.value, id)?.label || id;
}

function addressLabelFor(id) {
  return findChannel(config.value, id)?.address_label || proxy.$t('plugins.address');
}

function usesOwnAddress(id) {
  return findChannel(config.value, id)?.address_source !== 'phone';
}

function showError(err) {
  proxy.$utils.toast((err.response && err.response.message) || err.message, 'error');
}

async function load() {
  try {
    channels.value = (await getSubscriberChannels(props.subscriberId)) || [];
  } catch (err) {
    showError(err);
  }
}

async function run(action) {
  try {
    await action();
  } catch (err) {
    showError(err);
  }
  await load();
}

function addAddress(channel) {
  const address = drafts.value[channel];
  drafts.value[channel] = '';
  return run(() => addSubscriberChannelAddress(props.subscriberId, channel, address));
}

function removeAddress(channel, address) {
  return run(() => removeSubscriberChannelAddress(props.subscriberId, channel, address));
}

function setConsent(channel, listId, confirmed) {
  return run(() => setSubscriberChannelConsent(
    props.subscriberId,
    channel,
    listId,
    confirmed ? 'confirmed' : 'unsubscribed',
  ));
}

watch(() => props.subscriberId, load);
onMounted(load);
</script>
