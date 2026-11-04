import { esc, icon, initials, date, plural, safeLesson } from '../lib/format.js';
import { selectors, findByID, workPhase, verdictNames, standingsMark } from '../lib/state.js';
import { button, empty, field, formFooter, phaseBadge, verdictBadge,
  workCard, workProgress, submissionsTable } from '../lib/components.js';

/** @typedef {{title:string, subtitle:string, body:string, wide?:boolean}} ModalSpec */

export function newTopicModal() {
  return {
    title: 'Новая тема', subtitle: 'Объедините занятие и практику по одной теме.',
    body: `<form data-form="topic">
      ${field('Название темы', 'title', { required: true, placeholder: 'Например, условия и циклы' })}
      ${field('О чём занятие', 'description', { type: 'textarea', placeholder: 'Что разберёте и чему научатся ученики' })}
      ${field('Ссылка на занятие', 'lesson_url', { type: 'url', placeholder: 'https://telemost.yandex.ru/j/…',
        hint: 'Необязательно. Подойдёт HTTPS-ссылка на вашу видеовстречу.' })}
      ${formFooter('Добавить тему')}
    </form>`,
  };
}

export function newGroupModal() {
  return {
    title: 'Новая группа', subtitle: 'Ученики присоединяются самостоятельно по коду.',
    body: `<form data-form="group">
      ${field('Название группы', 'name', { required: true, placeholder: 'Например, Python · начинающие' })}
      ${formFooter('Создать группу')}
    </form>`,
  };
}

export function testPair(kind, index = 0) {
  return `<div class="test-pair" data-test="${kind}">
    <div class="test-pair-head"><span>${kind === 'sample' ? 'Пример' : 'Скрытый тест'} ${index + 1}</span>
      ${index ? '<button type="button" class="text-button" data-action="remove-test">Убрать</button>' : ''}
    </div>
    <div class="form-grid">
      ${field('Входные данные', `${kind}_input`, { type: 'textarea', placeholder: '2 3' })}
      ${field('Ожидаемый вывод', `${kind}_output`, { type: 'textarea', placeholder: '5' })}
    </div>
  </div>`;
}

export function newProblemModal() {
  return {
    title: 'Новая задача', subtitle: 'Условие и примеры видны ученикам, скрытые тесты — только преподавателю.', wide: true,
    body: `<form data-form="problem">
      ${field('Название', 'title', { required: true, placeholder: 'Например, сумма двух чисел' })}
      ${field('Условие', 'statement', { type: 'textarea', required: true, rows: 4, placeholder: 'Опишите задачу простым и точным языком' })}
      <div class="form-grid">
        ${field('Формат входных данных', 'input_format', { type: 'textarea' })}
        ${field('Формат выходных данных', 'output_format', { type: 'textarea' })}
      </div>
      <div class="form-grid">
        ${field('Лимит времени, мс', 'time_limit_ms', { type: 'number', value: 2000, min: 100, max: 30000, required: true })}
        ${field('Лимит памяти, МБ', 'memory_limit_mb', { type: 'number', value: 256, min: 16, max: 2048, required: true })}
      </div>
      <div class="section-label">Примеры
        <button type="button" class="text-button" data-action="add-test" data-kind="sample">${icon('plus', 13)} Добавить</button>
      </div>
      <div data-test-list="sample">${testPair('sample')}</div>
      <div class="section-label">Скрытые тесты
        <button type="button" class="text-button" data-action="add-test" data-kind="hidden">${icon('plus', 13)} Добавить</button>
      </div>
      <p class="help-text">Сохраняются для проверки. Сейчас результаты выставляет преподаватель вручную.</p>
      <div data-test-list="hidden">${testPair('hidden')}</div>
      ${formFooter('Добавить задачу')}
    </form>`,
  };
}

function workAudience(snapshot) {
  return `<fieldset class="fieldset"><legend>Для каких групп</legend><div class="checkbox-list">
    ${snapshot.groups.map((group, index) => `<label class="checkbox-label">
      <input type="checkbox" name="group_ids" value="${esc(group.id)}"${index === 0 ? ' checked' : ''}>${esc(group.name)}
    </label>`).join('')}
  </div></fieldset>
  <fieldset class="fieldset"><legend>Задачи в работе</legend><div class="checkbox-list">
    ${snapshot.problems.map(problem => `<label class="checkbox-label">
      <input type="checkbox" name="problem_ids" value="${esc(problem.id)}">${esc(problem.title)}
    </label>`).join('')}
  </div></fieldset>`;
}

