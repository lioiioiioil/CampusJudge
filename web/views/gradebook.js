import { button, empty } from '../lib/components.js';
import { esc } from '../lib/format.js';
import { judgingBadge, reviewStatuses, submissionStatus } from '../lib/judging.js';
import { latestSubmission, selectors } from '../lib/state.js';

function attemptKey(userID, problemID) {
  return JSON.stringify([userID, problemID]);
}

function attemptButton(submission, result = submission) {
  return `<button class="row-link" data-action="submission" data-id="${esc(submission.id)}"
    title="Открыть посылку">${judgingBadge(result)}</button>`;
}

function gradebookCell(attempts) {
  const latest = latestSubmission(attempts);
  if (!latest) return '<td class="muted">Нет попыток</td>';
  const accepted = latestSubmission(attempts, submission => submission.verdict === 'accepted');
  if (!accepted) return `<td>${attemptButton(latest)}</td>`;

  const review = [latest, accepted].find(attempt => reviewStatuses.includes(submissionStatus(attempt)));
  return `<td>${attemptButton(accepted, { verdict: 'accepted' })}
    ${review ? `<div class="cell-subtitle">${attemptButton(review)}</div>` : ''}
  </td>`;
}

export function gradebookModal(id, snapshot) {
  const { teacher, work, problem } = selectors(snapshot);
  const item = work(id);
  if (!teacher() || !item) return null;
  const students = snapshot.users.filter(user => user.role === 'student' && item.group_ids.includes(user.group_id));
  const attempts = new Map();
  for (const submission of snapshot.submissions) {
    if (submission.work_id !== id) continue;
    const key = attemptKey(submission.user_id, submission.problem_id);
    if (!attempts.has(key)) attempts.set(key, []);
    attempts.get(key).push(submission);
  }
  const problems = item.problem_ids.map((id, index) => ({ id, title: problem(id)?.title || `Задача ${index + 1}` }));
  const table = students.length && problems.length
    ? `<div class="table-scroll"><table class="gradebook-table">
      <caption class="sr-only">Результаты учеников по задачам</caption>
      <thead><tr><th scope="col">Ученик</th>
        ${problems.map(task => `<th scope="col">${esc(task.title)}</th>`).join('')}
      </tr></thead>
      <tbody>${students.map(student => `<tr>
        <th scope="row"><span class="cell-title">${esc(student.name)}</span>
          <div class="cell-subtitle">${esc(student.username)}</div></th>
        ${problems.map(task => gradebookCell(attempts.get(attemptKey(student.id, task.id)) || [])).join('')}
      </tr>`).join('')}</tbody>
    </table></div>`
    : empty('Табель пока пуст', 'Табель заполняется, когда у работы есть задачи и ученики.', '', true);
  const workAttr = `data-id="${esc(id)}"`;
  return {
    title: 'Табель', subtitle: item.title, wide: true,
    body: `<div class="gradebook-actions">
      ${button('work', 'К работе', 'file', '', workAttr)}
      ${button('refresh-gradebook', 'Обновить табель', 'refresh', '', workAttr)}
    </div>${table}`,
  };
}
