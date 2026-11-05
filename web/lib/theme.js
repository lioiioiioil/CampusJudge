const themeKey = 'campusjudge:theme';

function browserStorage() {
  try { return globalThis.localStorage; } catch { return undefined; }
}

export function preferredTheme(storage = browserStorage(), darkSystem = false) {
  try {
    const stored = storage?.getItem(themeKey);
    if (stored === 'dark' || stored === 'light') return stored;
  } catch { /* Storage can be unavailable in private or restricted browsers. */ }
  return darkSystem ? 'dark' : 'light';
}

export function currentTheme(root = globalThis.document?.documentElement) {
  return root?.dataset.theme === 'dark' ? 'dark' : 'light';
}

export function initializeTheme({ root = globalThis.document?.documentElement,
  storage = browserStorage(), darkSystem = globalThis.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false } = {}) {
  const theme = preferredTheme(storage, darkSystem);
  if (root) root.dataset.theme = theme;
  return theme;
}

export function toggleTheme({ root = globalThis.document?.documentElement, storage = browserStorage() } = {}) {
  const theme = currentTheme(root) === 'dark' ? 'light' : 'dark';
  if (root) root.dataset.theme = theme;
  try { storage?.setItem(themeKey, theme); } catch { /* The current theme still works without persistence. */ }
  return theme;
}
