import test from 'node:test';
import assert from 'node:assert/strict';
import { latestSubmission } from '../lib/state.js';
import { problemModal } from '../views/modals.js';
import { hasActiveJudging, judgingBody, judgingLabel, patchJudgingResults } from '../lib/judging.js';

function submission(status, verdict = 'pending') {
  return { id: 's', user_id: 'u', work_id: 'w', problem_id: 'p', created_at: '2026-11-05T10:00:00Z', language: 'cpp', verdict,
    feedback: '', judging: { status, passed_tests: 1, total_tests: 3 } };
}

test('only queued/running jobs require polling, not manual pending or completed verdicts', () => {
  assert.equal(hasActiveJudging({ submissions: [submission('queued')] }), true);
  assert.equal(hasActiveJudging({ submissions: [submission('running')] }), true);
  assert.equal(hasActiveJudging({ submissions: [submission('done', 'accepted')] }), false);
  assert.equal(hasActiveJudging({ submissions: [{ verdict: 'pending' }] }), false);
  assert.equal(judgingLabel(submission('queued')), 'В очереди');
  assert.equal(judgingLabel(submission('running')), 'Проверяется');
});

test('result rendering escapes diagnostics and never includes hidden test payloads', () => {
  const item = submission('error', 'judge_error');
  Object.assign(item.judging, { compile_output: '<img src=x onerror=alert(1)>', error: 'Temporary failure',
    input: 'PRIVATE INPUT', output: 'PRIVATE OUTPUT', expected_output: 'PRIVATE EXPECTED' });
  const html = judgingBody(item);
  assert.match(html, /Ошибка проверки/);
  assert.match(html, /&lt;img/);
  assert.doesNotMatch(html, /<img/);
  assert.doesNotMatch(html, /PRIVATE/);
  assert.match(html, /Пройдено тестов: 1 из 3/);
});

test('quiet result patch changes feedback nodes without touching an editor or dirty review form', () => {
  const result = { dataset: { work: 'w', problem: 'p' }, innerHTML: 'queued' };
  const editor = { value: 'unfinished solution', selectionStart: 5 };
  const dirtyReview = { value: 'my unsaved teacher comment' };
  const dialog = {
    querySelectorAll: selector => selector === '[data-editor-result]' ? [result] : [],
  };
  patchJudgingResults(dialog, { user: { id: 'u' }, submissions: [submission('done', 'accepted')] });
  assert.match(result.innerHTML, /Зачтено/);
  assert.equal(editor.value, 'unfinished solution');
  assert.equal(editor.selectionStart, 5);
  assert.equal(dirtyReview.value, 'my unsaved teacher comment');
});


test('latest result uses chronological RFC3339 fractions rather than lexical order', () => {
  const previous = { ...submission('done', 'wrong_answer'), id: 'old', created_at: '2026-11-05T12:00:00.1Z' };
  const latest = { ...submission('done', 'accepted'), id: 'new', created_at: '2026-11-05T12:00:00.12Z' };
  const snapshot = {
    user: { id: 'u', role: 'student' }, submissions: [previous, latest], languages: [],
    problems: [{ id: 'p', title: 'Сумма', statement: '', time_limit_ms: 1000, memory_limit_mb: 128, samples: [] }],
    works: [{ id: 'w', title: 'Домашка', status: 'open' }],
  };
  assert.equal(latestSubmission(snapshot.submissions).id, 'new');
  const html = problemModal('p', 'w', snapshot).body;
  assert.match(html, /data-judge-result="new"/);
  assert.doesNotMatch(html, /data-judge-result="old"/);
  const result = { dataset: { work: 'w', problem: 'p' }, innerHTML: '' };
  patchJudgingResults({ querySelectorAll: selector => selector === '[data-editor-result]' ? [result] : [] }, snapshot);
  assert.match(result.innerHTML, /Зачтено/);
  assert.doesNotMatch(result.innerHTML, /Неверный ответ/);
});

test('same-millisecond results prefer the last storage arrival and keep user/task scope', () => {
  const previous = { ...submission('done', 'wrong_answer'), id: 'old', created_at: '2026-11-05T12:00:00.1230001Z' };
  const latest = { ...submission('done', 'accepted'), id: 'new', created_at: '2026-11-05T12:00:00.1239999Z' };
  const unrelated = { ...latest, id: 'other-user', user_id: 'other', created_at: '2026-11-06T12:00:00Z' };
  assert.equal(Date.parse(previous.created_at), Date.parse(latest.created_at));
  assert.equal(latestSubmission([previous, latest, unrelated], item => item.user_id === 'u').id, 'new');
  const result = { dataset: { work: 'w', problem: 'p' }, innerHTML: '' };
  patchJudgingResults({ querySelectorAll: selector => selector === '[data-editor-result]' ? [result] : [] },
    { user: { id: 'u' }, submissions: [previous, latest, unrelated] });
  assert.match(result.innerHTML, /data-judge-result="new"/);
  assert.match(result.innerHTML, /Зачтено/);
});
