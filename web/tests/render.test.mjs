import test from 'node:test';
import assert from 'node:assert/strict';
import { createViewState, acceptSnapshot } from '../lib/state.js';
import { workProgress } from '../lib/components.js';
import { renderDashboard } from '../views/dashboard.js';
import { gradebookModal } from '../views/gradebook.js';
import { renderTopics, renderWorks, renderSubmissions } from '../views/pages.js';
import { renderLayout } from '../views/layout.js';
import { editWorkModal, newProblemModal, problemModal, submissionModal, standingsModal, workModal } from '../views/modals.js';

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

test('gradebook only shows work participants and keeps a solved task during a rejudge', () => {
  const view = fixture();
  const pupil = fixture('student');
  const { snapshot } = view;
  snapshot.users.push(
    { id: 'a', role: 'student', name: 'Анна <i>', username: 'anna', group_id: 'g' },
    { id: 'b', role: 'student', name: 'Борис', username: 'boris', group_id: 'g' },
    { id: 'outsider', role: 'student', name: 'Другая группа', group_id: 'other' },
  );
  snapshot.works[0].problem_ids.push('p2');
  snapshot.problems.push({ id: 'p2', title: 'Другая <задача>' });
  const attempt = snapshot.submissions[0];
  snapshot.submissions.push(
    { ...attempt, id: 'a-solved', user_id: 'a', verdict: 'accepted', judging: { status: 'queued' } },
    { ...attempt, id: 'a-later-fail', user_id: 'a', verdict: 'wrong_answer', created_at: '2026-11-05T12:00:00Z' },
    { ...attempt, id: 'b-error', user_id: 'b', verdict: 'judge_error', judging: { status: 'error' } },
    { ...attempt, id: 'a-other-work', user_id: 'a', problem_id: 'p2', work_id: 'another', verdict: 'accepted' },
    { ...attempt, id: 'outsider-attempt', user_id: 'outsider', verdict: 'accepted' },
  );
  assert.match(workModal('w', snapshot).body, /data-action="gradebook" data-id="w"/);
  assert.doesNotMatch(workModal('w', pupil.snapshot).body, /data-action="gradebook"/);
  assert.equal(gradebookModal('w', pupil.snapshot), null);
  assert.equal(gradebookModal('missing', snapshot), null);
  const html = gradebookModal('w', snapshot).body;
  assert.match(html, /scope="row"[\s\S]*Анна &lt;i&gt;/);
  assert.match(html, /scope="col">Другая &lt;задача&gt;/);
  assert.match(html, /data-action="submission" data-id="a-solved"\s+title="Открыть посылку"><span class="badge green">Зачтено/);
  assert.match(html, /data-action="submission" data-id="a-solved"\s+title="Открыть посылку"><span class="badge amber">В очереди/);
  assert.match(html, /data-action="submission" data-id="b-error"\s+title="Открыть посылку"><span class="badge red">Ошибка проверки/);
  assert.equal((html.match(/Нет попыток/g) || []).length, 2);
  assert.doesNotMatch(html, /data-id="(?:s|a-later-fail|a-other-work|outsider-attempt)"|Другая группа/);
});

