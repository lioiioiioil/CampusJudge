import { esc, icon, plural, safeLesson } from '../lib/format.js';
import { selectors, findByID, workPhase, matchesSearch } from '../lib/state.js';
import { pageHeading, button, empty, workTable, submissionsTable } from '../lib/components.js';

function filterTabs(options, selected) {
  return `<div class="tabs">${options.map(([value, label]) => `
    <button class="tab${selected === value ? ' active' : ''}" data-action="filter" data-value="${value}">${label}</button>
  `).join('')}</div>`;
}

export function renderWorks(kind, snapshot, view) {
  const { teacher, topic } = selectors(snapshot);
  const contest = kind === 'contest';
  const createAction = contest ? 'create-contest' : 'create-homework';
  let works = snapshot.works.filter(work => work.kind === kind
    && matchesSearch(`${work.title} ${work.description} ${topic(work.topic_id)?.title || ''}`, view.search));
  if (view.filter !== 'all') {
    works = works.filter(work => {
      const phase = workPhase(work);
      if (view.filter === 'open') return ['open', 'scheduled'].includes(phase);
      if (view.filter === 'closed') return ['closed', 'finished'].includes(phase);
      return phase === view.filter;
    });
  }
  const filters = [['all', 'Все работы'], ['open', 'Открытые'],
    ...(teacher() ? [['draft', 'Черновики']] : []), ['closed', 'Завершённые']];
  return `${pageHeading(contest ? 'Учебные контесты' : 'Домашние задания',
    contest ? 'Контесты с общим временем и таблицей результатов.' : teacher()
      ? 'Открывайте задания группе после занятия и проверяйте решения.'
      : 'Домашние задания, открытые для вашей группы.',
    teacher() ? button(createAction, contest ? 'Новый контест' : 'Новая домашка') : '')}
    <div class="filter-bar">${filterTabs(filters, view.filter)}
      <span class="muted small-copy">${plural(works.length, 'работа', 'работы', 'работ')}</span>
    </div>
    ${works.length ? workTable(works, snapshot)
      : `<div class="panel">${empty('Работы не найдены', view.search ? 'Попробуйте изменить поисковый запрос.' : teacher()
        ? 'Создайте работу, добавьте задачи и выберите группу учеников.'
        : 'Преподаватель ещё не открыл работы в этом разделе.')}</div>`}`;
}

function topicRow(topic, snapshot) {
  const lessonURL = safeLesson(topic.lesson_url);
  const count = snapshot.works.filter(work => work.topic_id === topic.id).length;
  return `<li class="topic-row">
    <div class="list-item-main">
      <button class="row-link" data-action="topic-detail" data-id="${esc(topic.id)}">${esc(topic.title)}</button>
      ${topic.description ? `<p class="list-description">${esc(topic.description)}</p>` : ''}
      <span class="muted small-copy">${plural(count, 'работа', 'работы', 'работ')}</span>
    </div>
    ${lessonURL ? `<a href="${esc(lessonURL)}" target="_blank" rel="noopener noreferrer">Открыть занятие</a>` : ''}
  </li>`;
}

function problemBank(snapshot, search) {
  const problems = snapshot.problems.filter(problem => matchesSearch(`${problem.title} ${problem.statement}`, search));
  const rows = problems.map(problem => `<tr>
    <td class="cell-title">${esc(problem.title)}</td><td>${problem.time_limit_ms} мс</td>
    <td>${problem.memory_limit_mb} МБ</td><td>${plural((problem.tests || []).length, 'скрытый тест', 'скрытых теста', 'скрытых тестов')} · ${plural((problem.samples || []).length, 'пример', 'примера', 'примеров')}</td>
    <td><button class="text-button" data-action="problem-preview" data-id="${esc(problem.id)}">Условие ${icon('arrow', 12)}</button></td>
  </tr>`).join('');
  return `<section class="activity">
    <div class="section-heading"><h2>Банк задач <span class="count-label">${snapshot.problems.length}</span></h2>
      ${button('create-problem', 'Добавить задачу', 'plus', '')}
    </div>
    <div class="panel">${rows ? `<div class="table-scroll"><table>
      <thead><tr><th>Задача</th><th>Лимит времени</th><th>Память</th><th>Тесты</th><th></th></tr></thead>
      <tbody>${rows}</tbody>
    </table></div>` : empty('Задачи не найдены', search ? 'Попробуйте другой запрос.'
      : 'Добавьте условие, примеры ввода и вывода. Одну задачу можно использовать в разных работах.', '', true)}
    </div>
  </section>`;
}

export function renderTopics(snapshot, view) {
  const { teacher } = selectors(snapshot);
  const topics = snapshot.topics.filter(topic => matchesSearch(`${topic.title} ${topic.description}`, view.search));
  return `${pageHeading('Темы и задачи', 'Темы занятий, ссылки на встречи и банк учебных задач.',
    teacher() ? button('create-topic', 'Добавить тему') : '')}
    ${topics.length ? `<ul class="plain-list">${topics.map(topic => topicRow(topic, snapshot)).join('')}</ul>`
      : `<div class="panel">${empty('Тем пока нет', teacher()
        ? 'Например, «Условия и циклы». Добавьте описание и ссылку на онлайн-занятие.'
        : 'Здесь появятся темы и ссылки на ваши занятия.')}</div>`}
    ${teacher() ? problemBank(snapshot, view.search) : ''}`;
}

