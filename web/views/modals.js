import { judgingResult } from '../lib/judging.js';
import { esc, icon, date, localDateTime, plural, safeLesson } from '../lib/format.js';
import { selectors, findByID, workPhase, verdictNames, standingsMark, latestSubmission } from '../lib/state.js';
import { button, empty, field, formFooter, phaseBadge, verdictBadge,
  workTable, workProgress, submissionsTable } from '../lib/components.js';

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

export function testPair(kind, index = 0, test = {}) {
  const original = Object.hasOwn(test, 'input') || Object.hasOwn(test, 'output');
  const originalAttr = original ? ` data-original-test="${esc(JSON.stringify({ input: test.input ?? '', output: test.output ?? '' }))}"` : '';
  return `<div class="test-pair" data-test="${kind}"${originalAttr}>
    <div class="test-pair-head"><span>${kind === 'sample' ? 'Пример' : 'Скрытый тест'} ${index + 1}</span>
      ${index || original ? '<button type="button" class="text-button" data-action="remove-test">Убрать</button>' : ''}
    </div>
    <div class="form-grid">
      ${field('Входные данные', `${kind}_input`, { type: 'textarea', value: test.input, placeholder: '2 3' })}
      ${field('Ожидаемый вывод', `${kind}_output`, { type: 'textarea', value: test.output, placeholder: '5' })}
    </div>
  </div>`;
}

export function newProblemModal(source) {
  const pairs = (kind, tests) => tests?.length ? tests.map((test, index) => testPair(kind, index, test)).join('') : testPair(kind);
  return {
    title: source ? 'Копировать задачу' : 'Новая задача',
    subtitle: source ? 'Отредактируйте копию и сохраните её в банке задач.'
      : 'Условие и примеры видны ученикам, скрытые тесты — только преподавателю.', wide: true,
    body: `<form data-form="problem">
      ${field('Название', 'title', { required: true, value: source?.title, placeholder: 'Например, сумма двух чисел' })}
      ${field('Условие', 'statement', { type: 'textarea', required: true, value: source?.statement, rows: 4, placeholder: 'Опишите задачу простым и точным языком' })}
      <div class="form-grid">
        ${field('Формат входных данных', 'input_format', { type: 'textarea', value: source?.input_format })}
        ${field('Формат выходных данных', 'output_format', { type: 'textarea', value: source?.output_format })}
      </div>
      <div class="form-grid">
        ${field('Лимит времени, мс', 'time_limit_ms', { type: 'number', value: source?.time_limit_ms ?? 2000, min: 100, max: 30000, required: true })}
        ${field('Лимит памяти, МБ', 'memory_limit_mb', { type: 'number', value: source?.memory_limit_mb ?? 256, min: 16, max: 2048, required: true })}
      </div>
      <div class="section-label">Примеры
        <button type="button" class="text-button" data-action="add-test" data-kind="sample">${icon('plus', 13)} Добавить</button>
      </div>
      <div data-test-list="sample">${pairs('sample', source?.samples)}</div>
      <div class="section-label">Скрытые тесты
        <button type="button" class="text-button" data-action="add-test" data-kind="hidden">${icon('plus', 13)} Добавить</button>
      </div>
      <p class="help-text">Скрытые тесты используются при проверке и не показываются ученикам.</p>
      <div data-test-list="hidden">${pairs('hidden', source?.tests)}</div>
      ${formFooter(source ? 'Сохранить копию' : 'Добавить задачу')}
    </form>`,
  };
}

function workAudience(snapshot, draft) {
  const groupIDs = new Set(draft?.group_ids ?? snapshot.groups.slice(0, 1).map(group => group.id));
  const problemIDs = draft?.problem_ids ?? [];
  const problems = [...problemIDs.map(id => findByID(snapshot.problems, id)).filter(Boolean),
    ...snapshot.problems.filter(problem => !problemIDs.includes(problem.id))];
  return `<fieldset class="fieldset"><legend>Для каких групп</legend><div class="checkbox-list">
    ${snapshot.groups.map(group => `<label class="checkbox-label">
      <input type="checkbox" name="group_ids" value="${esc(group.id)}"${groupIDs.has(group.id) ? ' checked' : ''}>${esc(group.name)}
    </label>`).join('')}
  </div></fieldset>
  <fieldset class="fieldset"><legend>Задачи в работе</legend><div class="checkbox-list">
    ${problems.map(problem => `<label class="checkbox-label">
      <input type="checkbox" name="problem_ids" value="${esc(problem.id)}"${problemIDs.includes(problem.id) ? ' checked' : ''}>${esc(problem.title)}
    </label>`).join('')}
  </div></fieldset>`;
}

