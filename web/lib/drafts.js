function browserStorage() {
  try { return globalThis.localStorage; } catch { return undefined; }
}

/** Explicit identity in every key prevents drafts leaking between signed-in accounts. */
export function draftKey({ userID, workID, problemID, language }) {
  if (!userID || !workID || !problemID || !language) throw new Error('A draft requires user, work, problem and language.');
  return `campusjudge:draft:v1:${[userID, workID, problemID, language].map(encodeURIComponent).join(':')}`;
}

export function createDraftStore(storage = browserStorage()) {
  const memory = new Map();
  const selectionKey = context => `campusjudge:language:v1:${[context.userID, context.workID, context.problemID].map(encodeURIComponent).join(':')}`;
  function read(key) {
    if (memory.has(key)) return memory.get(key);
    try {
      const value = storage?.getItem(key);
      if (value !== null && value !== undefined) {
        memory.set(key, value);
        return value;
      }
    } catch { /* Existing in-memory drafts remain available when disk storage fails. */ }
    return null;
  }
  function write(key, source) {
    memory.set(key, source);
    try {
      if (!storage) return false;
      storage.setItem(key, source);
      return true;
    } catch {
      return false;
    }
  }
  return {
    get: context => read(draftKey(context)),
    set: (context, source) => write(draftKey(context), source),
    selectedLanguage: context => read(selectionKey(context)),
    selectLanguage: (context, language) => write(selectionKey(context), language),
  };
}
