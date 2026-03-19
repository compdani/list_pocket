<template>
  <div class="list-selector">
    <div :class="['list-tags', ...classes]">
      <div class="d-flex flex-wrap gap-2 mb-3">
        <v-chip
          v-for="l in selectedItems"
          :key="getListValue(l)"
          :class="l.subscriptionStatus"
          :closable="!$props.disabled"
          @click:close="removeList(getListValue(l))"
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
        :model-value="pickerValue"
        :search="query"
        :placeholder="placeholder"
        :disabled="all.length === 0 || $props.disabled"
        :items="normalizedLists"
        item-title="name"
        item-value="listValue"
        return-object
        clearable
        @update:search="updateQuery"
        @update:model-value="selectListValue"
      />
      <div v-if="message" class="text-caption text-grey mt-1">
        {{ message }}
      </div>
    </div>
  </div>
</template>

<script>

let listSelectorId = 0;

export default {
  name: 'ListSelector',

  emits: ['input', 'update:modelValue'],

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
    modelValue: {
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
      pickerValue: null,
      query: '',
      selectedItems: [],
    };
  },

  methods: {
    getListValue(list) {
      if (!list) {
        return '';
      }
      if (list.id !== undefined && list.id !== null) {
        return String(list.id);
      }
      return '';
    },

    emitSelection() {
      this.$emit('input', this.selectedItems);
      this.$emit('update:modelValue', this.selectedItems);
    },

    updateQuery(value) {
      this.query = typeof value === 'string' ? value : '';
    },

    selectList(l) {
      if (!l) {
        return;
      }
      const listValue = this.getListValue(l);
      if (this.selectedItems.some((item) => this.getListValue(item) === listValue)) {
        this.pickerValue = null;
        this.query = '';
        return;
      }

      this.selectedItems = [...this.selectedItems, l];
      this.pickerValue = null;
      this.query = '';

      this.emitSelection();
    },

    selectListValue(value) {
      if (value === null || value === undefined || value === '') {
        return;
      }

      const item = typeof value === 'object'
        ? value
        : this.normalizedLists.find((l) => this.getListValue(l) === String(value));
      if (item) {
        this.selectList(item);
        return;
      }

      this.pickerValue = null;
    },

    removeList(listValue) {
      this.selectedItems = this.selectedItems.filter((l) => this.getListValue(l) !== listValue);

      this.emitSelection();
    },
  },

  computed: {
    normalizedLists() {
      return this.filteredLists.map((list) => ({
        ...list,
        listValue: this.getListValue(list),
      }));
    },

    // Return the list of unselected lists.
    filteredLists() {
      const selectedValues = new Set(this.selectedItems.map((item) => this.getListValue(item)));

      // Filter lists from the global lists whose IDs are not in the user's
      // subscribed ist.
      const q = typeof this.query === 'string' ? this.query.toLowerCase() : '';
      return this.$props.all.filter(
        (l) => (!selectedValues.has(this.getListValue(l)) && l.name.toLowerCase().indexOf(q) >= 0),
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

    modelValue() {
      this.selectedItems = JSON.parse(JSON.stringify(this.modelValue));
    },
  },

  mounted() {
    if (this.modelValue && this.modelValue.length) {
      this.selectedItems = JSON.parse(JSON.stringify(this.modelValue));
    } else if (this.selected) {
      this.selectedItems = JSON.parse(JSON.stringify(this.selected));
    }
  },
};
</script>
