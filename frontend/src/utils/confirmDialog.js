import { reactive } from 'vue';

const queue = [];

export const confirmState = reactive({
  open: false,
  mode: 'confirm',
  title: '',
  message: '',
  confirmText: '',
  inputValue: '',
  inputPlaceholder: '',
  resolve: null,
});

function showNext() {
  if (confirmState.open || queue.length === 0) {
    return;
  }

  const next = queue.shift();
  confirmState.mode = next.mode;
  confirmState.title = next.title || '';
  confirmState.message = next.message || '';
  confirmState.confirmText = next.confirmText || '';
  confirmState.inputValue = next.inputValue || '';
  confirmState.inputPlaceholder = next.inputPlaceholder || '';
  confirmState.resolve = next.resolve;
  confirmState.open = true;
}

export function openConfirm({
  message = '',
  title = '',
  confirmText = '',
} = {}) {
  return new Promise((resolve) => {
    queue.push({
      mode: 'confirm',
      message,
      title,
      confirmText,
      resolve,
    });
    showNext();
  });
}

export function openPrompt({
  message = '',
  title = '',
  confirmText = '',
  value = '',
  placeholder = '',
} = {}) {
  return new Promise((resolve) => {
    queue.push({
      mode: 'prompt',
      message,
      title,
      confirmText,
      inputValue: value,
      inputPlaceholder: placeholder,
      resolve,
    });
    showNext();
  });
}

export function settleConfirm(result) {
  const { resolve } = confirmState;
  confirmState.open = false;
  confirmState.resolve = null;
  if (typeof resolve === 'function') {
    resolve(result);
  }
  showNext();
}
