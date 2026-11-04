import { esc, icon, initials, date, plural } from './format.js';
import { findByID, selectors, workPhase, verdictNames } from './state.js';

export function brand() {
  return `<a href="#overview" class="brand" aria-label="CampusJudge — обзор">
    <span class="brand-mark">${icon('code', 22)}</span>
    <span class="brand-word">campus<span>judge</span></span>
  </a>`;
}

export function empty(title, description, action = '', compact = false) {
  return `<div class="empty${compact ? ' compact' : ''}">
    <div class="empty-icon">${icon('layers', 22)}</div>
    <h3>${esc(title)}</h3><p>${esc(description)}</p>${action}
  </div>`;
}

/** attrs is markup owned by the calling view; user strings must be escaped there. */
export function button(action, label, symbol = 'plus', variant = 'primary', attrs = '') {
  return `<button class="btn ${variant}" data-action="${action}" ${attrs}>
    ${icon(symbol, 15)}<span>${esc(label)}</span>
  </button>`;
}

export function pageHeading(title, description, action = '', eyebrow = '') {
  return `<div class="page-heading">
    <div>
      ${eyebrow ? `<div class="eyebrow">${esc(eyebrow)}</div>` : ''}
      <h1>${esc(title)}</h1><p class="page-description">${esc(description)}</p>
    </div>
    ${action ? `<div class="heading-actions">${action}</div>` : ''}
  </div>`;
}

export function footer() {
  return `<footer class="footer">
    <span>CampusJudge · место, где теория становится практикой</span>
    <span>${icon('server', 12)} Локальное пространство</span>
  </footer>`;
}

export function phaseBadge(work) {
  const phase = workPhase(work);
  const labels = {
    draft: ['gray', 'Черновик'], closed: ['gray', 'Закрыто'],
    scheduled: ['blue', 'Скоро'], finished: ['gray', 'Завершено'], open: ['green', 'Открыто'],
  };
  const [color, label] = labels[phase];
  return `<span class="badge ${color}">
    ${phase === 'open' ? '<i class="dot"></i>' : ''}${label}
  </span>`;
}

export function verdictBadge(verdict) {
  const color = verdict === 'accepted' ? 'green' : verdict === 'pending' ? 'amber' : 'red';
  const symbol = verdict === 'accepted' ? icon('check', 10) : verdict === 'pending' ? icon('clock', 10) : '';
  return `<span class="badge ${color}">${symbol}${esc(verdictNames[verdict] || verdict)}</span>`;
}

export function workProgress(work, snapshot) {
  const { teacher } = selectors(snapshot);
  const submissions = snapshot.submissions.filter(item => item.work_id === work.id);
  if (!teacher()) {
    const solved = new Set(submissions.filter(item => item.verdict === 'accepted')
      .map(item => item.problem_id)).size;
    return { done: solved, total: work.problem_ids.length, caption: `${solved} из ${work.problem_ids.length} задач` };
  }
  const students = snapshot.users.filter(user => user.role === 'student' && work.group_ids.includes(user.group_id));
  const complete = students.filter(user => work.problem_ids.every(problemID => submissions.some(item =>
    item.user_id === user.id && item.problem_id === problemID && item.verdict === 'accepted'))).length;
  return { done: complete, total: students.length, caption: students.length ? `${complete} из ${students.length} завершили` : 'Пока нет участников' };
}

