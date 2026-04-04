const STORAGE_KEY = 'listpocket_ai_builder_threads_v1';

function readPersisted() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return { bySessionKey: {} };
    }
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object' || !parsed.bySessionKey) {
      return { bySessionKey: {} };
    }
    return { bySessionKey: parsed.bySessionKey };
  } catch {
    return { bySessionKey: {} };
  }
}

function writePersisted(bySessionKey) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ bySessionKey }));
  } catch (e) {
    console.warn('AI builder threads: could not persist', e);
  }
}

function newThreadId() {
  return `th-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

function emptyThread() {
  const id = newThreadId();
  const now = Date.now();
  return {
    id,
    title: 'New chat',
    createdAt: now,
    updatedAt: now,
    messages: [],
    pictures: [],
  };
}

export default {
  namespaced: true,

  state: () => readPersisted(),

  mutations: {
    ensureSession(state, sessionKey) {
      if (!sessionKey) {
        return;
      }
      if (!state.bySessionKey[sessionKey]) {
        const t = emptyThread();
        state.bySessionKey[sessionKey] = {
          threads: [t],
          activeThreadId: t.id,
        };
        writePersisted(state.bySessionKey);
      }
    },

    setActiveThread(state, { sessionKey, threadId }) {
      const sess = state.bySessionKey[sessionKey];
      if (!sess || !threadId) {
        return;
      }
      if (!sess.threads.some((t) => t.id === threadId)) {
        return;
      }
      sess.activeThreadId = threadId;
      writePersisted(state.bySessionKey);
    },

    addThread(state, { sessionKey, thread }) {
      const sess = state.bySessionKey[sessionKey];
      if (!sess) {
        return;
      }
      sess.threads.unshift(thread);
      sess.activeThreadId = thread.id;
      writePersisted(state.bySessionKey);
    },

    removeThread(state, { sessionKey, threadId }) {
      const sess = state.bySessionKey[sessionKey];
      if (!sess) {
        return;
      }
      sess.threads = sess.threads.filter((t) => t.id !== threadId);
      if (sess.activeThreadId === threadId) {
        sess.activeThreadId = sess.threads[0]?.id || null;
      }
      if (sess.threads.length === 0) {
        const t = emptyThread();
        sess.threads = [t];
        sess.activeThreadId = t.id;
      }
      writePersisted(state.bySessionKey);
    },

    saveThreadData(state, { sessionKey, threadId, messages, pictures, title }) {
      const sess = state.bySessionKey[sessionKey];
      if (!sess || !threadId) {
        return;
      }
      const t = sess.threads.find((x) => x.id === threadId);
      if (!t) {
        return;
      }
      t.messages = Array.isArray(messages) ? messages : [];
      t.pictures = Array.isArray(pictures) ? pictures : [];
      if (title && String(title).trim()) {
        t.title = String(title).trim();
      }
      t.updatedAt = Date.now();
      writePersisted(state.bySessionKey);
    },
  },
};
