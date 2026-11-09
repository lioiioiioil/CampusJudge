import { esc, icon, date } from './format.js';
import { currentTheme } from './theme.js';
import { judgingBadge } from './judging.js';
import { findByID, selectors, workPhase } from './state.js';

export function brand() {
  return '<a href="#overview" class="brand" aria-label="CampusJudge — обзор">CampusJudge</a>';
}

export function themeToggle() {
  return `<button class="text-button theme-toggle" data-action="theme" aria-label="Переключить светлую и тёмную тему">
    <span aria-hidden="true">◐</span> <span data-theme-label>${currentTheme() === 'dark' ? 'Светлая тема' : 'Тёмная тема'}</span>
  </button>`;
}

export function empty(title, description, action = '', compact = false) {
  return `<div class="empty${compact ? ' compact' : ''}">
    <h3>${esc(title)}</h3><p>${esc(description)}</p>${action}
  </div>`;
}

/** attrs is markup owned by the calling view; user strings must be escaped there. */
export function button(action, label, symbol = 'plus', variant = 'primary', attrs = '') {
  return `<button class="btn ${variant}" data-action="${action}" ${attrs}>
    ${icon(symbol, 15)}<span>${esc(label)}</span>
  </button>`;
}

export function pageHeading(title, description, action = '') {
  return `<div class="page-heading">
    <div>
      <h1>${esc(title)}</h1><p class="page-description">${esc(description)}</p>
    </div>
    ${action ? `<div class="heading-actions">${action}</div>` : ''}
  </div>`;
}

export function footer() {
  return '<footer class="footer">CampusJudge · Учебные задания и контесты</footer>';
}

export function phaseBadge(work) {
  const phase = workPhase(work);
  const labels = {
    draft: ['gray', 'Черновик'], closed: ['gray', 'Закрыто'],
    scheduled: ['blue', 'Скоро'], finished: ['gray', 'Завершено'], open: ['green', 'Открыто'],
  };
  const [color, label] = labels[phase];
  return `<span class="badge ${color}">${label}</span>`;
}

export function verdictBadge(verdict, judging) {
  return judgingBadge({ verdict, judging });
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

function workRow(work, snapshot) {
  const { teacher, topic } = selectors(snapshot);
  const progress = workProgress(work, snapshot);
  const groups = work.group_ids.map(id => findByID(snapshot.groups, id)?.name).filter(Boolean);
  const topicTitle = topic(work.topic_id)?.title;
  return `<tr>
    <td class="work-name">
      <button class="row-link" data-action="work" data-id="${esc(work.id)}">${esc(work.title)}</button>
      <div class="cell-subtitle">${work.kind === 'contest' ? 'Контест' : 'Домашнее задание'}${topicTitle ? ` · ${esc(topicTitle)}` : ''}</div>
    </td>
    ${teacher() ? `<td>${esc(groups.join(', ') || '—')}</td>` : ''}
    <td class="number">${work.problem_ids.length}</td>
    <td class="nowrap">${work.ends_at ? esc(date(work.ends_at, true)) : 'Без срока'}</td>
    <td>${phaseBadge(work)}</td>
    <td class="muted">${esc(progress.caption)}</td>
  </tr>`;
}

export function workTable(works, snapshot) {
  const { teacher } = selectors(snapshot);
  return `<div class="table-scroll"><table class="work-table">
    <thead><tr><th>Название / тема</th>${teacher() ? '<th>Группа</th>' : ''}
      <th class="number">Задачи</th><th>Срок</th><th>Статус</th><th>Прогресс</th>
    </tr></thead>
    <tbody>${works.map(work => workRow(work, snapshot)).join('')}</tbody>
  </table></div>`;
}

export function submissionsTable(submissions, snapshot) {
  const { teacher, user, work, problem, language } = selectors(snapshot);
  const rows = submissions.map(submission => `<tr>
    ${teacher() ? `<td>${esc(user(submission.user_id).name)}</td>` : ''}
    <td><button class="row-link" data-action="submission" data-id="${esc(submission.id)}">${esc(problem(submission.problem_id)?.title || 'Задача')}</button>
      <div class="cell-subtitle">${esc(work(submission.work_id)?.title || 'Работа')}</div>
    </td>
    <td class="nowrap">${esc(language(submission.language))}</td>
    <td><span data-submission-verdict="${esc(submission.id)}">${verdictBadge(submission.verdict, submission.judging)}</span></td>
    <td class="muted nowrap">${esc(date(submission.created_at, true))}</td>
  </tr>`).join('');
  return `<div class="table-scroll"><table class="submission-table">
    <thead><tr>${teacher() ? '<th>Ученик</th>' : ''}<th>Задача / работа</th><th>Язык</th><th>Результат</th><th>Отправлено</th></tr></thead>
    <tbody>${rows}</tbody>
  </table></div>`;
}

export function field(label, name, options = {}) {
  const { type = 'text', value = '', placeholder = '', required = false, hint = '', min, max, rows, step } = options;
  const attrs = `name="${esc(name)}" ${required ? 'required' : ''}
    ${min !== undefined ? `min="${min}"` : ''} ${max !== undefined ? `max="${max}"` : ''}
    ${step !== undefined ? `step="${esc(step)}"` : ''} placeholder="${esc(placeholder)}"`;
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