export function newWorkModal(snapshot, kind = 'homework') {
  return {
    title: kind === 'contest' ? 'Новый учебный контест' : 'Новое домашнее задание',
    subtitle: 'Сохраните черновик или сразу откройте работу ученикам.', wide: true,
    body: `<form data-form="work">
      <div class="form-grid">
        <label class="form-field"><span class="form-label">Формат</span><select class="input" name="kind">
          <option value="homework"${kind === 'homework' ? ' selected' : ''}>Домашнее задание</option>
          <option value="contest"${kind === 'contest' ? ' selected' : ''}>Учебный контест</option>
        </select></label>
        <label class="form-field"><span class="form-label">Тема занятия</span><select class="input" name="topic_id">
          <option value="">Без темы</option>${snapshot.topics.map(topic => `<option value="${esc(topic.id)}">${esc(topic.title)}</option>`).join('')}
        </select></label>
      </div>
      ${field('Название работы', 'title', { required: true, placeholder: kind === 'contest'
        ? 'Например, разминка перед олимпиадой' : 'Например, практика по циклам' })}
      ${field('Описание', 'description', { type: 'textarea', placeholder: 'Что нужно сделать и на что обратить внимание' })}
      ${workAudience(snapshot)}
      <div class="form-grid">
        ${field('Начало доступа', 'starts_at', { type: 'datetime-local', hint: 'Пусто — доступно сразу после открытия.' })}
        ${field('Срок сдачи / окончание', 'ends_at', { type: 'datetime-local', hint: 'Пусто — без ограничения по времени.' })}
      </div>
      <label class="form-field"><span class="form-label">Доступ</span><select class="input" name="status">
        <option value="draft">Черновик — виден только преподавателю</option>
        <option value="open">Открыто — ученики могут решать</option>
      </select></label>
      ${formFooter('Создать работу')}
    </form>`,
  };
}

export function topicModal(id, snapshot) {
  const topic = findByID(snapshot.topics, id);
  if (!topic) return null;
  const works = snapshot.works.filter(work => work.topic_id === id);
  const lessonURL = safeLesson(topic.lesson_url);
  return {
    title: topic.title, subtitle: 'Материалы и практика по теме',
    body: `<p class="detail-description">${esc(topic.description || 'Здесь собраны работы по теме.')}</p>
      ${lessonURL ? `<a class="btn soft" href="${esc(lessonURL)}" target="_blank" rel="noopener noreferrer">
        ${icon('link', 15)} Открыть занятие
      </a><hr>` : ''}
      <div class="work-list">${works.length ? works.map(work => workCard(work, snapshot)).join('')
        : empty('Практика пока не добавлена', 'Работы по этой теме появятся здесь.', '', true)}
      </div>`,
  };
}

function workProblemRow(work, problemID, index, snapshot) {
  const { teacher, problem } = selectors(snapshot);
  const task = problem(problemID);
  const accepted = snapshot.submissions.some(item => item.work_id === work.id
    && item.problem_id === problemID && item.verdict === 'accepted' && item.user_id === snapshot.user.id);
  const canSolve = workPhase(work) === 'open' && !teacher();
  return `<div class="problem-row">
    <span class="problem-letter">${String.fromCharCode(65 + index)}</span>
    <div class="problem-row-content"><h3>${esc(task?.title || 'Условие пока скрыто')}</h3>
      <p>${task ? `${task.time_limit_ms} мс · ${task.memory_limit_mb} МБ` : 'Откроется к началу контеста'}</p>
    </div>
    ${accepted ? '<span class="badge green">Зачтено</span>' : ''}
    ${task ? `<button class="btn small ${canSolve ? 'primary' : ''}" data-action="problem"
      data-id="${esc(problemID)}" data-work="${esc(work.id)}">${teacher() ? 'Условие' : canSolve ? 'Решать' : 'Посмотреть'}
    </button>` : ''}
  </div>`;
}

