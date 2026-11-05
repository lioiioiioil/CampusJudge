import { createDraftStore } from './drafts.js';

/** Owns editor values and draft transitions; callers own network/session effects. */
export function createEditor({ drafts = createDraftStore() } = {}) {
  const context = (form, snapshot, language = form.dataset.currentLanguage || form.elements.language.value) => ({
    userID: snapshot.user.id,
    workID: form.dataset.work,
    problemID: form.dataset.problem,
    language,
  });
  const languageFor = (form, snapshot) => snapshot.languages.find(item => item.id === form.elements.language.value);

  function updateSupport(form, snapshot) {
    const label = form.querySelector('[data-language-support]');
    if (!label) return;
    const automatic = snapshot.judging_mode === 'automatic' && languageFor(form, snapshot)?.automatic;
    label.textContent = automatic ? 'Автоматическая проверка по тестам' : 'Проверка преподавателем';
    const templateButton = form.querySelector('[data-action="insert-template"]');
    if (templateButton) templateButton.disabled = !languageFor(form, snapshot)?.template;
  }

  function save(form, snapshot) {
    if (!snapshot || !form?.elements?.source || !form.elements.language) return true;
    const persisted = drafts.set(context(form, snapshot), form.elements.source.value);
    const label = form.querySelector('[data-draft-status]');
    if (label) label.textContent = persisted ? 'Черновик сохранён в этом браузере' : 'Черновик сохранён только до закрытия страницы';
    return persisted;
  }

  function load(form, snapshot) {
    form.dataset.currentLanguage = form.elements.language.value;
    const draft = drafts.get(context(form, snapshot));
    form.elements.source.value = draft ?? languageFor(form, snapshot)?.template ?? '';
    const confirmation = form.querySelector('[data-template-confirm]');
    if (confirmation) confirmation.hidden = true;
    updateSupport(form, snapshot);
  }

  function mount(form, snapshot) {
    if (form?.dataset.form !== 'submission') return;
    const preferred = drafts.selectedLanguage(context(form, snapshot));
    if (snapshot.languages.some(language => language.id === preferred)) form.elements.language.value = preferred;
    load(form, snapshot);
  }

  function changeLanguage(form, snapshot) {
    // The select already contains the new value; currentLanguage still identifies the old code.
    save(form, snapshot);
    load(form, snapshot);
    drafts.selectLanguage(context(form, snapshot), form.elements.language.value);
  }

  function insertTemplate(form, snapshot, { replace = false } = {}) {
    const template = languageFor(form, snapshot)?.template;
    if (!template) return false;
    const current = form.elements.source.value;
    if (current.trim() && current !== template && !replace) {
      const confirmation = form.querySelector('[data-template-confirm]');
      if (confirmation) confirmation.hidden = false;
      return false;
    }
    form.elements.source.value = template;
    const confirmation = form.querySelector('[data-template-confirm]');
    if (confirmation) confirmation.hidden = true;
    save(form, snapshot);
    return true;
  }

  function importSource(form, snapshot, source, language) {
    save(form, snapshot);
    if (language && [...form.elements.language.options].some(option => option.value === language)) {
      form.elements.language.value = language;
    }
    form.dataset.currentLanguage = form.elements.language.value;
    form.elements.source.value = source;
    updateSupport(form, snapshot);
    save(form, snapshot);
    drafts.selectLanguage(context(form, snapshot), form.elements.language.value);
  }

  function selectTab(form, tab) {
    for (const button of form.querySelectorAll('[data-action="editor-tab"]')) {
      const selected = button.dataset.tab === tab;
      button.classList.toggle('active', selected);
      button.setAttribute('aria-selected', String(selected));
      button.tabIndex = selected ? 0 : -1;
    }
    for (const panel of form.querySelectorAll('[data-editor-panel]')) panel.hidden = panel.dataset.editorPanel !== tab;
  }

  return { mount, save, changeLanguage, insertTemplate, importSource, selectTab };
}
