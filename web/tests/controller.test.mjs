import test from 'node:test';
import assert from 'node:assert/strict';
import { createController } from '../lib/actions.js';
import { ApiError } from '../lib/api.js';
import { createViewState, acceptSnapshot } from '../lib/state.js';

function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

function snapshot(id = 'teacher') {
  return {
    user: { id, name: id, username: id, role: id === 'teacher' ? 'teacher' : 'student' },
    users: [], groups: [], topics: [], problems: [], works: [], submissions: [], languages: [], demo: true,
  };
}

class FakeFormData {
  constructor(form) { this.values = form.values; }
  [Symbol.iterator]() { return Object.entries(this.values)[Symbol.iterator](); }
  getAll(name) {
    const value = this.values[name];
    return value === undefined ? [] : Array.isArray(value) ? value : [value];
  }
}

function harness(t, api, { authenticated = true } = {}) {
  const listeners = {};
  const root = { innerHTML: '', querySelector() { return null; } };
  const dialog = { open: false, close() { this.open = false; }, showModal() { this.open = true; },
    addEventListener() {}, classList: { toggle() {} } };
  const toast = { textContent: '', className: '', classList: { remove() {} } };
  const fakeGlobals = {
    document: { addEventListener(name, callback) { listeners[name] = callback; } },
    window: { addEventListener() {} },
    history: { replaceState() {} }, location: { hash: '#overview' },
    FormData: FakeFormData,
    setTimeout: () => 0, clearTimeout() {},
  };
  for (const [name, value] of Object.entries(fakeGlobals)) {
    const previous = Object.getOwnPropertyDescriptor(globalThis, name);
    Object.defineProperty(globalThis, name, { configurable: true, writable: true, value });
    t.after(() => {
      if (previous) Object.defineProperty(globalThis, name, previous);
      else delete globalThis[name];
    });
  }
  const view = createViewState();
  if (authenticated) acceptSnapshot(view, snapshot());
  const renders = [];
  const controller = createController({ view, api, root, dialog, toastNode: toast,
    render() { renders.push(view.snapshot?.user.id || null); }, renderContent() {} });
  controller.attach();
  function click(action) {
    const element = { dataset: { action }, disabled: false, isConnected: true };
    return listeners.click({ target: { closest: () => element }, preventDefault() {} });
  }
  function submit(name, values) {
    const submitButton = { innerHTML: 'Сохранить', isConnected: true };
    const error = { textContent: '' };
    const form = {
      dataset: { form: name }, values,
      querySelector: selector => selector === '.form-error' ? error : submitButton,
      querySelectorAll: () => [],
    };
    return listeners.submit({ target: { closest: () => form }, preventDefault() {} });
  }
  return { view, root, dialog, toast, renders, click, submit, controller };
}

test('late refresh cannot restore a logged-out user or show a stale success toast', async t => {
  const oldRead = deferred();
  const h = harness(t, { snapshot: () => oldRead.promise, logout: async () => ({ ok: true }) });
  const refresh = h.click('refresh');
  await h.click('logout');
  assert.equal(h.view.snapshot, null);
  const renderCount = h.renders.length;
  oldRead.resolve(snapshot());
  await refresh;
  assert.equal(h.view.snapshot, null);
  assert.equal(h.renders.length, renderCount);
  assert.equal(h.toast.textContent, '');
});

test('late old snapshot cannot overwrite the new login', async t => {
  const oldRead = deferred();
  let reads = 0;
  const h = harness(t, {
    snapshot: () => ++reads === 1 ? oldRead.promise : Promise.resolve(snapshot('new-student')),
    request: async () => ({}),
  });
  const refresh = h.click('refresh');
  await h.submit('login', { username: 'new-student', password: 'password' });
  oldRead.resolve(snapshot());
  await refresh;
  assert.equal(h.view.snapshot.user.id, 'new-student');
  assert.equal(h.toast.textContent, 'Добро пожаловать!');
});

test('old refresh 401 does not clear a newer authenticated session', async t => {
  const oldRead = deferred();
  let reads = 0;
  const h = harness(t, {
    snapshot: () => ++reads === 1 ? oldRead.promise : Promise.resolve(snapshot('new-student')),
    request: async () => ({}),
  });
  const refresh = h.click('refresh');
  await h.submit('login', { username: 'new-student', password: 'password' });
  const renderCount = h.renders.length;
  oldRead.reject(new ApiError('Old session expired', 401));
  await refresh;
  assert.equal(h.view.snapshot.user.id, 'new-student');
  assert.equal(h.renders.length, renderCount);
  assert.equal(h.toast.textContent, 'Добро пожаловать!');
});