export function workModal(id, snapshot) {
  const { teacher, work } = selectors(snapshot);
  const item = work(id);
  if (!item) return null;
  const phase = workPhase(item);
  const progress = workProgress(item, snapshot);
  const submissions = snapshot.submissions.filter(submission => submission.work_id === id)
    .sort((a, b) => b.created_at.localeCompare(a.created_at));
  return {
    title: item.title, subtitle: item.kind === 'contest' ? 'Учебный контест' : 'Домашнее задание', wide: true,
    body: `<div class="work-detail-meta">${phaseBadge(item)}
      <span>${plural(item.problem_ids.length, 'задача', 'задачи', 'задач')}</span>
      <span>${item.ends_at ? `До ${esc(date(item.ends_at, true))}` : 'Без срока'}</span>
    </div>
    <p class="detail-description">${esc(item.description)}</p>
    ${phase === 'scheduled' && !teacher() ? `<div class="alert blue">${icon('lock', 16)}
      <span>Условия откроются ${esc(date(item.starts_at, true))}. Возвращайтесь к началу работы.</span>
    </div>` : ''}
    ${phase === 'finished' ? `<div class="alert">${icon('clock', 16)}
      <span>Время работы завершено. Новые решения больше не принимаются.</span>
    </div>` : ''}
    <div>${item.problem_ids.map((id, index) => workProblemRow(item, id, index, snapshot)).join('')}</div>
    ${!teacher() ? `<p class="help-text work-progress-note">Ваш прогресс: ${esc(progress.caption)}. Решения проверяются преподавателем.</p>` : ''}
    <div class="detail-actions">
      ${item.kind === 'contest' ? button('standings', 'Таблица результатов', 'trophy', '', `data-id="${esc(item.id)}"`) : ''}
      ${teacher() ? item.status !== 'open'
        ? button('publish-work', 'Открыть доступ', 'check', 'primary', `data-id="${esc(item.id)}"`)
        : button('close-work', 'Закрыть доступ', 'lock', '', `data-id="${esc(item.id)}"`) : ''}
      ${teacher() && item.status === 'draft' ? '<span class="help-text">Черновик скрыт от учеников.</span>' : ''}
    </div>
    <section class="activity">
      <div class="section-heading"><h2>${teacher() ? 'Посылки по работе' : 'Ваши посылки'}</h2></div>
      <div class="panel">${submissions.length ? submissionsTable(submissions, snapshot)
        : empty('Попыток пока нет', 'Все отправленные решения появятся здесь.', '', true)}
      </div>
    </section>`,
  };
}

function samplesHTML(tests, hidden = false) {
  return tests.map(test => `<div class="samples">
    <div class="sample-block"><div class="sample-label">${hidden ? 'Вход' : 'Входные данные'}</div><pre>${esc(test.input)}</pre></div>
    <div class="sample-block"><div class="sample-label">${hidden ? 'Ожидаемый вывод' : 'Выходные данные'}</div><pre>${esc(test.output)}</pre></div>
  </div>`).join('');
}

function statementHTML(problem) {
  return `<div class="work-detail-meta"><span>${icon('clock', 12)} ${problem.time_limit_ms} мс</span>
      <span>Память: ${problem.memory_limit_mb} МБ</span>
    </div>
    <p class="problem-statement">${esc(problem.statement)}</p>
    ${problem.input_format ? `<h3 class="statement-heading">Входные данные</h3><p class="problem-statement statement-copy">${esc(problem.input_format)}</p>` : ''}
    ${problem.output_format ? `<h3 class="statement-heading">Выходные данные</h3><p class="problem-statement statement-copy">${esc(problem.output_format)}</p>` : ''}
    ${problem.samples?.length ? `<h3 class="statement-heading">Примеры</h3>${samplesHTML(problem.samples)}` : ''}`;
}

function submissionForm(problem, work, languages) {
  const options = languages.map(language => `<option value="${esc(language.id)}">${esc(language.name)}</option>`).join('');
  return `<hr><h3 class="statement-heading">Отправить решение</h3>
    <div class="alert blue">${icon('info', 15)}
      <span>Ручная проверка: исходный код получит преподаватель. Автоматический запуск пока не выполняется.</span>
    </div>
    <form data-form="submission" data-work="${esc(work.id)}" data-problem="${esc(problem.id)}">
      <label class="form-field"><span class="form-label">Язык решения</span>
        <select class="input" name="language">${options}
          ${!languages.some(language => language.id === 'other') ? '<option value="other">Другой язык</option>' : ''}
        </select>
      </label>
      <label class="upload">${icon('upload', 14)} Загрузить файл
        <input type="file" id="source-file" aria-label="Загрузить исходный код из файла">
      </label>
      <label class="form-field"><span class="form-label">Исходный код</span>
        <textarea class="input code" name="source" required spellcheck="false" aria-label="Исходный код решения"
          placeholder="Вставьте код решения или загрузите файл…"></textarea>
        <span class="form-hint">До 128 КБ. Можно отправить несколько попыток.</span>
      </label>
      ${formFooter('Отправить решение')}
    </form>`;
}

