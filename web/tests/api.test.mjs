import test from 'node:test';
import assert from 'node:assert/strict';
import { createAPI, ApiError } from '../lib/api.js';

test('mutations include same-origin cookie and CSRF header without retry', async () => {
  const calls = [];
  const api = createAPI(async (path, options) => {
    calls.push({ path, options });
    return { ok: true, status: 201, json: async () => ({ id: 'saved' }) };
  });
  await api.request('/api/topics', { method: 'POST', body: { title: 'Тема' } });
  assert.equal(calls.length, 1);
  assert.equal(calls[0].options.credentials, 'same-origin');
  assert.equal(calls[0].options.headers['X-Campus-Request'], '1');
  assert.equal(JSON.parse(calls[0].options.body).title, 'Тема');
});

test('authentication failure retains status for the controller', async () => {
  const api = createAPI(async () => ({ ok: false, status: 401, json: async () => ({ error: 'Войдите снова' }) }));
  await assert.rejects(api.snapshot(), error => error instanceof ApiError && error.status === 401);
});

test('lost mutation response is reported once and does not resubmit', async () => {
  let attempts = 0;
  const api = createAPI(async () => { attempts++; throw new Error('offline'); });
  await assert.rejects(api.request('/api/submissions', { method: 'POST', body: {} }), /историю посылок/);
  assert.equal(attempts, 1);
});


test('submission reads request a compact snapshot, specific statuses and individual source', async () => {
  const paths = [];
  const api = createAPI(async path => {
    paths.push(path);
    return { ok: true, status: 200, json: async () => ({}) };
  });
  await api.snapshot();
  await api.submissionStatuses(['a/b', 'c']);
  await api.submission('a/b');
  assert.deepEqual(paths, ['/api/state?summary=1', '/api/submission-statuses?ids=a%2Fb%2Cc', '/api/submissions/a%2Fb']);
});