function workForm(snapshot, kind, draft) {
  const formAttrs = draft ? `data-form="work-draft" data-id="${esc(draft.id)}" data-kind="${esc(kind)}"
    data-starts-at="${esc(draft.starts_at)}" data-ends-at="${esc(draft.ends_at)}"` : 'data-form="work"';
  return {
    title: draft ? 'Редактировать черновик' : kind === 'contest' ? 'Новый учебный контест' : 'Новое домашнее задание',
    subtitle: draft ? 'Измените параметры работы перед публикацией.' : 'Сохраните черновик или сразу откройте работу ученикам.', wide: true,
    body: `<form ${formAttrs}>
      <div class="form-grid">
        ${draft ? '' : `<label class="form-field"><span class="form-label">Формат</span><select class="input" name="kind">
          <option value="homework"${kind === 'homework' ? ' selected' : ''}>Домашнее задание</option>
          <option value="contest"${kind === 'contest' ? ' selected' : ''}>Учебный контест</option>
        </select></label>`}
        <label class="form-field"><span class="form-label">Тема занятия</span><select class="input" name="topic_id">
          <option value="">Без темы</option>${snapshot.topics.map(topic => `<option value="${esc(topic.id)}"${topic.id === draft?.topic_id ? ' selected' : ''}>${esc(topic.title)}</option>`).join('')}
        </select></label>
      </div>
      ${field('Название работы', 'title', { required: true, value: draft?.title, placeholder: kind === 'contest'
        ? 'Например, разминка перед олимпиадой' : 'Например, практика по циклам' })}
      ${field('Описание', 'description', { type: 'textarea', value: draft?.description, placeholder: 'Что нужно сделать и на что обратить внимание' })}
      ${workAudience(snapshot, draft)}
      <div class="form-grid">
        ${field('Начало доступа', 'starts_at', { type: 'datetime-local', step: draft ? 'any' : undefined,
          value: localDateTime(draft?.starts_at), hint: 'Пусто — доступно сразу после открытия.' })}
        ${field('Срок сдачи / окончание', 'ends_at', { type: 'datetime-local', step: draft ? 'any' : undefined,
          value: localDateTime(draft?.ends_at), hint: 'Пусто — без ограничения по времени.' })}
      </div>
      ${draft ? '' : `<label class="form-field"><span class="form-label">Доступ</span><select class="input" name="status">
        <option value="draft">Черновик — виден только преподавателю</option>
        <option value="open">Открыто — ученики могут решать</option>
      </select></label>`}
      ${formFooter(draft ? 'Сохранить черновик' : 'Создать работу')}
    </form>`,
  };
}

export function newWorkModal(snapshot, kind = 'homework') {
  return workForm(snapshot, kind);
}

function editableDraft(work, snapshot) {
  return snapshot.user.role === 'teacher' && work?.status === 'draft'
    && !snapshot.submissions.some(submission => submission.work_id === work.id);
}

