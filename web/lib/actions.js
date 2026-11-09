import { createEditor } from './editor.js';
import { toggleTheme } from './theme.js';
import { hasActiveJudging, patchJudgingResults } from './judging.js';
import { ApiError } from './api.js';
import { esc, icon } from './format.js';
import { acceptSnapshot, navigate, resetFilters, resetSession, findByID } from './state.js';
import { buildFormCommand, sourceLimitBytes } from './forms.js';
import { newTopicModal, newGroupModal, newProblemModal, newWorkModal, editWorkModal, testPair,
  topicModal, workModal, problemModal, submissionModal, standingsModal, testRevisionModal, workTestRevisionModal } from '../views/modals.js';

function readFields(form) {
  const values = new FormData(form);
  const fields = Object.fromEntries(values);
  fields.group_ids = values.getAll('group_ids');
  fields.problem_ids = values.getAll('problem_ids');
  const readTests = kind => Array.from(form.querySelectorAll(`[data-test="${kind}"]`))
    .map(element => {
      const original = element.dataset.originalTest ? JSON.parse(element.dataset.originalTest) : null;
      const read = name => {
        const value = element.querySelector(`[name="${kind}_${name}"]`).value;
        // The browser normalizes textarea line endings. Retain unchanged test bytes.
        return original?.[name]?.replace(/\r\n?/g, '\n').replace(/\0/g, '\uFFFD') === value ? original[name] : value;
      };
      return { input: read('input'), output: read('output'), existing: original !== null };
    });
  fields.samples = readTests('sample');
  fields.tests = readTests('hidden');
  return fields;
}