test('review dashboard and filters distinguish judge jobs, failures and teacher review', () => {
  const view = fixture();
  const { snapshot } = view;
  const submission = snapshot.submissions[0];
  snapshot.submissions = [
    { ...submission, id: 'manual' },
    { ...submission, id: 'queued', judging: { status: 'queued' } },
    { ...submission, id: 'running', verdict: 'accepted', judging: { status: 'running' } },
    { ...submission, id: 'judge_error', verdict: 'accepted', judging: { status: 'error' } },
    { ...submission, id: 'accepted', verdict: 'accepted', judging: { status: 'done' } },
    { ...submission, id: 'failed', verdict: 'wrong_answer', judging: { status: 'done' } },
  ];
  for (const item of snapshot.submissions) {
    view.filter = item.id;
    const html = renderSubmissions(snapshot, view);
    const submissionLinks = [...html.matchAll(/data-action="submission" data-id="([^"]+)"/g)].map(match => match[1]);
    assert.deepEqual(submissionLinks, [item.id]);
  }
  const dashboard = renderDashboard(snapshot);
  for (const label of ['Ручная проверка', 'В очереди', 'Проверяется', 'Ошибка проверки']) {
    assert.ok(dashboard.includes(`<dt>${label}</dt><dd>1</dd>`), label);
  }
  const attentionLinks = [...dashboard.matchAll(/data-action="submission" data-id="([^"]+)"/g)].map(match => match[1]);
  assert.deepEqual(attentionLinks, ['manual', 'judge_error']);
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

test('problem copy form carries both test sets and escapes copied text', () => {
  const teacher = fixture();
  const pupil = fixture('student');
  assert.match(renderTopics(teacher.snapshot, teacher), /data-action="copy-problem" data-id="p"/);
  assert.doesNotMatch(renderTopics(pupil.snapshot, pupil), /data-action="copy-problem"/);
  const source = teacher.snapshot.problems[0];
  const original = structuredClone(source);
  const form = newProblemModal(source).body;
  assert.match(form, /data-form="problem"/);
  assert.match(form, /name="title"[^>]*value="Задача"/);
  assert.match(form, /name="time_limit_ms"[^>]*value="1000"/);
  assert.match(form, /data-test="sample" data-original-test="[^"]*2 3/);
  assert.match(form, /data-test="hidden" data-original-test="[^"]*secret/);
  assert.match(form, /&lt;script&gt;alert/);
  assert.doesNotMatch(form, /<script>/);
  assert.deepEqual(source, original);
});

test('draft work form preselects its ordered tasks and is only offered before submissions', () => {
  const { snapshot } = fixture();
  const work = snapshot.works[0];
  work.status = 'draft';
  work.title = '<Черновик>';
  work.problem_ids = ['second', 'p'];
  snapshot.problems.push({ id: 'second', title: 'Вторая задача' });
  const submitted = snapshot.submissions.pop();
  assert.match(workModal('w', snapshot).body, /data-action="edit-draft"/);
  const body = editWorkModal('w', snapshot).body;
  assert.match(body, /value="&lt;Черновик&gt;"/);
  assert.match(body, /name="group_ids" value="g" checked/);
  assert.match(body, /name="problem_ids" value="second" checked[\s\S]*name="problem_ids" value="p" checked/);
  assert.doesNotMatch(body, /name="(?:kind|status)"/);
  snapshot.submissions.push(submitted);
  assert.equal(editWorkModal('w', snapshot), null);
  assert.doesNotMatch(workModal('w', snapshot).body, /data-action="edit-draft"/);
  snapshot.submissions.length = 0;
  snapshot.user.role = 'student';
  assert.equal(editWorkModal('w', snapshot), null);
});

test('standings renders +1 for exactly one prior failure', () => {
  const view = fixture();
  const spec = standingsModal('w', [{ name: 'Участник', solved: 1, penalty: 21,
    problems: { p: { solved: true, attempts: 1, minutes: 1 } } }], view.snapshot);
  assert.match(spec.body, /class="standings-cell solved">\+1\s*<div/);
});

test('pending manual review requires an explicit verdict and preserves an existing result', () => {
  const view = fixture();
  function selectedVerdict(body) {
    const select = body.match(/<select[^>]*name="verdict"[^>]*>([\s\S]*?)<\/select>/);
    assert.ok(select, 'teacher review must include the verdict selector');
    const options = [...select[1].matchAll(/<option\b([^>]*)>/g)];
    return options.find(option => /\bselected\b/.test(option[1]))?.[1];
  }
  const pending = selectedVerdict(submissionModal('s', view.snapshot).body);
  assert.ok(pending, 'pending submission needs an explicit placeholder');
  assert.match(pending, /value=""/);
  assert.match(pending, /\bdisabled\b/);
  view.snapshot.submissions[0].verdict = 'accepted';
  assert.match(selectedVerdict(submissionModal('s', view.snapshot).body), /value="accepted"/);
});
