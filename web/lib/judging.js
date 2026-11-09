import { date, esc } from './format.js';
import { verdictNames, latestSubmission } from './state.js';

export function hasActiveJudging(snapshot) {
  return Boolean(snapshot?.submissions.some(item => ['queued', 'running'].includes(item.judging?.status)));
}

export function judgingLabel(submission) {
  if (submission.judging?.status === 'queued') return 'В очереди';
  if (submission.judging?.status === 'running') return 'Проверяется';
  if (submission.judging?.status === 'error') return verdictNames.judge_error;
  return verdictNames[submission.verdict] || submission.verdict;
}

function judgeColor(submission) {
  if (submission.judging?.status === 'error') return 'red';
  if (submission.verdict === 'pending' || ['queued', 'running'].includes(submission.judging?.status)) return 'amber';
  return submission.verdict === 'accepted' ? 'green' : 'red';
}

export function judgingBadge(submission) {
  return `<span class="badge ${judgeColor(submission)}">${esc(judgingLabel(submission))}</span>`;
}

export function canRetryJudging(submission) {
  return !submission.reviewer_id && submission.judging?.status === 'error';
}

export function canRejudge(submission) {
  return !submission.reviewer_id && ['done', 'error'].includes(submission.judging?.status)
    && !['pending', 'judge_error'].includes(submission.verdict);
}

export function judgingBody(submission) {
  const judging = submission.judging;
  const color = judgeColor(submission);
  const active = ['queued', 'running'].includes(judging?.status);
  const details = [];
  const revision = submission.test_revision || 1;
  const target = judging?.target_revision || revision;
  const effective = !['pending', 'judge_error'].includes(submission.verdict);
  details.push(`Версия тестов ${target}`);
  if (Number.isInteger(judging?.total_tests) && judging.total_tests > 0) {
    details.push(`Пройдено тестов: ${judging.passed_tests || 0} из ${judging.total_tests}`);
  }
  if (Number.isInteger(judging?.failed_test) && judging.failed_test > 0) details.push(`Тест: ${judging.failed_test}`);
  if (!active && Number.isFinite(judging?.time_ms)) details.push(`Время: ${judging.time_ms} мс`);
  return `<div class="judge-result-heading"><strong class="badge ${color}">${esc(judgingLabel(submission))}</strong>
      <span class="muted small-copy">${esc(submission.language)}</span>
    </div>
    ${(active || judging?.status === 'error') && effective
      ? `<p class="help-text">Действующий результат ${esc(verdictNames[submission.verdict] || submission.verdict)} · версия ${esc(revision)}</p>` : ''}
    ${details.length ? `<p class="judge-metrics">${details.map(esc).join(' · ')}</p>` : ''}
    ${submission.verdict === 'pending' && !judging ? '<p class="help-text">Решение ожидает проверки преподавателем.</p>' : ''}
    ${judging?.compile_output ? `<pre class="judge-diagnostics">${esc(judging.compile_output)}</pre>` : ''}
    ${judging?.error ? `<p class="judge-error">${esc(judging.error)}</p>` : ''}
    ${submission.feedback && submission.feedback !== judging?.error ? `<p class="feedback">${esc(submission.feedback)}</p>` : ''}
    ${submission.past_judgments?.length ? `<details data-judgment-history><summary>История вердиктов</summary><ul>
      ${[...submission.past_judgments].reverse().map(item => `<li>${esc(verdictNames[item.verdict] || item.verdict)} · версия ${esc(item.test_revision)} · ${esc(date(item.replaced_at, true))}</li>`).join('')}
    </ul></details>` : ''}`;
}

export function judgingResult(submission) {
  return `<div class="judge-result" data-judge-result="${esc(submission.id)}" aria-live="polite">${judgingBody(submission)}</div>`;
}

function patchResult(node, content) {
  if (node.innerHTML === content) return;
  const wasOpen = node.querySelector?.('[data-judgment-history]')?.open;
  node.innerHTML = content;
  const history = node.querySelector?.('[data-judgment-history]');
  if (history && wasOpen) history.open = true;
}

/** Patch only result nodes; editor source, selection, focus and dirty forms remain owned by the user. */
export function patchJudgingResults(dialog, snapshot) {
  for (const node of dialog.querySelectorAll?.('[data-judge-result]') || []) {
    const submission = snapshot.submissions.find(item => item.id === node.dataset.judgeResult);
    if (submission) patchResult(node, judgingBody(submission));
  }
  for (const node of dialog.querySelectorAll?.('[data-editor-result]') || []) {
    const submission = latestSubmission(snapshot.submissions, item => item.user_id === snapshot.user.id
      && item.work_id === node.dataset.work && item.problem_id === node.dataset.problem);
    if (submission) patchResult(node, judgingResult(submission));
  }
  for (const node of dialog.querySelectorAll?.('[data-submission-verdict]') || []) {
    const submission = snapshot.submissions.find(item => item.id === node.dataset.submissionVerdict);
    if (submission) node.innerHTML = judgingBadge(submission);
  }
  for (const node of dialog.querySelectorAll?.('[data-judge-retry]') || []) {
    const submission = snapshot.submissions.find(item => item.id === node.dataset.judgeRetry);
    node.hidden = !submission || !canRetryJudging(submission);
  }
  for (const node of dialog.querySelectorAll?.('[data-judge-rejudge]') || []) {
    const submission = snapshot.submissions.find(item => item.id === node.dataset.judgeRejudge);
    node.hidden = !submission || !canRejudge(submission);
  }
}
