import test from 'node:test';
import assert from 'node:assert/strict';
import { buildFormCommand } from '../lib/forms.js';
import { problemModal, testRevisionModal, workTestRevisionModal } from '../views/modals.js';

const problem = { id: 'p', title: 'Задача', statement: 'Условие', time_limit_ms: 1000,
  memory_limit_mb: 64, samples: [{ input: 'old-example', output: '1' }],
  test_revisions: [{ number: 2, samples: [{ input: 'new-example', output: '2' }],
    tests: [{ input: 'secret', output: '3' }] }] };
const work = { id: 'w', title: 'Работа', status: 'open', problem_revisions: { p: 2 } };

test('work workspace displays the activated example and keeps teacher controls private', () => {
  const snapshot = { user: { id: 'student', role: 'student' }, users: [],
    works: [work], problems: [problem], submissions: [], languages: [], groups: [], topics: [] };
  const markup = problemModal('p', 'w', snapshot).body;
  assert.match(markup, /new-example/);
  assert.doesNotMatch(markup, /old-example|secret|new-test-revision/);
});

test('teacher can copy latest tests into a new revision and select a revision for future attempts', () => {
  assert.match(testRevisionModal(problem).body, /new-example/);
  assert.match(workTestRevisionModal(problem, work).body, /value="2" selected/);
  const command = buildFormCommand('work-test-revision', { revision: '2' }, { id: 'w', problemID: 'p' });
  assert.equal(command.path, '/api/works/w/problem-revisions');
  assert.deepEqual(command.body, { problem_id: 'p', revision: 2 });
});
