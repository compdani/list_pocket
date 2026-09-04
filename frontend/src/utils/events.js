function createEventBus() {
  const listeners = new Map();

  return {
    $on(event, handler) {
      const handlers = listeners.get(event) || new Set();
      handlers.add(handler);
      listeners.set(event, handlers);
    },

    $off(event, handler) {
      const handlers = listeners.get(event);
      if (!handlers) {
        return;
      }

      if (!handler) {
        listeners.delete(event);
        return;
      }

      handlers.delete(handler);
      if (handlers.size === 0) {
        listeners.delete(event);
      }
    },

    $emit(event, ...args) {
      const handlers = listeners.get(event);
      if (!handlers) {
        return;
      }

      handlers.forEach((handler) => {
        handler(...args);
      });
    },
  };
}

export const events = createEventBus();
