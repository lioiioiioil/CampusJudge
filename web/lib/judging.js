import { esc } from './format.js';
import { verdictNames, latestSubmission } from './state.js';

export function hasActiveJudging(snapshot) {
  return Boolean(snapshot?.submissions.some(item => ['queued', 'running'].includes(item.judging?.status)));
}

export function judgingLabel(submission) {
  if (submission.judging?.status === 'queued') return 'В очереди';
  if (submission.judging?.status === 'running') return 'Проверяется';
  return verdictNames[submission.verdict] || submission.verdict;
}

export function judgingBadge(submission) {
  const color = submission.verdict === 'accepted' ? 'green' : submission.verdict === 'pending' ? 'amber' : 'red';
  return `<span class="badge ${color}">${esc(judgingLabel(submission))}</span>`;
}

export function judgingBody(submission) {
  const judging = submission.judging;
  const color = submission.verdict === 'accepted' ? 'green'
    : submission.verdict === 'pending' ? 'amber' : 'red';
  const active = ['queued', 'running'].includes(judging?.status);
  const details = [];
  if (Number.isInteger(judging?.total_tests) && judging.total_tests > 0) {
    details.push(`Пройдено тестов: ${judging.passed_tests || 0} из ${judging.total_tests}`);
  }
  if (Number.isInteger(judging?.failed_test) && judging.failed_test > 0) details.push(`Тест: ${judging.failed_test}`);
  if (!active && Number.isFinite(judging?.time_ms)) details.push(`Время: ${judging.time_ms} мс`);
  return `<div class="judge-result-heading"><strong class="badge ${color}">${esc(judgingLabel(submission))}</strong>
      <span class="muted small-copy">${esc(submission.language)}</span>
    </div>
    ${details.length ? `<p class="judge-metrics">${details.map(esc).join(' · ')}</p>` : ''}
    ${submission.verdict === 'pending' && !judging ? '<p class="help-text">Решение ожидает проверки преподавателем.</p>' : ''}
    ${judging?.compile_output ? `<pre class="judge-diagnostics">${esc(judging.compile_output)}</pre>` : ''}
    ${judging?.error ? `<p class="judge-error">${esc(judging.error)}</p>` : ''}
    ${submission.feedback && submission.feedback !== judging?.error ? `<p class="feedback">${esc(submission.feedback)}</p>` : ''}`;
}

export function judgingResult(submission) {
  return `<div class="judge-result" data-judge-result="${esc(submission.id)}" aria-live="polite">${judgingBody(submission)}</div>`;
}

/** Patch only result nodes; editor source, selection, focus and dirty forms remain owned by the user. */
export function patchJudgingResults(dialog, snapshot) {
  for (const node of dialog.querySelectorAll?.('[data-judge-result]') || []) {
    const submission = snapshot.submissions.find(item => item.id === node.dataset.judgeResult);
    if (submission) node.innerHTML = judgingBody(submission);
  }
  for (const node of dialog.querySelectorAll?.('[data-editor-result]') || []) {
    const submission = latestSubmission(snapshot.submissions, item => item.user_id === snapshot.user.id
      && item.work_id === node.dataset.work && item.problem_id === node.dataset.problem);
    if (submission) node.innerHTML = judgingResult(submission);
  }
  for (const node of dialog.querySelectorAll?.('[data-submission-verdict]') || []) {
    const submission = snapshot.submissions.find(item => item.id === node.dataset.submissionVerdict);
    if (submission) node.innerHTML = judgingBadge(submission);
  }
}
