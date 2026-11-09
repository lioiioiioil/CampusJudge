import test from 'node:test';
import assert from 'node:assert/strict';
import { createController } from '../lib/actions.js';
import { createEditor } from '../lib/editor.js';
import { createDraftStore } from '../lib/drafts.js';
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

function harness(t, api, { authenticated = true, editor } = {}) {
  const listeners = {};
  const windowListeners = {};
  const timers = new Map();
  let nextTimer = 0;
  const root = { innerHTML: '', querySelector() { return null; } };
  const dialog = { open: false, close() { this.open = false; }, showModal() { this.open = true; },
    addEventListener() {}, classList: { toggle() {} } };
  const toast = { textContent: '', className: '', classList: { remove() {} } };
  const fakeGlobals = {
    document: { addEventListener(name, callback) { listeners[name] = callback; } },
    window: { addEventListener(name, callback) { windowListeners[name] = callback; } },
    history: { replaceState() {} }, location: { hash: '#overview' },
    FormData: FakeFormData,
    setTimeout(callback, delay) { const id = ++nextTimer; timers.set(id, { callback, delay }); return id; },
    clearTimeout(id) { timers.delete(id); },
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
  let contentRenders = 0;
  const controller = createController({ view, api, root, dialog, toastNode: toast, editor,
    render() { renders.push(view.snapshot?.user.id || null); }, renderContent() { contentRenders++; } });
  controller.attach();
  function click(action) {
    const element = { dataset: { action }, disabled: false, isConnected: true };
    return listeners.click({ target: { closest: () => element }, preventDefault() {} });
  }
  function submit(name, values, context = {}, testRows = {}) {
    const submitButton = { innerHTML: 'Сохранить', isConnected: true };
    const error = { textContent: '' };
    const form = {
      dataset: { form: name, ...context }, values,
      querySelector: selector => selector === '.form-error' ? error : submitButton,
      querySelectorAll: selector => testRows[selector] || [],
    };
    return listeners.submit({ target: { closest: () => form }, preventDefault() {} });
  }
  function firePoll() {
    const timer = [...timers].find(([, item]) => item.delay === 1200);
    assert.ok(timer, 'no polling timer was scheduled');
    timers.delete(timer[0]);
    return timer[1].callback();
  }
  return { view, root, dialog, toast, renders, click, submit, controller, firePoll,
    fireWindow: (name, event = {}) => windowListeners[name]?.(event),
    pendingPolls: () => [...timers.values()].filter(item => item.delay === 1200).length,
    contentRenders: () => contentRenders };

}

test('saving a problem copy creates a new problem with original unchanged test bytes', async t => {
  const requests = [];
  const h = harness(t, {
    request: async (...args) => { requests.push(args); },
    snapshot: async () => snapshot(),
  });
  const row = (input, output, original) => ({
    dataset: original ? { originalTest: JSON.stringify(original) } : {},
    querySelector: selector => ({ value: selector.endsWith('_input"]') ? input : output }),
  });
  await h.submit('problem', { title: 'Копия', statement: 'Сложите числа.', time_limit_ms: '1000', memory_limit_mb: '128' }, {}, {
    '[data-test="sample"]': [row('\n2 3\n', '5\n', { input: '\r\n2 3\r\n', output: '5\r\n' }), row('', '')],
    '[data-test="hidden"]': [row('', '', { input: '', output: '' }), row('8 9', '17', { input: '8 9', output: 'old' })],
  });
  assert.deepEqual(requests, [['/api/problems', { method: 'POST', body: {
    title: 'Копия', statement: 'Сложите числа.', input_format: '', output_format: '',
    time_limit_ms: 1000, memory_limit_mb: 128,
    samples: [{ input: '\r\n2 3\r\n', output: '5\r\n' }],
    tests: [{ input: '', output: '' }, { input: '8 9', output: '17' }],
  } }]]);
});

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

function judgedSnapshot(status, verdict = 'pending') {
  const result = snapshot('student');
  result.submissions = [{ id: 's', user_id: 'student', work_id: 'w', problem_id: 'p',
    created_at: '2026-11-05T10:00:00Z', language: 'cpp', verdict, feedback: '',
    judging: { status, passed_tests: status === 'done' ? 2 : 0, total_tests: 2 } }];
  return result;
}

test('automatic result polling patches results without replacing open editor/form DOM', async t => {
  let reads = 0;
  const h = harness(t, { snapshot: async () => ++reads === 1 ? judgedSnapshot('queued') : judgedSnapshot('done', 'accepted') });
  await h.click('refresh');
  h.dialog.open = true;
  h.dialog.innerHTML = 'unchanged editor and dirty form';
  const result = { dataset: { work: 'w', problem: 'p' }, innerHTML: '' };
  h.dialog.querySelectorAll = selector => selector === '[data-editor-result]' ? [result] : [];
  const renders = h.renders.length;
  await h.firePoll();
  assert.equal(h.renders.length, renders);
  assert.equal(h.dialog.innerHTML, 'unchanged editor and dirty form');
  assert.equal(h.dialog.open, true);
  assert.equal(h.contentRenders(), 1);
  assert.match(result.innerHTML, /Зачтено/);
  assert.equal(h.pendingPolls(), 0);
});

test('logout cancels polling and discards an already running poll response', async t => {
  const late = deferred();
  let reads = 0;
  const h = harness(t, { snapshot: () => ++reads === 1 ? Promise.resolve(judgedSnapshot('running')) : late.promise,
    logout: async () => ({}) });
  await h.click('refresh');
  const poll = h.firePoll();
  await h.click('logout');
  late.resolve(judgedSnapshot('done', 'accepted'));
  await poll;
  assert.equal(h.view.snapshot, null);
  assert.equal(h.pendingPolls(), 0);
  assert.equal(h.contentRenders(), 0);
});

test('submission keeps the editor open while queued result polling starts', async t => {
  const h = harness(t, { request: async () => ({ id: 's' }), snapshot: async () => judgedSnapshot('queued') });
  acceptSnapshot(h.view, snapshot('student'));
  h.dialog.open = true;
  await h.submit('submission', { language: 'cpp', source: 'int main() {}' }, { work: 'w', problem: 'p' });
  assert.equal(h.dialog.open, true);
  assert.equal(h.renders.length, 0);
  assert.equal(h.contentRenders(), 1);
  assert.equal(h.pendingPolls(), 1);
});


test('BFCache return quietly reloads and resumes an active judging queue', async t => {
  let reads = 0;
  const h = harness(t, { snapshot: async () => ++reads < 3 ? judgedSnapshot('running') : judgedSnapshot('done', 'accepted') });
  await h.click('refresh');
  assert.equal(h.pendingPolls(), 1);
  h.fireWindow('pagehide');
  assert.equal(h.pendingPolls(), 0);
  const renders = h.renders.length;
  await h.fireWindow('pageshow', { persisted: true });
  assert.equal(h.pendingPolls(), 1);
  assert.equal(h.renders.length, renders, 'restoring must not destroy the editor/form DOM');
  await h.firePoll();
  assert.equal(h.view.snapshot.submissions[0].verdict, 'accepted');
  assert.equal(h.pendingPolls(), 0);
});

test('expired BFCache session returns to login instead of resuming old polling', async t => {
  let reads = 0;
  const h = harness(t, { snapshot: async () => {
    if (++reads === 1) return judgedSnapshot('running');
    throw new ApiError('Expired while away', 401);
  } });
  await h.click('refresh');
  h.fireWindow('pagehide');
  await h.fireWindow('pageshow', { persisted: true });
  assert.equal(h.view.snapshot, null);
  assert.equal(h.pendingPolls(), 0);
  assert.equal(h.toast.textContent, 'Сессия завершилась. Войдите снова.');
});

test('late BFCache restoration response cannot restore a user after logout', async t => {
  const late = deferred();
  let reads = 0;
  const h = harness(t, { snapshot: () => ++reads === 1 ? Promise.resolve(judgedSnapshot('running')) : late.promise,
    logout: async () => ({}) });
  await h.click('refresh');
  h.fireWindow('pagehide');
  const restoring = h.fireWindow('pageshow', { persisted: true });
  await h.click('logout');
  late.resolve(judgedSnapshot('running'));
  await restoring;
  assert.equal(h.view.snapshot, null);
  assert.equal(h.pendingPolls(), 0);
});


test('quiet identity change saves and closes the old editor before installing another account', async t => {
  const disk = new Map();
  const drafts = createDraftStore({ getItem: key => disk.get(key) ?? null, setItem: (key, value) => disk.set(key, value) });
  const a = snapshot('teacher');
  a.submissions = [{ ...judgedSnapshot('running').submissions[0], user_id: 'teacher' }];
  let reads = 0;
  const h = harness(t, { snapshot: async () => ++reads === 1 ? a : judgedSnapshot('running') },
    { editor: createEditor({ drafts }) });
  await h.click('refresh');
  const oldForm = {
    dataset: { form: 'submission', work: 'w', problem: 'p', currentLanguage: 'cpp' },
    elements: { source: { value: 'private code belonging to A' }, language: { value: 'cpp' } },
    querySelector: () => null,
  };
  h.dialog.open = true;
  h.dialog.innerHTML = 'editor belonging to A';
  h.dialog.querySelector = () => oldForm;
  h.view.page = 'submissions';
  h.view.groupFilter = 'old-group';
  const renders = h.renders.length;
  await h.firePoll();
  assert.equal(h.view.snapshot.user.id, 'student');
  assert.equal(h.view.page, 'overview');
  assert.equal(h.view.groupFilter, '');
  assert.equal(h.dialog.open, false);
  assert.equal(h.dialog.innerHTML, '');
  assert.equal(h.renders.length, renders + 1, 'a changed identity needs a complete header/view render');
  const scope = { workID: 'w', problemID: 'p', language: 'cpp' };
  assert.equal(drafts.get({ ...scope, userID: 'teacher' }), 'private code belonging to A');
  assert.equal(drafts.get({ ...scope, userID: 'student' }), null);
});

test('explicit refresh also resets account-scoped navigation on a cookie identity change', async t => {
  const h = harness(t, { snapshot: async () => snapshot('student') });
  h.view.page = 'students';
  h.view.search = 'old teacher search';
  await h.click('refresh');
  assert.equal(h.view.snapshot.user.id, 'student');
  assert.equal(h.view.page, 'overview');
  assert.equal(h.view.search, '');
  assert.equal(h.renders.at(-1), 'student');
});
