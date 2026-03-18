<template>
  <div class="list-selector">
    <div :class="['list-tags', ...classes]">
      <div class="d-flex flex-wrap gap-2 mb-3">
        <v-chip
          v-for="l in selectedItems"
          :key="l.id"
          :class="l.subscriptionStatus"
          :closable="!$props.disabled"
          @click:close="removeList(l.id)"
          class="list"
        >
          {{ l.name }}
          <sup v-if="l.optin === 'double' && l.subscriptionStatus">
            {{ $t(`subscribers.status.${l.subscriptionStatus}`) }}
          </sup>
        </v-chip>
      </div>
    </div>

    <div>
      <label v-if="label" :for="inputId" class="d-block mb-2">
        {{ label }}
        <span v-if="selectedItems">
          ({{ selectedItems.length }})
        </span>
      </label>
      <v-autocomplete
        :id="inputId"
        v-model="query"
        :placeholder="placeholder"
        :disabled="all.length === 0 || $props.disabled"
        :items="filteredLists"
        item-title="name"
        item-value="id"
        clearable
        @update:model-value="selectListValue"
      />
      <div v-if="message" class="text-caption text-grey mt-1">
        {{ message }}
      </div>
    </div>
  </div>
</template>

<script>
import { nextTick } from 'vue';

let listSelectorId = 0;

export default {
  name: 'ListSelector',

  props: {
    label: { type: String, default: '' },
    placeholder: { type: String, default: '' },
    message: { type: String, default: '' },
    required: Boolean,
    disabled: Boolean,
    classes: {
      type: Array,
      default: () => [],
    },
    selected: {
      type: Array,
      default: () => [],
    },
    all: {
      type: Array,
      default: () => [],
    },
  },

  data() {
    listSelectorId += 1;

    return {
      inputId: `list-selector-${listSelectorId}`,
      query: '',
      selectedItems: [],
    };
  },

  methods: {
    selectList(l) {
      if (!l) {
        return;
      }
      this.selectedItems.push(l);
      this.query = '';

      // Propagate the items to the parent's v-model binding.
      nextTick(() => {
        this.$emit('input', this.selectedItems);
      });
    },

    selectListValue(value) {
      const item = this.filteredLists.find((l) => l.id === value);
      if (item) {
        this.selectList(item);
      }
    },

    removeList(id) {
      this.selectedItems = this.selectedItems.filter((l) => l.id !== id);

      // Propagate the items to the parent's v-model binding.
      nextTick(() => {
        this.$emit('input', this.selectedItems);
      });
    },
  },

  computed: {
    // Return the list of unselected lists.
    filteredLists() {
      // Get a map of IDs of the user subscriptions. eg: {1: true, 2: true};
      const subIDs = this.selectedItems.reduce((obj, item) => ({ ...obj, [item.id]: true }), {});

      // Filter lists from the global lists whose IDs are not in the user's
      // subscribed ist.
      const q = this.query.toLowerCase();
      return this.$props.all.filter(
        (l) => (!(l.id in subIDs) && l.name.toLowerCase().indexOf(q) >= 0),
      );
    },
  },

  watch: {
    // This is required to update the array of lists to propagate from parent
    // components and "react" on the selector.
    selected() {
      // Deep-copy.
      this.selectedItems = JSON.parse(JSON.stringify(this.selected));
    },
  },

  mounted() {
    if (this.selected) {
      this.selectedItems = JSON.parse(JSON.stringify(this.selected));
    }
  },
};
</script>