/** Owns browser effects: event listeners, dialog, HTTP and explicit view transitions. */
export function createController({ view, api, root, dialog, toastNode, render, renderContent,
  editor = createEditor() }) {
  let toastTimer;
  let sessionGeneration = 0;
  let latestSnapshotRequest = 0;
  let sessionChange = Promise.resolve();
  let judgeTimer = null;
  let pollErrorShown = false;
  const isCurrent = generation => generation === sessionGeneration;

  // Cookie changes are ordered as HTTP effects: a late Set-Cookie from an old
  // logout or login must finish before the next identity installs its cookie.
  function changeSession(operation) {
    const pending = sessionChange.catch(() => {}).then(operation);
    sessionChange = pending;
    return pending;
  }


  function toast(message, error = false) {
    clearTimeout(toastTimer);
    toastNode.textContent = message;
    toastNode.className = `toast visible${error ? ' error' : ''}`;
    toastTimer = setTimeout(() => toastNode.classList.remove('visible'), 4500);
  }

  function closeModal() {
    const form = dialog.querySelector?.('form[data-form="submission"]');
    if (form && view.snapshot) editor.save(form, view.snapshot);
    dialog.close();
  }

  function stopPolling() {
    clearTimeout(judgeTimer);
    judgeTimer = null;
  }

  function schedulePolling() {
    if (!hasActiveJudging(view.snapshot)) { stopPolling(); return; }
    if (judgeTimer !== null) return;
    const generation = sessionGeneration;
    judgeTimer = setTimeout(async () => {
      judgeTimer = null;
      try {
        await refresh(undefined, generation, { quiet: true });
      } catch (error) {
        if (!isCurrent(generation)) return;
        if (error instanceof ApiError && error.status === 401) reportError(error, generation);
        else if (!pollErrorShown) {
          pollErrorShown = true;
          toast('Результаты пока не обновляются: нет связи с сервером.', true);
        }
      } finally {
        if (isCurrent(generation)) schedulePolling();
      }
    }, 1200);
  }

  function openModal(spec) {
    if (!spec) return;
    dialog.classList.toggle('wide', Boolean(spec.wide));
    dialog.innerHTML = `<div class="modal-header">
      <div><h2 id="modal-title">${esc(spec.title)}</h2>
        ${spec.subtitle ? `<p class="muted">${esc(spec.subtitle)}</p>` : ''}
      </div>
      <button class="icon-btn" data-action="close-modal" aria-label="Закрыть окно">${icon('close', 18)}</button>
    </div><div class="modal-body">${spec.body}</div>`;
    if (!dialog.open) dialog.showModal();
    dialog.scrollTop = 0;
    editor.mount(dialog.querySelector?.('form[data-form="submission"]'), view.snapshot);
  }

  function updateHash() {
    history.replaceState(null, '', `#${view.page}`);
  }

  function reportError(error, generation) {
    if (!isCurrent(generation)) return;
    if (error instanceof ApiError && error.status === 401) {
      sessionGeneration++;
      stopPolling();
      closeModal();
      resetSession(view);
      updateHash();
      render();
      toast('Сессия завершилась. Войдите снова.', true);
      return;
    }
    toast(error.message, true);
  }

  function showRecovery(error) {
    root.innerHTML = `<div class="initial-loading"><div class="startup-error">
      <h2>Не удалось открыть пространство</h2><p class="startup-error-copy">${esc(error.message)}</p>
      <button class="btn primary" data-action="retry-init">Попробовать снова</button>
    </div></div>`;
  }

  async function refresh(options, generation = sessionGeneration, { quiet = false } = {}) {
    const request = ++latestSnapshotRequest;
    const ownsResult = () => isCurrent(generation) && request === latestSnapshotRequest;
    try {
      const snapshot = await api.snapshot();
      if (!ownsResult()) return false;
      if (view.snapshot && view.snapshot.user.id !== snapshot.user.id) {
        // Another tab may replace the shared cookie. The old editor must leave
        // under its original identity before any subsequent input can save a draft.
        closeModal();
        dialog.innerHTML = '';
        sessionGeneration++;
        stopPolling();
        resetSession(view);
        acceptSnapshot(view, snapshot, { newSession: true });
        updateHash();
        render();
        schedulePolling();
        return false;
      }
      acceptSnapshot(view, snapshot, options);
      pollErrorShown = false;
      if (quiet) {
        renderContent();
        patchJudgingResults(dialog, view.snapshot);
      } else render();
      schedulePolling();
      return true;
    } catch (error) {
      if (!ownsResult()) return false;
      throw error;
    }
  }

  async function start() {
    const generation = ++sessionGeneration;
    stopPolling();
    try {
      await sessionChange.catch(() => {});
      if (!isCurrent(generation)) return;
      const session = await api.session();
      if (!isCurrent(generation)) return;
      view.demo = Boolean(session.demo);
      if (session.user) {
        if (!await refresh({ newSession: true }, generation)) return;
        navigate(view, location.hash.slice(1));
      } else {
        resetSession(view);
      }
      render();
    } catch (error) {
      if (!isCurrent(generation)) return;
      if (error instanceof ApiError && error.status === 401) reportError(error, generation);
      else showRecovery(error);
    }
  }

  async function submitForm(form) {
    const submit = form.querySelector('[type="submit"]');
    const errorNode = form.querySelector('.form-error');
    const original = submit.innerHTML;
    errorNode.textContent = '';
    submit.disabled = true;
    submit.textContent = 'Сохраняем…';
    let saved = false;
    let authenticating = false;
    let generation = sessionGeneration;
    try {
      const command = buildFormCommand(form.dataset.form, readFields(form), {
        id: form.dataset.id, workID: form.dataset.work, problemID: form.dataset.problem,
        draft: form.dataset.form === 'work-draft' ? {
          id: form.dataset.id, kind: form.dataset.kind, starts_at: form.dataset.startsAt, ends_at: form.dataset.endsAt,
        } : undefined,
      });
      authenticating = Boolean(command.auth);
      if (authenticating) {
        generation = ++sessionGeneration;
        stopPolling();
        resetSession(view);
      }
      const send = () => api.request(command.path, { method: command.method, body: command.body });
      if (authenticating) {
        await changeSession(() => isCurrent(generation) ? send() : undefined);
      } else {
        await send();
      }
      if (!isCurrent(generation)) return;
      saved = true;
      if (form.dataset.form === 'submission') {
        editor.save(form, view.snapshot);
        await refresh(undefined, generation, { quiet: true });
        if (!isCurrent(generation)) return;
        editor.selectTab(form, 'result');
        toast(command.message);
      } else {
        closeModal();
        resetFilters(view);
        if (!await refresh({ newSession: authenticating }, generation)) return;
        if (command.page) navigate(view, command.page);
        updateHash();
        render();
        toast(command.message);
      }
    } catch (error) {
      if (!isCurrent(generation)) return;
      if (error instanceof ApiError && error.status === 401 && (saved || !authenticating)) {
        reportError(error, generation);
      } else if (saved && authenticating) {
        // The cookie exists, but the authenticated screen has not loaded yet.
        // Recovery re-reads the session; it does not repeat the login mutation.
        showRecovery(error);
      } else if (saved) {
        toast('Изменение сохранено, но список не удалось обновить. Нажмите «Обновить данные».', true);
      } else {
        errorNode.textContent = error.message;
      }
    } finally {
      if (isCurrent(generation) && submit.isConnected) {
        submit.disabled = false;
        submit.innerHTML = original;
      }
    }
  }

  function createWork(kind) {
    if (!view.snapshot.groups.length) {
      openModal(newGroupModal());
      toast('Сначала создайте группу для учеников.');
    } else if (!view.snapshot.problems.length) {
      openModal(newProblemModal());
      toast('Сначала добавьте хотя бы одну задачу.');
    } else {
      openModal(newWorkModal(view.snapshot, kind));
    }
  }

  async function changeAccess(id, status) {
    const generation = sessionGeneration;
    await api.request(`/api/works/${encodeURIComponent(id)}`, { method: 'PATCH', body: { status } });
    if (!isCurrent(generation) || !await refresh(undefined, generation)) return;
    openModal(workModal(id, view.snapshot));
    toast(status === 'open' ? 'Доступ открыт' : 'Доступ закрыт');
  }

  async function showStandings(id) {
    const generation = sessionGeneration;
    const result = await api.request(`/api/works/${encodeURIComponent(id)}/standings`);
    if (!isCurrent(generation)) return;
    openModal(standingsModal(id, Array.isArray(result.rows) ? result.rows : [], view.snapshot));
  }

  async function copyInvite(id) {
    const generation = sessionGeneration;
    const code = findByID(view.snapshot.groups, id)?.invite_code;
    if (!code) return;
    try {
      await navigator.clipboard.writeText(code);
      if (isCurrent(generation)) toast('Код группы скопирован');
    } catch {
      if (isCurrent(generation)) toast(`Код группы: ${code}`);
    }
  }

  const actions = {
    'retry-init': start,
    theme: () => {
      const theme = toggleTheme();
      for (const label of document.querySelectorAll?.('[data-theme-label]') || []) {
        label.textContent = theme === 'dark' ? 'Светлая тема' : 'Тёмная тема';
      }
    },
    'editor-tab': element => editor.selectTab(element.closest('form'), element.dataset.tab),
    'insert-template': element => editor.insertTemplate(element.closest('form'), view.snapshot),
    'confirm-template': element => editor.insertTemplate(element.closest('form'), view.snapshot, { replace: true }),
    'cancel-template': element => { element.closest('form').querySelector('[data-template-confirm]').hidden = true; },
    menu: () => root.querySelector('.shell')?.classList.toggle('menu-open'),
    'close-modal': closeModal,
    'auth-tab': element => { view.authTab = element.dataset.value; render(); },
    'demo-login': () => {
      view.authTab = 'login';
      render();
      const form = root.querySelector('form');
      form.elements.username.value = 'teacher';
      form.elements.password.value = 'campus-demo';
      form.requestSubmit();
    },
    refresh: async () => { if (await refresh()) toast('Данные обновлены'); },
    logout: async () => {
      // Invalidate reads at intent time: a response may arrive while logout is pending.
      sessionGeneration++;
      stopPolling();
      closeModal();
      resetSession(view);
      updateHash();
      render();
      await changeSession(() => api.logout());
    },
    filter: element => { view.filter = element.dataset.value; render(); },
    'create-topic': () => openModal(newTopicModal()),
    'create-group': () => openModal(newGroupModal()),
    'create-problem': () => openModal(newProblemModal()),
    'copy-problem': element => {
      const source = findByID(view.snapshot.problems, element.dataset.id);
      if (view.snapshot.user.role === 'teacher' && source) openModal(newProblemModal(source));
    },
    'create-work': () => createWork('homework'),
    'create-homework': () => createWork('homework'),
    'create-contest': () => createWork('contest'),
    work: element => openModal(workModal(element.dataset.id, view.snapshot)),
    'edit-draft': element => openModal(editWorkModal(element.dataset.id, view.snapshot)),
    'topic-detail': element => openModal(topicModal(element.dataset.id, view.snapshot)),
    problem: element => openModal(problemModal(element.dataset.id, element.dataset.work, view.snapshot)),
    'problem-preview': element => openModal(problemModal(element.dataset.id, undefined, view.snapshot)),
    'new-test-revision': element => openModal(testRevisionModal(findByID(view.snapshot.problems, element.dataset.id))),
    'work-test-revision': element => openModal(workTestRevisionModal(
      findByID(view.snapshot.problems, element.dataset.id), findByID(view.snapshot.works, element.dataset.work))),
    submission: element => openModal(submissionModal(element.dataset.id, view.snapshot)),
    standings: element => showStandings(element.dataset.id),
    'publish-work': element => changeAccess(element.dataset.id, 'open'),
    'close-work': element => changeAccess(element.dataset.id, 'closed'),
    'copy-invite': element => copyInvite(element.dataset.id),
    'add-test': element => {
      const kind = element.dataset.kind;
      const list = dialog.querySelector(`[data-test-list="${kind}"]`);
      list.insertAdjacentHTML('beforeend', testPair(kind, list.children.length));
    },
    'remove-test': element => element.closest('[data-test]').remove(),
  };

  async function handleAction(element) {
    const action = actions[element.dataset.action];
    if (!action || element.disabled) return;
    element.disabled = true;
    // Auth/logout actions advance the generation synchronously before their first await.
    let generation = sessionGeneration;
    try {
      const pending = action(element);
      generation = sessionGeneration;
      await pending;
    } catch (error) {
      reportError(error, generation);
    } finally {
      if (isCurrent(generation) && element.isConnected) element.disabled = false;
    }
  }

  async function uploadSource(input) {
    const generation = sessionGeneration;
    const file = input.files?.[0];
    const form = input.closest('form');
    if (!file || !form) return;
    if (file.size > sourceLimitBytes) {
      toast('Файл превышает 128 КБ.', true);
      input.value = '';
      return;
    }
    try {
      const source = await file.text();
      if (!isCurrent(generation) || !form.isConnected) return;
      const extension = file.name.split('.').pop().toLowerCase();
      const languages = { py: 'python', go: 'go', cpp: 'cpp', cc: 'cpp', c: 'c', java: 'java',
        js: 'javascript', rs: 'rust', kt: 'kotlin', cs: 'csharp', pas: 'pascal' };
      const language = languages[extension];
      editor.importSource(form, view.snapshot, source, language);
    } catch {
      if (isCurrent(generation)) toast('Не удалось прочитать файл.', true);
    }
  }

  function attach() {
    document.addEventListener('click', event => {
      const element = event.target.closest('[data-action]');
      if (element) { event.preventDefault(); return handleAction(element); }
    });
    document.addEventListener('submit', event => {
      const form = event.target.closest('form[data-form]');
      if (form) { event.preventDefault(); return submitForm(form); }
    });
    document.addEventListener('input', event => {
      if (event.target.id === 'global-search') { view.search = event.target.value; renderContent(); }
      if (event.target.matches?.('textarea.code')) editor.save(event.target.closest('form'), view.snapshot);
    });
    document.addEventListener('change', event => {
      if (event.target.id === 'group-filter') { view.groupFilter = event.target.value; render(); }
      if (event.target.id === 'source-file') uploadSource(event.target);
      if (event.target.name === 'language' && event.target.closest('form')?.dataset.form === 'submission') {
        editor.changeLanguage(event.target.closest('form'), view.snapshot);
      }
    });
    document.addEventListener('keydown', event => {
      const field = event.target;
      if (field.matches?.('[data-action="editor-tab"]') && ['ArrowLeft', 'ArrowRight'].includes(event.key)) {
        event.preventDefault();
        const form = field.closest('form');
        const tab = field.dataset.tab === 'code' ? 'result' : 'code';
        editor.selectTab(form, tab);
        form.querySelector(`[data-action="editor-tab"][data-tab="${tab}"]`)?.focus();
      }
      if (event.key === 'Tab' && field.matches('textarea.code')) {
        event.preventDefault();
        field.setRangeText('    ', field.selectionStart, field.selectionEnd, 'end');
        editor.save(field.closest('form'), view.snapshot);
      }
    });
    dialog.addEventListener('click', event => {
      if (event.target !== dialog) return;
      const box = dialog.getBoundingClientRect();
      if (event.clientX < box.left || event.clientX > box.right || event.clientY < box.top || event.clientY > box.bottom) closeModal();
    });
    window.addEventListener('pagehide', () => {
      const form = dialog.querySelector?.('form[data-form="submission"]');
      if (form && view.snapshot) editor.save(form, view.snapshot);
      stopPolling();
    });
    window.addEventListener('pageshow', async event => {
      if (!event.persisted || !view.snapshot) return;
      const generation = sessionGeneration;
      stopPolling();
      try {
        await refresh(undefined, generation, { quiet: true });
      } catch (error) {
        reportError(error, generation);
      } finally {
        if (isCurrent(generation)) schedulePolling();
      }
    });
    window.addEventListener('hashchange', () => {
      if (view.snapshot && navigate(view, location.hash.slice(1))) render();
    });
  }

  return { attach, start };
}
