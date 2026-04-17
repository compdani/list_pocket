import { reactive } from 'vue';

// Global reactive queue consumed by <v-snackbar-queue> in App.vue.
// Messages are SnackbarMessage-compatible objects (text, color, timeout...).
export const toastQueue = reactive([]);

const DEFAULT_TIMEOUT_SUCCESS = 3000;
const DEFAULT_TIMEOUT_ERROR = 6000;

function resolveColor(type) {
  if (!type) return 'success';
  const t = String(type).toLowerCase();
  if (t === 'is-danger' || t === 'danger' || t === 'error' || t === 'is-error') {
    return 'error';
  }
  if (t === 'is-warning' || t === 'warning') return 'warning';
  if (t === 'is-info' || t === 'info') return 'info';
  if (t === 'is-success' || t === 'success') return 'success';
  return t;
}

function resolveTimeout(duration, color) {
  if (duration === -1 || duration === 0) return duration;
  if (typeof duration === 'number' && Number.isFinite(duration) && duration > 0) {
    return duration;
  }
  if (typeof duration === 'string' && duration.trim() !== '') {
    const n = Number(duration);
    if (Number.isFinite(n)) return n;
  }
  return color === 'error' ? DEFAULT_TIMEOUT_ERROR : DEFAULT_TIMEOUT_SUCCESS;
}

export function pushToast(msg, type, duration) {
  const color = resolveColor(type);
  const text = typeof msg === 'string' ? msg : String(msg ?? '');
  if (!text) return;
  toastQueue.push({
    text,
    color,
    timeout: resolveTimeout(duration, color),
  });
}

export function clearToasts() {
  toastQueue.splice(0, toastQueue.length);
}