export function editWorkModal(id, snapshot) {
  const draft = findByID(snapshot.works, id);
  return editableDraft(draft, snapshot) ? workForm(snapshot, draft.kind, draft) : null;
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
      ${works.length ? workTable(works, snapshot)
        : empty('Работ по теме пока нет', 'Домашние задания и контесты по этой теме появятся здесь.', '', true)}`,
  };
}

function workProblemRow(work, problemID, index, snapshot) {
  const { problem } = selectors(snapshot);
  const task = problem(problemID);
  const accepted = snapshot.submissions.some(item => item.work_id === work.id
    && item.problem_id === problemID && item.verdict === 'accepted' && item.user_id === snapshot.user.id);
  return `<tr>
    <td class="problem-letter">${String.fromCharCode(65 + index)}</td>
    <td>${task ? `<button class="row-link" data-action="problem" data-id="${esc(problemID)}"
      data-work="${esc(work.id)}">${esc(task.title)}</button>` : '<span class="muted">Условие пока скрыто</span>'}
    </td>
    <td class="nowrap">${task ? `${task.time_limit_ms} мс · ${task.memory_limit_mb} МБ` : 'До начала контеста'}</td>
    <td>${accepted ? '<span class="badge green">Зачтено</span>' : '—'}</td>
  </tr>`;
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
    <div class="table-scroll"><table class="task-table">
      <thead><tr><th>#</th><th>Задача</th><th>Ограничения</th><th>Результат</th></tr></thead>
      <tbody>${item.problem_ids.map((id, index) => workProblemRow(item, id, index, snapshot)).join('')}</tbody>
    </table></div>
    ${!teacher() ? `<p class="help-text work-progress-note">Ваш прогресс: ${esc(progress.caption)}. Результаты появляются после проверки.</p>` : ''}
    <div class="detail-actions">
      ${editableDraft(item, snapshot) ? button('edit-draft', 'Редактировать', 'file', '', `data-id="${esc(item.id)}"`) : ''}
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

function submissionForm(problem, work, snapshot) {
  const languages = snapshot.languages;
  const options = languages.map(language => `<option value="${esc(language.id)}">${esc(language.name)}${language.automatic ? '' : ' · вручную'}</option>`).join('');
  const latest = latestSubmission(snapshot.submissions, item => item.user_id === snapshot.user.id
    && item.work_id === work.id && item.problem_id === problem.id);
  return `<form data-form="submission" data-work="${esc(work.id)}" data-problem="${esc(problem.id)}">
    <div class="workspace-tabbar" role="tablist" aria-label="Решение и результат">
      <button type="button" class="workspace-tab active" data-action="editor-tab" data-tab="code" id="editor-code-tab" role="tab" aria-controls="editor-code-panel" aria-selected="true">Код</button>
      <button type="button" class="workspace-tab" data-action="editor-tab" data-tab="result" id="editor-result-tab" role="tab" aria-controls="editor-result-panel" aria-selected="false" tabindex="-1">Результат</button>
    </div>
    <div data-editor-panel="code" id="editor-code-panel" role="tabpanel" aria-labelledby="editor-code-tab" class="editor-panel">
      <div class="editor-toolbar">
        <label><span class="sr-only">Язык решения</span><select class="input" name="language">${options}
          ${!languages.some(language => language.id === 'other') ? '<option value="other">Другой язык · вручную</option>' : ''}
        </select></label>
        <button type="button" class="text-button" data-action="insert-template">Вставить шаблон</button>
      </div>
      <p class="language-support" data-language-support></p>
      <div class="template-confirm" data-template-confirm hidden>
        <p>Заменить текущий код шаблоном?</p>
        <button type="button" class="btn small" data-action="confirm-template">Заменить</button>
        <button type="button" class="text-button" data-action="cancel-template">Отмена</button>
      </div>
      <label class="form-field source-field"><span class="sr-only">Исходный код</span>
        <textarea class="input code" name="source" spellcheck="false" aria-label="Исходный код решения"
          placeholder="Введите решение или загрузите файл…"></textarea>
      </label>
      <div class="editor-bottomline">
        <label class="upload"><span>Загрузить файл</span><input type="file" id="source-file" aria-label="Загрузить исходный код из файла"></label>
        <span data-draft-status class="form-hint">До 128 КБ</span>
      </div>
    </div>
    <div data-editor-panel="result" id="editor-result-panel" role="tabpanel" aria-labelledby="editor-result-tab" class="editor-result-panel" hidden>
      <div data-editor-result data-work="${esc(work.id)}" data-problem="${esc(problem.id)}">
        ${latest ? judgingResult(latest) : '<p class="help-text">Отправьте решение, чтобы увидеть результат проверки.</p>'}
      </div>
    </div>
    ${formFooter('Отправить решение')}
  </form>`;
}

export function problemModal(id, workID, snapshot) {
  const { teacher, problem, work } = selectors(snapshot);
  const original = problem(id);
  const assignment = work(workID);
  if (!original) return null;
  const revision = assignment?.problem_revisions?.[id] || 1;
  const version = original.test_revisions?.find(item => item.number === revision);
  const task = version ? { ...original, samples: version.samples, tests: version.tests } : original;
  const canSolve = assignment && workPhase(assignment) === 'open' && !teacher();
  return {
    title: task.title, subtitle: assignment?.title || 'Предпросмотр условия', wide: true,
    body: `<div class="${canSolve ? 'problem-workspace' : 'problem-preview'}">
        <section class="problem-condition" aria-label="Условие задачи">
          <div class="workspace-tabbar"><span class="workspace-tab active">Условие</span></div>
          <div class="condition-body">${statementHTML(task)}</div>
        </section>
        ${canSolve ? `<section class="solution-editor" aria-label="Отправка решения">${submissionForm(task, assignment, snapshot)}</section>` : ''}
      </div>
      ${teacher() ? `<div class="detail-actions"><span class="muted">Версия тестов ${revision}</span>
        ${button('new-test-revision', 'Новая версия тестов', 'plus', '', `data-id="${esc(id)}"`)}
        ${assignment && original.test_revisions?.length ? button('work-test-revision', 'Версия для новых посылок', '', '', `data-id="${esc(id)}" data-work="${esc(workID)}"`) : ''}
      </div>` : ''}
      ${teacher() && task.tests?.length ? `<hr><h3 class="statement-heading">Скрытые тесты
        <span class="count-label">Видны только вам</span></h3>${samplesHTML(task.tests, true)}` : ''}`,
  };
}

export function testRevisionModal(problem) {
  if (!problem) return null;
  const base = problem.test_revisions?.at(-1) || problem;
  const list = (kind, tests) => (tests?.length ? tests : [{}])
    .map((test, index) => testPair(kind, index, test)).join('');
  return {
    title: 'Новая версия тестов', subtitle: problem.title, wide: true,
    body: `<form data-form="test-revision" data-id="${esc(problem.id)}">
      <div class="section-label">Примеры <button type="button" class="text-button" data-action="add-test" data-kind="sample">Добавить</button></div>
      <div data-test-list="sample">${list('sample', base.samples)}</div>
      <div class="section-label">Скрытые тесты <button type="button" class="text-button" data-action="add-test" data-kind="hidden">Добавить</button></div>
      <div data-test-list="hidden">${list('hidden', base.tests)}</div>
      ${formFooter('Сохранить версию')}
    </form>`,
  };
}

function revisionOptions(problem, selected) {
  return [1, ...(problem.test_revisions || []).map(item => item.number)]
    .map(number => `<option value="${esc(number)}"${number === selected ? ' selected' : ''}>Версия ${esc(number)}</option>`).join('');
}

export function workTestRevisionModal(problem, work) {
  if (!problem || !work) return null;
  return {
    title: 'Версия для новых посылок', subtitle: `${work.title} · ${problem.title}`,
    body: `<form data-form="work-test-revision" data-id="${esc(work.id)}" data-problem="${esc(problem.id)}">
      <label class="form-field"><span class="form-label">Версия тестов</span>
        <select class="input" name="revision">${revisionOptions(problem, work.problem_revisions?.[problem.id] || 1)}</select>
      </label>
      ${formFooter('Применить')}
    </form>`,
  };
}

export function submissionModal(id, snapshot) {
  const { teacher, user, work, problem, language } = selectors(snapshot);
  const submission = findByID(snapshot.submissions, id);
  if (!submission) return null;
  const author = user(submission.user_id);
  const options = Object.entries(verdictNames).filter(([value]) => !['pending', 'judge_error'].includes(value))
    .map(([value, label]) => `<option value="${value}"${submission.verdict === value ? ' selected' : ''}>${label}</option>`).join('');
  return {
    title: problem(submission.problem_id)?.title || 'Посылка', subtitle: work(submission.work_id)?.title || 'Решение ученика', wide: true,
    body: `<div class="submission-summary">
      <div><strong>${esc(author.name)}</strong>
        <span class="muted"> · ${esc(date(submission.created_at, true))} · ${esc(language(submission.language))}</span>
      </div><span data-submission-verdict="${esc(submission.id)}">${verdictBadge(submission.verdict, submission.judging)}</span>
    </div>
    <div class="${teacher() ? 'review-workspace' : 'submission-result'}">
      <section class="review-source" aria-label="Исходный код">
        <h3>Исходный код</h3><pre class="source-preview">${esc(submission.source)}</pre>
      </section>
      ${teacher() ? `<section class="review-form"><h3>Проверка</h3>
        ${judgingResult(submission)}
        ${snapshot.judging_mode === 'automatic' && submission.judging?.status === 'error' && !submission.reviewer_id
          ? button('retry-judging', 'Повторить проверку', '', '', `data-id="${esc(submission.id)}"`) : ''}
        <form data-form="review" data-id="${esc(submission.id)}">
          <label class="form-field"><span class="form-label">Результат проверки</span>
            <select class="input" name="verdict" required>
              <option value="" disabled${['pending', 'judge_error'].includes(submission.verdict) ? ' selected' : ''}>Выберите результат</option>
              ${options}
            </select>
          </label>
          ${field('Комментарий для ученика', 'feedback', { type: 'textarea', value: submission.feedback })}
          ${formFooter('Сохранить результат')}
        </form>
      </section>` : `<section class="review-feedback"><h3>Проверка</h3>${judgingResult(submission)}</section>`}
    </div>`,
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
      Результаты обновляются после проверки.</p>
      <div class="panel">${rows.length ? `<div class="table-scroll"><table>
        <thead><tr><th>#</th><th>Участник</th>${problemHeaders}<th>Решено</th><th>Штраф</th></tr></thead>
        <tbody>${tableRows}</tbody>
      </table></div>` : empty('Первый результат ещё впереди', 'Таблица появится, когда участники начнут решать задачи.', '', true)}
      </div>`,
  };
}
