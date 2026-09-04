import { getCurrentInstance, onBeforeUnmount, onMounted } from 'vue';
import { events } from '../utils/events';

export function usePageRefresh(handler) {
  const instance = getCurrentInstance();
  const bus = instance?.proxy?.$events || events;

  onMounted(() => {
    bus.$on('page.refresh', handler);
  });

  onBeforeUnmount(() => {
    bus.$off('page.refresh', handler);
  });
}

export function emitPageRefresh() {
  const instance = getCurrentInstance();
  const bus = instance?.proxy?.$events || events;
  bus.$emit('page.refresh');
}
