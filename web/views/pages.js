import { esc, icon, initials, plural, safeLesson } from '../lib/format.js';
import { selectors, findByID, workPhase, matchesSearch } from '../lib/state.js';
import { pageHeading, button, empty, workCard, submissionsTable } from '../lib/components.js';

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
    contest ? 'Немного азарта, интересные задачи и понятный результат.' : teacher()
      ? 'Пройдите тему на занятии, затем откройте практику своей группе.'
      : 'Закрепляйте пройденное в своём темпе. Все попытки сохраняются.',
    teacher() ? button(createAction, contest ? 'Новый контест' : 'Новая домашка') : '')}
    <div class="filter-bar">${filterTabs(filters, view.filter)}
      <span class="muted small-copy">${plural(works.length, 'работа', 'работы', 'работ')}</span>
    </div>
    ${works.length ? `<div class="work-grid">${works.map(work => workCard(work, snapshot)).join('')}</div>`
      : `<div class="panel">${empty('Здесь пока тихо', view.search ? 'Попробуйте изменить поисковый запрос.' : teacher()
        ? 'Создайте работу, добавьте задачи и выберите группу учеников.'
        : 'Преподаватель ещё не открыл работы в этом разделе.',
      teacher() ? button(createAction, 'Создать первую работу', 'plus', 'soft') : '')}</div>`}`;
}

function topicCard(topic, index, snapshot) {
  const lessonURL = safeLesson(topic.lesson_url);
  const symbols = ['{ }', '[ ]', '&lt;/&gt;'];
  return `<article class="topic-card">
    <span class="topic-symbol">${symbols[index % symbols.length]}</span>
    <h3>${esc(topic.title)}</h3><p>${esc(topic.description || 'Материалы и практика по теме.')}</p>
    <div class="work-card-meta topic-card-meta"><span>${icon('book', 12)}
      ${plural(snapshot.works.filter(work => work.topic_id === topic.id).length, 'работа', 'работы', 'работ')}
    </span></div>
    ${lessonURL ? `<a class="btn small soft" href="${esc(lessonURL)}" target="_blank" rel="noopener noreferrer">
      ${icon('link', 13)} Открыть занятие
    </a>` : ''}
    <button class="text-button topic-open" data-action="topic-detail" data-id="${esc(topic.id)}">Открыть тему ${icon('arrow', 13)}</button>
  </article>`;
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
  return `${pageHeading('От темы — к практике', 'Соберите материалы занятия, задачи и домашние задания в одном месте.',
    teacher() ? button('create-topic', 'Добавить тему') : '')}
    ${topics.length ? `<div class="topic-grid">${topics.map((topic, index) => topicCard(topic, index, snapshot)).join('')}</div>`
      : `<div class="panel">${empty('Первая тема — начало курса', teacher()
        ? 'Например, «Условия и циклы». Добавьте описание и ссылку на онлайн-занятие.'
        : 'Здесь появятся темы и ссылки на ваши занятия.',
      teacher() ? button('create-topic', 'Добавить тему', 'plus', 'soft') : '')}</div>`}
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
  return `${pageHeading(teacher() ? 'Каждая попытка имеет значение' : 'Ваш путь к решению', teacher()
    ? 'Посмотрите исходный код, оставьте комментарий и помогите двигаться дальше.'
    : 'Здесь вся история решений и обратная связь от преподавателя.')}
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

function groupCard(group, snapshot) {
  const students = snapshot.users.filter(user => user.role === 'student' && user.group_id === group.id);
  return `<article class="group-card">
    <div class="student-cell group-heading"><span class="avatar">${icon('users', 19)}</span>
      <div><h3>${esc(group.name)}</h3><p class="muted tiny-copy">${plural(students.length, 'ученик', 'ученика', 'учеников')}</p></div>
    </div>
    ${group.invite_code ? `<div class="invite">
      <div><div class="invite-label">Код для регистрации учеников</div><code>${esc(group.invite_code)}</code></div>
      <button class="icon-btn" data-action="copy-invite" data-id="${esc(group.id)}" title="Скопировать код" aria-label="Скопировать код группы">${icon('copy', 15)}</button>
    </div>` : ''}
  </article>`;
}

function studentsTable(students, snapshot) {
  const rows = students.map(user => {
    const submissions = snapshot.submissions.filter(item => item.user_id === user.id);
    const solved = new Set(submissions.filter(item => item.verdict === 'accepted')
      .map(item => `${item.work_id}:${item.problem_id}`)).size;
    return `<tr>
      <td><div class="student-cell"><span class="avatar">${esc(initials(user.name))}</span>
        <div><div class="cell-title">${esc(user.name)}</div><div class="cell-subtitle">${esc(user.username)}</div></div>
      </div></td>
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
  return `${pageHeading('Учиться интереснее вместе', 'Пригласите учеников по коду группы. Они смогут зарегистрироваться самостоятельно.',
    button('create-group', 'Создать группу'))}
    ${groups.length ? `<div class="group-grid">${groups.map(group => groupCard(group, snapshot)).join('')}</div>`
      : `<div class="panel">${empty('Соберите первую группу', 'Группа помогает открывать работы только нужным ученикам.',
        button('create-group', 'Создать группу', 'plus', 'soft'))}</div>`}
    <div class="section-heading"><h2>Ученики <span class="count-label">${students.length}</span></h2></div>
    <div class="panel">${students.length ? studentsTable(students, snapshot)
      : empty('Ждём первых учеников', 'Передайте ученикам код группы. После регистрации они появятся здесь.', '', true)}
    </div>`;
}