export function renderSubmissions(snapshot, view) {
  const { teacher, user, problem, work } = selectors(snapshot);
  let submissions = [...snapshot.submissions]
    .sort((a, b) => b.created_at.localeCompare(a.created_at))
    .filter(item => matchesSearch(`${problem(item.problem_id)?.title} ${work(item.work_id)?.title} ${user(item.user_id).name}`, view.search));
  if (view.filter !== 'all') {
    submissions = submissions.filter(item => view.filter === 'failed'
      ? !['accepted', 'pending'].includes(item.verdict) : item.verdict === view.filter);
  }
  if (view.groupFilter) submissions = submissions.filter(item => user(item.user_id).group_id === view.groupFilter);
  const filters = [['all', 'Все посылки'], ['pending', 'На проверке'], ['accepted', 'Зачтено'], ['failed', 'Нужно доработать']];
  return `${pageHeading(teacher() ? 'Посылки учеников' : 'Мои посылки', teacher()
    ? 'Исходный код, результаты и комментарии к решениям.'
    : 'История ваших решений и комментарии преподавателя.')}
    <div class="filter-bar">${filterTabs(filters, view.filter)}
      ${teacher() ? `<select id="group-filter" class="filter-select" aria-label="Группа">
        <option value="">Все группы</option>${snapshot.groups.map(group => `
          <option value="${esc(group.id)}"${view.groupFilter === group.id ? ' selected' : ''}>${esc(group.name)}</option>
        `).join('')}
      </select>` : ''}
    </div>
    <div class="panel">${submissions.length ? submissionsTable(submissions, snapshot)
      : empty('Подходящих решений нет', 'Попробуйте другой фильтр или дождитесь первых посылок учеников.')}
    </div>`;
}

function groupRow(group, snapshot) {
  const count = snapshot.users.filter(user => user.role === 'student' && user.group_id === group.id).length;
  return `<li class="group-row">
    <div class="list-item-main"><h3>${esc(group.name)}</h3>
      <span class="muted small-copy">${plural(count, 'ученик', 'ученика', 'учеников')}</span>
    </div>
    ${group.invite_code ? `<div class="invite">
      <span class="muted small-copy">Код группы:</span> <code>${esc(group.invite_code)}</code>
      <button class="icon-btn" data-action="copy-invite" data-id="${esc(group.id)}" title="Скопировать код" aria-label="Скопировать код группы">${icon('copy', 15)}</button>
    </div>` : ''}
  </li>`;
}

function studentsTable(students, snapshot) {
  const rows = students.map(user => {
    const submissions = snapshot.submissions.filter(item => item.user_id === user.id);
    const solved = new Set(submissions.filter(item => item.verdict === 'accepted')
      .map(item => `${item.work_id}:${item.problem_id}`)).size;
    return `<tr>
      <td><div class="cell-title">${esc(user.name)}</div><div class="cell-subtitle">${esc(user.username)}</div></td>
      <td class="muted">${esc(findByID(snapshot.groups, user.group_id)?.name || '—')}</td>
      <td>${submissions.length}</td><td>${solved}</td>
      <td>${submissions.filter(item => item.verdict === 'pending').length}</td>
    </tr>`;
  }).join('');
  return `<div class="table-scroll"><table>
    <thead><tr><th>Ученик</th><th>Группа</th><th>Посылки</th><th>Решено задач</th><th>На проверке</th></tr></thead>
    <tbody>${rows}</tbody>
  </table></div>`;
}

export function renderStudents(snapshot, view) {
  const { teacher } = selectors(snapshot);
  if (!teacher()) return empty('Раздел преподавателя', 'Откройте свои домашние задания или контесты.');
  const groups = snapshot.groups.filter(group => matchesSearch(group.name, view.search));
  const students = snapshot.users.filter(user => user.role === 'student' && matchesSearch(`${user.name} ${user.username}`, view.search));
  return `${pageHeading('Группы и ученики', 'Передайте ученикам код группы для самостоятельной регистрации.',
    button('create-group', 'Создать группу'))}
    ${groups.length ? `<ul class="plain-list groups-list">${groups.map(group => groupRow(group, snapshot)).join('')}</ul>`
      : `<div class="panel">${empty('Групп пока нет', 'Группа помогает открывать работы только нужным ученикам.')}</div>`}
    <div class="section-heading"><h2>Ученики <span class="count-label">${students.length}</span></h2></div>
    <div class="panel">${students.length ? studentsTable(students, snapshot)
      : empty('В группе пока нет учеников', 'Передайте ученикам код группы. После регистрации они появятся здесь.', '', true)}
    </div>`;
}
