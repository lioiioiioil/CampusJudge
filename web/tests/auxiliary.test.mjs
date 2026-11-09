import test from 'node:test';
import assert from 'node:assert/strict';
import { pollAuxiliary, referenceControls, referenceRequest, renderAuxiliaryResult, trialRequest } from '../lib/auxiliary.js';

test('trial request includes only one input selector and preserves custom whitespace', () => {
  const form = {
    dataset: { work: 'work', problem: 'problem', testRevision: '2' },
    elements: {
      source: { value: 'print(5)\n' },
      language: { value: 'python' },
      trial_example: { value: '0' },
      trial_input: { value: '  2 3\n' },
    },
  };
  const snapshot = { user: { id: 'student' }, languages: [{ id: 'python', automatic: true }] };
  assert.deepEqual(trialRequest(form, snapshot), {
    actor_id: 'student', work_id: 'work', problem_id: 'problem',
    source: 'print(5)\n', language: 'python', sample_index: 0, test_revision: 2,
  });
  form.elements.trial_example.value = 'custom';
  const request = trialRequest(form, snapshot);
  assert.equal(request.input, '  2 3\n');
  assert.equal('sample_index' in request, false);
  assert.equal('test_revision' in request, false);
  snapshot.languages[0].automatic = false;
  assert.throws(() => trialRequest(form, snapshot), /недоступен/);
});

test('trial polls only its owner-scoped endpoint and stops after a dialog change', async () => {
  const calls = [];
  const updates = [];
  const api = { request: async (path, options) => {
    calls.push({ path, options });
    return calls.length === 1 ? { id: 'run 1', status: 'running' } : { id: 'run 1', status: 'done', output: '5\n' };
  } };
  const publish = result => updates.push(result.status);
  await pollAuxiliary(api, '/api/trials', { sample_index: 0 }, publish, () => true, async () => {});
  assert.deepEqual(calls, [
    { path: '/api/trials', options: { method: 'POST', body: { sample_index: 0 } } },
    { path: '/api/auxiliary-runs/run%201', options: undefined },
  ]);
  assert.deepEqual(updates, ['running', 'done']);

  calls.length = updates.length = 0;
  let live = true;
  await pollAuxiliary(api, '/api/trials', {}, publish, () => live, async () => { live = false; });
  assert.equal(calls.length, 1);
  assert.deepEqual(updates, ['running']);
});

test('trial output and diagnostics are escaped', () => {
  const html = renderAuxiliaryResult({ status: 'done', verdict: 'completed', time_ms: 1,
    output: '</pre><script>alert(1)</script>', compile_output: '<img src=x>', output_truncated: true });
  assert.match(html, /&lt;script&gt;/);
  assert.match(html, /&lt;img src=x&gt;/);
  assert.match(html, /первая часть/);
  assert.doesNotMatch(html, /<script>|<img/);
});

test('reference run chooses a concrete test revision and renders only the report', () => {
  const languages = [{ id: 'python', name: 'Python', automatic: true }, { id: 'other', name: 'Другое', automatic: false }];
  const problem = { id: 'p"1', test_revisions: [{ number: 2, tests: [{ input: 'private input', output: 'private expected' }] }] };
  const controls = referenceControls(problem, languages);
  assert.match(controls, /data-reference-problem="p&quot;1"/);
  assert.match(controls, /value="2" selected/);
  assert.doesNotMatch(controls, /private|value="other"/);
  const form = { elements: { source: { value: 'print(2)' }, language: { value: 'python' }, test_revision: { value: '2' } } };
  assert.deepEqual(referenceRequest(form, { user: { id: 'teacher' }, languages }),
    { actor_id: 'teacher', source: 'print(2)', language: 'python', test_revision: 2 });
  const result = renderAuxiliaryResult({ status: 'done', verdict: 'wrong_answer', time_ms: 5, passed_tests: 1, total_tests: 3, failed_test: 2 });
  assert.match(result, /1 из 3 тестов/);
  assert.match(result, /тест № 2/);
  assert.doesNotMatch(result, /Фактический вывод/);
});
