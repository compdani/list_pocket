<template>
  <div class="admin-table-shell">
    <div class="table-wrap">
      <v-data-table-server
        v-bind="attrs"
        :headers="headers"
        :items="items"
        :items-length="itemsLength"
        :loading="loading"
        :page="page"
        :items-per-page="itemsPerPage"
        hide-default-footer
      >
        <template v-for="(_, name) in slots" :key="name" #[name]="slotProps">
          <slot :name="name" v-bind="slotProps || {}" />
        </template>
        <template v-if="!slots['no-data']" #no-data>
          <empty-placeholder
            v-if="!loading"
            :icon="emptyIcon"
            :label="emptyLabel"
            :action-label="emptyActionLabel"
            :action-to="emptyActionTo"
          />
        </template>
      </v-data-table-server>
    </div>
    <div v-if="itemsLength > 0" class="table-pagination">
      <v-pagination
        :length="pageCount"
        :model-value="page"
        rounded="circle"
        total-visible="7"
        @update:model-value="$emit('update:page', $event)"
      />
    </div>
  </div>
</template>

<script setup>
import { computed, useAttrs, useSlots } from 'vue';
import EmptyPlaceholder from './EmptyPlaceholder.vue';

defineOptions({ inheritAttrs: false });

const props = defineProps({
  headers: { type: Array, default: () => [] },
  items: { type: Array, default: () => [] },
  itemsLength: { type: Number, default: 0 },
  loading: { type: Boolean, default: false },
  page: { type: Number, default: 1 },
  itemsPerPage: { type: Number, default: 20 },
  emptyIcon: { type: String, default: '' },
  emptyLabel: { type: String, default: '' },
  emptyActionLabel: { type: String, default: '' },
  emptyActionTo: { type: [String, Object], default: null },
});

defineEmits(['update:page']);

const attrs = useAttrs();
const slots = useSlots();

const pageCount = computed(() => {
  if (!props.itemsPerPage || !props.itemsLength) {
    return 1;
  }
  return Math.max(1, Math.ceil(props.itemsLength / props.itemsPerPage));
});
</script>

<style scoped>
.table-wrap {
  overflow-x: auto;
}

.table-pagination {
  align-items: center;
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  margin: 16px 0;
}
</style>
