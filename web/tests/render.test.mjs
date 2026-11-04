import test from 'node:test';
import assert from 'node:assert/strict';
import { createViewState, acceptSnapshot } from '../lib/state.js';
import { workProgress } from '../lib/components.js';
import { renderDashboard } from '../views/dashboard.js';
import { renderTopics, renderWorks, renderSubmissions } from '../views/pages.js';
import { renderLayout } from '../views/layout.js';
import { problemModal, submissionModal, standingsModal } from '../views/modals.js';

function fixture(role = 'teacher') {
  const view = createViewState();
  acceptSnapshot(view, {
    user: { id: 'u', role, name: 'Участник', username: 'user' },
    users: [{ id: 'u', role, name: 'Участник', username: 'user' }],
    groups: [{ id: 'g', name: 'Группа', invite_code: 'INVITE' }],
    topics: [],
    problems: [{ id: 'p', title: 'Задача', statement: '<script>alert(1)</script>', time_limit_ms: 1000,
      memory_limit_mb: 128, samples: [{ input: '2 3', output: '5' }], tests: [{ input: 'hidden', output: 'secret' }] }],
    works: [{ id: 'w', title: 'Работа', kind: 'contest', description: '', status: 'open',
      group_ids: ['g'], problem_ids: ['p'], topic_id: '', created_at: '2026-11-05T10:00:00Z' }],
    submissions: [{ id: 's', user_id: 'u', problem_id: 'p', work_id: 'w', source: '</pre><img src=x onerror=alert(1)>',
      language: 'python', verdict: 'pending', feedback: '', created_at: '2026-11-05T10:00:00Z' }],
    languages: [{ id: 'python', name: 'Python' }],
  });
  return view;
}

test('student pages hide management actions and use a distinct navigation', () => {
  const view = fixture('student');
  for (const content of [renderDashboard(view.snapshot), renderWorks('homework', view.snapshot, view),
    renderTopics(view.snapshot, view), renderSubmissions(view.snapshot, view)]) {
    assert.doesNotMatch(content, /data-action="create-/);
  }
  const layout = renderLayout(view.snapshot, view, '');
  assert.doesNotMatch(layout, /href="#students"/);
  assert.match(layout, /Ученик/);
});

test('conditions and source are escaped, and hidden tests are teacher-only', () => {
  const teacher = fixture();
  const pupil = fixture('student');
  const condition = problemModal('p', 'w', pupil.snapshot).body;
  assert.match(condition, /&lt;script&gt;alert/);
  assert.doesNotMatch(condition, /<script>/);
  assert.doesNotMatch(condition, /secret/);
  assert.match(problemModal('p', 'w', teacher.snapshot).body, /secret/);
  const source = submissionModal('s', pupil.snapshot).body;
  assert.match(source, /&lt;\/pre&gt;&lt;img/);
  assert.doesNotMatch(source, /<img/);
  assert.doesNotMatch(source, /data-form="review"/);
});

test('empty class progress and Russian sample count are accurate', () => {
  const view = fixture();
  assert.equal(workProgress(view.snapshot.works[0], view.snapshot).caption, 'Пока нет участников');
  assert.match(renderTopics(view.snapshot, view), /1 пример/);
  assert.doesNotMatch(renderTopics(view.snapshot, view), /1 примеров/);
});

test('standings renders +1 for exactly one prior failure', () => {
  const view = fixture();
  const spec = standingsModal('w', [{ name: 'Участник', solved: 1, penalty: 21,
    problems: { p: { solved: true, attempts: 1, minutes: 1 } } }], view.snapshot);
  assert.match(spec.body, /class="standings-cell solved">\+1\s*<div/);
});
