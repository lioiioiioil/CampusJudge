import test from 'node:test';
import assert from 'node:assert/strict';
import { createDraftStore, draftKey } from '../lib/drafts.js';
import { createEditor } from '../lib/editor.js';
import { preferredTheme, initializeTheme, toggleTheme } from '../lib/theme.js';

function storage() {
  const values = new Map();
  return { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) };
}

function editorFixture(userID = 'alice') {
  const confirmation = { hidden: true };
  const support = { textContent: '' };
  const status = { textContent: '' };
  const templateButton = { disabled: false };
  const form = {
    dataset: { form: 'submission', work: 'w', problem: 'p' },
    elements: { language: { value: 'cpp', options: [{ value: 'cpp' }, { value: 'python' }] }, source: { value: '' } },
    querySelector: selector => ({ '[data-template-confirm]': confirmation, '[data-language-support]': support,
      '[data-draft-status]': status, '[data-action="insert-template"]': templateButton }[selector]),
  };
  const snapshot = { user: { id: userID }, judging_mode: 'automatic', languages: [
    { id: 'cpp', automatic: true, template: '#include <iostream>\nint main() {}\n' },
    { id: 'python', automatic: false, template: 'def main():\n    pass\n' },
  ] };
  return { form, snapshot, confirmation, support, status };
}

test('theme follows the system until an explicit preference is stored', () => {
  const disk = storage(), root = { dataset: {} };
  assert.equal(preferredTheme(disk, true), 'dark');
  initializeTheme({ root, storage: disk, darkSystem: true });
  assert.equal(root.dataset.theme, 'dark');
  assert.equal(toggleTheme({ root, storage: disk }), 'light');
  assert.equal(preferredTheme(disk, true), 'light');
});

test('restricted storage does not prevent themes or draft language switching', () => {
  const blocked = { getItem() { throw new Error('blocked'); }, setItem() { throw new Error('quota'); } };
  const root = { dataset: {} };
  assert.doesNotThrow(() => initializeTheme({ root, storage: blocked, darkSystem: true }));
  assert.equal(toggleTheme({ root, storage: blocked }), 'light');
  const editor = createEditor({ drafts: createDraftStore(blocked) });
  const { form, snapshot, status } = editorFixture();
  editor.mount(form, snapshot);
  form.elements.source.value = 'my cpp';
  form.elements.language.value = 'python';
  editor.changeLanguage(form, snapshot);
  form.elements.language.value = 'cpp';
  editor.changeLanguage(form, snapshot);
  assert.equal(form.elements.source.value, 'my cpp');
  editor.save(form, snapshot);
  assert.match(status.textContent, /только до закрытия/);
});

test('changing languages restores exact code separately for each language and account', () => {
  const drafts = createDraftStore(storage());
  const editor = createEditor({ drafts });
  const { form, snapshot, support } = editorFixture();
  editor.mount(form, snapshot);
  assert.equal(form.elements.source.value, snapshot.languages[0].template);
  assert.match(support.textContent, /Автоматическая/);
  form.elements.source.value = '\n// my cpp\n';
  form.elements.language.value = 'python';
  editor.changeLanguage(form, snapshot);
  assert.equal(form.elements.source.value, snapshot.languages[1].template);
  assert.match(support.textContent, /преподавателем/);
  form.elements.source.value = '# my python';
  form.elements.language.value = 'cpp';
  editor.changeLanguage(form, snapshot);
  assert.equal(form.elements.source.value, '\n// my cpp\n');
  form.elements.language.value = 'python';
  editor.changeLanguage(form, snapshot);
  assert.equal(form.elements.source.value, '# my python');
  const bob = editorFixture('bob');
  editor.mount(bob.form, bob.snapshot);
  assert.equal(bob.form.elements.source.value, bob.snapshot.languages[0].template);
  assert.notEqual(draftKey({ userID: 'a:b', workID: 'c', problemID: 'p', language: 'cpp' }),
    draftKey({ userID: 'a', workID: 'b:c', problemID: 'p', language: 'cpp' }));
});

test('template replacement requires explicit confirmation for nonempty source', () => {
  const editor = createEditor({ drafts: createDraftStore(storage()) });
  const { form, snapshot, confirmation } = editorFixture();
  editor.mount(form, snapshot);
  form.elements.source.value = 'my important solution';
  assert.equal(editor.insertTemplate(form, snapshot), false);
  assert.equal(form.elements.source.value, 'my important solution');
  assert.equal(confirmation.hidden, false);
  assert.equal(editor.insertTemplate(form, snapshot, { replace: true }), true);
  assert.equal(form.elements.source.value, snapshot.languages[0].template);
  assert.equal(confirmation.hidden, true);
});


test('reopening a problem restores the chosen language and its persisted draft', () => {
  const disk = storage();
  const first = editorFixture();
  const editor = createEditor({ drafts: createDraftStore(disk) });
  editor.mount(first.form, first.snapshot);
  first.form.elements.language.value = 'python';
  editor.changeLanguage(first.form, first.snapshot);
  first.form.elements.source.value = 'print("saved across reload")';
  editor.save(first.form, first.snapshot);
  const reopened = editorFixture();
  createEditor({ drafts: createDraftStore(disk) }).mount(reopened.form, reopened.snapshot);
  assert.equal(reopened.form.elements.language.value, 'python');
  assert.equal(reopened.form.elements.source.value, 'print("saved across reload")');
});


test('switching editor tabs transfers keyboard tab stop to the selected tab', () => {
  function tab(name, index) {
    return { dataset: { tab: name }, tabIndex: index, classList: { toggle() {} },
      attributes: {}, setAttribute(name, value) { this.attributes[name] = value; } };
  }
  const code = tab('code', 0), result = tab('result', -1);
  const panels = [{ dataset: { editorPanel: 'code' }, hidden: false }, { dataset: { editorPanel: 'result' }, hidden: true }];
  const form = { querySelectorAll: selector => selector === '[data-action="editor-tab"]' ? [code, result] : panels };
  createEditor().selectTab(form, 'result');
  assert.equal(code.tabIndex, -1);
  assert.equal(result.tabIndex, 0);
  assert.equal(result.attributes['aria-selected'], 'true');
  assert.equal(panels[0].hidden, true);
  assert.equal(panels[1].hidden, false);
});