export function workCard(work, snapshot) {
  const { teacher, topic } = selectors(snapshot);
  const progress = workProgress(work, snapshot);
  const members = snapshot.users.filter(user => user.role === 'student' && work.group_ids.includes(user.group_id));
  const groups = work.group_ids.map(id => findByID(snapshot.groups, id)?.name).filter(Boolean);
  const topicTitle = topic(work.topic_id)?.title || (work.kind === 'contest' ? 'Учебный контест' : 'Самостоятельная практика');
  return `<article class="work-card">
    <div class="work-card-top">
      <span class="work-card-type">${icon(work.kind === 'contest' ? 'trophy' : 'book', 12)}${esc(topicTitle)}</span>
      ${phaseBadge(work)}
    </div>
    <button class="work-card-title" data-action="work" data-id="${esc(work.id)}">${esc(work.title)}</button>
    <p class="work-card-desc">${esc(work.description || 'Откройте работу, чтобы посмотреть задачи и условия.')}</p>
    <div class="work-card-meta">
      <span>${icon('file', 12)}${plural(work.problem_ids.length, 'задача', 'задачи', 'задач')}</span>
      <span>${icon('calendar', 12)}${work.ends_at ? `До ${esc(date(work.ends_at, true))}` : 'Без срока'}</span>
      ${teacher() ? `<span>${icon('users', 12)}${esc(groups.join(', '))}</span>` : ''}
    </div>
    <div class="work-card-footer">
      <div class="work-progress">
        <progress class="progress-track" max="${Math.max(1, progress.total)}" value="${progress.done}" aria-label="Прогресс работы"></progress>
        <span class="progress-caption">${progress.caption}</span>
      </div>
      ${teacher() && members.length ? `<div class="avatars" aria-label="${members.length} участников">
        ${members.slice(0, 3).map(user => `<span class="mini-avatar" title="${esc(user.name)}">${esc(initials(user.name))}</span>`).join('')}
      </div>` : ''}
      <button class="text-button" data-action="work" data-id="${esc(work.id)}">Открыть ${icon('arrow', 13)}</button>
    </div>
  </article>`;
}

export function submissionsTable(submissions, snapshot) {
  const { teacher, user, work, problem, language } = selectors(snapshot);
  const rows = submissions.map(submission => `<tr>
    ${teacher() ? `<td><div class="student-cell">
      <span class="avatar">${esc(initials(user(submission.user_id).name))}</span>
      <span class="cell-title">${esc(user(submission.user_id).name)}</span>
    </div></td>` : ''}
    <td><button class="row-link" data-action="submission" data-id="${esc(submission.id)}">
      <div class="cell-title">${esc(problem(submission.problem_id)?.title || 'Задача')}</div>
      <div class="cell-subtitle">${esc(work(submission.work_id)?.title || 'Работа')}</div>
    </button></td>
    <td><span class="language-tag">${esc(language(submission.language))}</span></td>
    <td>${verdictBadge(submission.verdict)}</td>
    <td class="muted nowrap">${esc(date(submission.created_at, true))}</td>
    <td><button class="icon-btn" data-action="submission" data-id="${esc(submission.id)}" aria-label="Открыть решение">${icon('chevron', 14)}</button></td>
  </tr>`).join('');
  return `<div class="table-scroll"><table>
    <thead><tr>${teacher() ? '<th>Ученик</th>' : ''}<th>Задача</th><th>Язык</th><th>Результат</th><th>Отправлено</th><th></th></tr></thead>
    <tbody>${rows}</tbody>
  </table></div>`;
}

export function field(label, name, options = {}) {
  const { type = 'text', value = '', placeholder = '', required = false, hint = '', min, max, rows } = options;
  const attrs = `name="${esc(name)}" ${required ? 'required' : ''}
    ${min !== undefined ? `min="${min}"` : ''} ${max !== undefined ? `max="${max}"` : ''}
    placeholder="${esc(placeholder)}"`;
  const control = type === 'textarea'
    ? `<textarea class="input" ${attrs} ${rows ? `rows="${rows}"` : ''}>${esc(value)}</textarea>`
    : `<input class="input" type="${esc(type)}" ${attrs} value="${esc(value)}">`;
  return `<label class="form-field"><span class="form-label">${esc(label)}</span>${control}
    ${hint ? `<span class="form-hint">${esc(hint)}</span>` : ''}
  </label>`;
}

export function formFooter(label = 'Сохранить') {
  return `<p class="form-error" role="alert"></p>
    <div class="modal-footer">
      <button type="button" class="btn" data-action="close-modal">Отмена</button>
      <button type="submit" class="btn primary">${esc(label)}</button>
    </div>`;
}
