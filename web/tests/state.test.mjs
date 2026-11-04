import test from 'node:test';
import assert from 'node:assert/strict';
import { createViewState, acceptSnapshot, resetSession, navigate, canVisit,
  workPhase, standingsMark } from '../lib/state.js';

function snapshot(id, role) {
  return { user: { id, role }, demo: true };
}

test('changing teacher to student resets role-specific filters and route', () => {
  const view = createViewState();
  acceptSnapshot(view, snapshot('teacher', 'teacher'));
  Object.assign(view, { page: 'students', groupFilter: 'g', filter: 'pending', search: 'Иван' });
  acceptSnapshot(view, snapshot('student', 'student'));
  assert.deepEqual([view.page, view.search, view.filter, view.groupFilter], ['overview', '', 'all', '']);
  assert.equal(canVisit(view.snapshot, 'students'), false);
  assert.equal(navigate(view, 'students'), false);
});

test('refresh preserves filters, while logout and new login clear them', () => {
  const view = createViewState();
  acceptSnapshot(view, snapshot('teacher', 'teacher'));
  view.groupFilter = 'g';
  acceptSnapshot(view, snapshot('teacher', 'teacher'));
  assert.equal(view.groupFilter, 'g');
  resetSession(view);
  assert.equal(view.snapshot, null);
  assert.equal(view.groupFilter, '');
  view.search = 'stale';
  acceptSnapshot(view, snapshot('teacher', 'teacher'), { newSession: true });
  assert.equal(view.search, '');
});

test('navigation rejects unknown pages and clears cross-page filters', () => {
  const view = createViewState();
  acceptSnapshot(view, snapshot('teacher', 'teacher'));
  view.filter = 'pending';
  assert.equal(navigate(view, 'homework'), true);
  assert.equal(view.filter, 'all');
  assert.equal(navigate(view, 'not-a-page'), false);
  assert.equal(view.page, 'homework');
});

test('accepted standings display zero, one and two prior wrong attempts', () => {
  assert.equal(standingsMark({ solved: true, attempts: 0 }), '+');
  assert.equal(standingsMark({ solved: true, attempts: 1 }), '+1');
  assert.equal(standingsMark({ solved: true, attempts: 2 }), '+2');
  assert.equal(standingsMark({ solved: false, attempts: 2 }), '−2');
  assert.equal(standingsMark(undefined), '·');
});

test('work phases use explicit time boundaries', () => {
  const at = Date.parse('2026-11-05T10:00:00Z');
  assert.equal(workPhase({ status: 'draft' }, at), 'draft');
  assert.equal(workPhase({ status: 'open', starts_at: '2026-11-05T11:00:00Z' }, at), 'scheduled');
  assert.equal(workPhase({ status: 'open', ends_at: '2026-11-05T09:00:00Z' }, at), 'finished');
  assert.equal(workPhase({ status: 'open', starts_at: '2026-11-05T10:00:00Z' }, at), 'open');
  assert.equal(workPhase({ status: 'open', ends_at: '2026-11-05T10:00:00Z' }, at), 'finished');
  assert.equal(workPhase({ status: 'closed' }, at), 'closed');
});
