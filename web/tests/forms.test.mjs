import test from 'node:test';
import assert from 'node:assert/strict';
import { buildFormCommand, sourceLimitBytes } from '../lib/forms.js';
import { localDateTime } from '../lib/format.js';

const homework = {
  title: ' Практика ', description: 'После занятия', kind: 'homework', topic_id: '',
  group_ids: ['group-1'], problem_ids: ['problem-1'], status: 'draft',
};

test('work command preserves selected audience, task order and visibility', () => {
  const result = buildFormCommand('work', {
    ...homework, group_ids: ['group-1', 'group-1'], problem_ids: ['b', 'a'],
    starts_at: '2026-11-05T10:00:00Z', ends_at: '2026-11-05T11:00:00Z',
  });
  assert.equal(result.path, '/api/works');
  assert.equal(result.body.title, 'Практика');
  assert.deepEqual(result.body.group_ids, ['group-1']);
  assert.deepEqual(result.body.problem_ids, ['b', 'a']);
  assert.equal(result.body.status, 'draft');
  assert.equal(result.body.starts_at, '2026-11-05T10:00:00.000Z');
});

test('work rejects missing audience/tasks and reversed or invalid dates', () => {
  assert.throws(() => buildFormCommand('work', { ...homework, group_ids: [] }), /группу/);
  assert.throws(() => buildFormCommand('work', { ...homework, problem_ids: [] }), /задачу/);
  assert.throws(() => buildFormCommand('work', { ...homework, starts_at: 'broken' }), /дату/);
  assert.throws(() => buildFormCommand('work', {
    ...homework, starts_at: '2026-11-05T11:00:00Z', ends_at: '2026-11-05T10:00:00Z',
  }), /позже/);
});

test('draft edit sends editable fields only and preserves an unchanged deadline exactly', () => {
  const draft = { id: 'draft/a', kind: 'homework', ends_at: '2026-11-05T11:00:30.123456789Z' };
  const result = buildFormCommand('work-draft', {
    ...homework, description: ' Описание ', group_ids: [], problem_ids: ['b', 'a'],
    ends_at: localDateTime(draft.ends_at),
  }, { draft });
  assert.equal(result.path, '/api/works/draft%2Fa/draft');
  assert.equal(result.method, 'PATCH');
  assert.deepEqual(result.body, {
    title: 'Практика', description: ' Описание ', topic_id: '', group_ids: [], problem_ids: ['b', 'a'],
    ends_at: draft.ends_at,
  });
  assert.equal(buildFormCommand('work-draft', { ...homework, ends_at: '' }, { draft }).body.ends_at, undefined);
});

test('submission preserves source whitespace and binds to the opened work', () => {
  const source = '\n  print(5)\n';
  const result = buildFormCommand('submission', { language: 'python', source }, { workID: 'w', problemID: 'p' });
  assert.deepEqual(result.body, { language: 'python', source, work_id: 'w', problem_id: 'p' });
});

test('source size uses bytes, and empty code is not a submission', () => {
  const context = { workID: 'w', problemID: 'p' };
  assert.throws(() => buildFormCommand('submission', { language: 'go', source: '  ' }, context), /исходный код/);
  assert.throws(() => buildFormCommand('submission', { language: 'go', source: 'я'.repeat(sourceLimitBytes / 2 + 1) }, context), /128 КБ/);
  assert.doesNotThrow(() => buildFormCommand('submission', { language: 'go', source: 'a'.repeat(sourceLimitBytes) }, context));
});

test('lesson link only accepts HTTPS, including an empty optional field', () => {
  assert.equal(buildFormCommand('topic', { title: 'Тема', lesson_url: '' }).body.lesson_url, '');
  assert.equal(buildFormCommand('topic', { title: 'Тема', lesson_url: 'https://example.com/lesson' }).body.lesson_url, 'https://example.com/lesson');
  for (const url of ['javascript:alert(1)', 'http://example.com', 'not a URL']) {
    assert.throws(() => buildFormCommand('topic', { title: 'Тема', lesson_url: url }));
  }
});

test('problem command preserves test whitespace and separates visible/hidden tests', () => {
  const result = buildFormCommand('problem', {
    title: 'Сумма', statement: 'Сложите числа.', time_limit_ms: '1000', memory_limit_mb: '128',
    samples: [{ input: ' 2 3\n', output: '5\n' }], tests: [{ input: '', output: '' }, { input: '8 9', output: '17' }],
  });
  assert.equal(result.body.time_limit_ms, 1000);
  assert.deepEqual(result.body.samples, [{ input: ' 2 3\n', output: '5\n' }]);
  assert.deepEqual(result.body.tests, [{ input: '8 9', output: '17' }]);
});

test('review uses an encoded identifier and rejects pending as a final verdict', () => {
  assert.equal(buildFormCommand('review', { verdict: 'accepted', feedback: ' Хорошо ' }, { id: 'a/b' }).path, '/api/submissions/a%2Fb');
  assert.throws(() => buildFormCommand('review', { verdict: 'pending' }, { id: 'a' }), /результат/);
});