test('saved mutation followed by current snapshot 401 clears authentication', async t => {
  const h = harness(t, {
    request: async () => ({ id: 'saved-topic' }),
    snapshot: async () => { throw new ApiError('Expired', 401); },
  });
  await h.submit('topic', { title: 'Тема', description: '', lesson_url: '' });
  assert.equal(h.view.snapshot, null);
  assert.equal(h.renders.at(-1), null);
  assert.equal(h.toast.textContent, 'Сессия завершилась. Войдите снова.');
  assert.doesNotMatch(h.toast.textContent, /Изменение сохранено/);
});

test('successful login with failed first snapshot offers recovery without repeating login', async t => {
  let requests = 0, reads = 0;
  const h = harness(t, {
    request: async () => { requests++; return {}; },
    snapshot: async () => {
      if (++reads === 1) throw new ApiError('Offline');
      return snapshot('student');
    },
    session: async () => ({ user: { id: 'student' }, demo: true }),
  }, { authenticated: false });
  await h.submit('login', { username: 'student', password: 'password' });
  assert.match(h.root.innerHTML, /data-action="retry-init"/);
  assert.doesNotMatch(h.toast.textContent, /Обновить данные/);
  await h.click('retry-init');
  assert.equal(h.view.snapshot.user.id, 'student');
  assert.equal(requests, 1);
});

test('session-changing requests are serialized so the later login owns both cookie and screen', async t => {
  const oldLogin = deferred();
  const started = deferred();
  let requests = 0, reads = 0;
  const h = harness(t, {
    request: () => {
      requests++;
      if (requests === 1) { started.resolve(); return oldLogin.promise; }
      return Promise.resolve({});
    },
    snapshot: async () => { reads++; return snapshot('new-student'); },
  }, { authenticated: false });
  const firstLogin = h.submit('login', { username: 'old-user', password: 'password' });
  await started.promise;
  const nextLogin = h.submit('login', { username: 'new-student', password: 'password' });
  assert.equal(requests, 1, 'another cookie-changing request started before the old response');
  oldLogin.resolve({});
  await firstLogin;
  await nextLogin;
  assert.equal(requests, 2);
  assert.equal(h.view.snapshot.user.id, 'new-student');
  assert.equal(reads, 1);
  assert.equal(h.toast.textContent, 'Добро пожаловать!');
});

test('older same-session snapshot cannot overwrite data loaded after a mutation', async t => {
  const oldRead = deferred();
  let reads = 0;
  const newer = snapshot();
  newer.topics = [{ id: 'new-topic', title: 'Новая тема' }];
  const h = harness(t, {
    request: async () => ({ id: 'new-topic' }),
    snapshot: () => ++reads === 1 ? oldRead.promise : Promise.resolve(newer),
  });
  const refresh = h.click('refresh');
  await h.submit('topic', { title: 'Новая тема', description: '', lesson_url: '' });
  oldRead.resolve(snapshot());
  await refresh;
  assert.equal(h.view.snapshot.topics[0].id, 'new-topic');
});

test('older same-session snapshot error cannot discard a newer successful snapshot', async t => {
  const oldRead = deferred();
  let reads = 0;
  const h = harness(t, {
    request: async () => ({ id: 'new-topic' }),
    snapshot: () => ++reads === 1 ? oldRead.promise : Promise.resolve(snapshot()),
  });
  const refresh = h.click('refresh');
  await h.submit('topic', { title: 'Новая тема', description: '', lesson_url: '' });
  oldRead.reject(new ApiError('Old response', 401));
  await refresh;
  assert.equal(h.view.snapshot.user.id, 'teacher');
});

test('new login waits for the logout response before replacing the session cookie', async t => {
  const logoutResponse = deferred();
  let loginRequests = 0;
  const h = harness(t, {
    logout: () => logoutResponse.promise,
    request: async () => { loginRequests++; return {}; },
    snapshot: async () => snapshot('student'),
  });
  const logout = h.click('logout');
  const login = h.submit('login', { username: 'student', password: 'password' });
  assert.equal(loginRequests, 0, 'login was sent before logout could clear its old cookie');
  logoutResponse.resolve({ ok: true });
  await logout;
  await login;
  assert.equal(loginRequests, 1);
  assert.equal(h.view.snapshot.user.id, 'student');
});