export function problemModal(id, workID, snapshot) {
  const { teacher, problem, work } = selectors(snapshot);
  const task = problem(id);
  const assignment = work(workID);
  if (!task) return null;
  const canSolve = assignment && workPhase(assignment) === 'open' && !teacher();
  return {
    title: task.title, subtitle: assignment?.title || 'Предпросмотр условия', wide: true,
    body: `${statementHTML(task)}
      ${canSolve ? submissionForm(task, assignment, snapshot.languages) : ''}
      ${teacher() && task.tests?.length ? `<hr><h3 class="statement-heading">Скрытые тесты
        <span class="count-label">Видны только вам</span></h3>${samplesHTML(task.tests, true)}` : ''}`,
  };
}

export function submissionModal(id, snapshot) {
  const { teacher, user, work, problem, language } = selectors(snapshot);
  const submission = findByID(snapshot.submissions, id);
  if (!submission) return null;
  const author = user(submission.user_id);
  const options = Object.entries(verdictNames).filter(([value]) => value !== 'pending')
    .map(([value, label]) => `<option value="${value}"${submission.verdict === value ? ' selected' : ''}>${label}</option>`).join('');
  return {
    title: problem(submission.problem_id)?.title || 'Посылка', subtitle: work(submission.work_id)?.title || 'Решение ученика', wide: true,
    body: `<div class="submission-summary">
      <div class="student-cell"><span class="avatar">${esc(initials(author.name))}</span>
        <div><div class="cell-title">${esc(author.name)}</div>
          <div class="cell-subtitle">${esc(date(submission.created_at, true))} · ${esc(language(submission.language))}</div>
        </div>
      </div>${verdictBadge(submission.verdict)}
    </div>
    <p class="help-text">${submission.verdict === 'pending' ? 'Ожидает ручной проверки преподавателем.' : 'Результат ручной проверки преподавателем.'}</p>
    <pre class="source-preview">${esc(submission.source)}</pre>
    ${submission.feedback ? `<div class="feedback">${esc(submission.feedback)}</div>` : ''}
    ${teacher() ? `<hr><form data-form="review" data-id="${esc(submission.id)}">
      <label class="form-field"><span class="form-label">Результат проверки</span><select class="input" name="verdict">${options}</select></label>
      ${field('Комментарий для ученика', 'feedback', { type: 'textarea', value: submission.feedback, placeholder: 'Что получилось и на что обратить внимание' })}
      ${formFooter('Сохранить результат')}
    </form>` : ''}`,
  };
}

export function standingsModal(workID, rows, snapshot) {
  const { work, problem } = selectors(snapshot);
  const contest = work(workID);
  if (!contest) return null;
  const problemHeaders = contest.problem_ids.map((id, index) => `<th class="standings-cell"
    title="${esc(problem(id)?.title || 'Задача')}">${String.fromCharCode(65 + index)}</th>`).join('');
  const tableRows = rows.map((row, index) => `<tr>
    <td class="rank${index === 0 ? ' first' : ''}">${index + 1}</td><td class="cell-title">${esc(row.name)}</td>
    ${contest.problem_ids.map(id => {
      const result = row.problems?.[id];
      const color = result?.solved ? ' solved' : result?.attempts ? ' failed' : '';
      return `<td class="standings-cell${color}">${standingsMark(result)}
        ${result?.solved ? `<div class="cell-subtitle">${esc(result.minutes)} мин</div>` : ''}
      </td>`;
    }).join('')}
    <td class="cell-title">${esc(row.solved)}</td><td class="muted">${esc(row.penalty)}</td>
  </tr>`).join('');
  return {
    title: 'Таблица результатов', subtitle: contest.title, wide: true,
    body: `<p class="help-text standings-note">Сначала число решённых задач, затем штрафное время.
      Результаты обновляются после проверки преподавателем.</p>
      <div class="panel">${rows.length ? `<div class="table-scroll"><table>
        <thead><tr><th>#</th><th>Участник</th>${problemHeaders}<th>Решено</th><th>Штраф</th></tr></thead>
        <tbody>${tableRows}</tbody>
      </table></div>` : empty('Первый результат ещё впереди', 'Таблица появится, когда участники начнут решать задачи.', '', true)}
      </div>`,
  };
}
