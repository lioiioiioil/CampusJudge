import { esc, icon, date, plural } from '../lib/format.js';
import { selectors, workPhase } from '../lib/state.js';
import { pageHeading, button, empty, workCard, submissionsTable } from '../lib/components.js';

function stat(label, value, note, symbol) {
  return `<div class="stat-card">
    <div class="stat-label">${esc(label)}<span class="stat-icon">${icon(symbol, 16)}</span></div>
    <div class="stat-value">${esc(value)}</div><div class="stat-bottom">${esc(note)}</div>
  </div>`;
}

function stats(snapshot) {
  const { teacher, mine } = selectors(snapshot);
  const students = snapshot.users.filter(user => user.role === 'student');
  const open = snapshot.works.filter(work => workPhase(work) === 'open');
  const pending = snapshot.submissions.filter(submission => submission.verdict === 'pending');
  const accepted = snapshot.submissions.filter(submission => submission.verdict === 'accepted');
  const solved = new Set(mine().filter(submission => submission.verdict === 'accepted')
    .map(submission => `${submission.work_id}:${submission.problem_id}`)).size;
  return `<div class="stats">
    ${teacher()
      ? stat('Учеников в группах', students.length, plural(snapshot.groups.length, 'учебная группа', 'учебные группы', 'учебных групп'), 'users')
      : stat('Решено задач', solved, 'В домашних заданиях и контестах', 'check')}
    ${stat('Открытых работ', open.length, 'Можно приступать к практике', 'book')}
    ${stat('Ожидают проверки', pending.length, teacher() ? 'Уделите внимание новым решениям' : 'Преподаватель проверит решения', 'clock')}
    ${stat(teacher() ? 'Решений зачтено' : 'Отправлено решений', teacher() ? accepted.length : mine().length,
      teacher() ? 'В вашем учебном пространстве' : 'Все попытки сохраняются', 'terminal')}
  </div>`;
}

function contestPreview(snapshot) {
  const { teacher } = selectors(snapshot);
  const contest = snapshot.works.filter(work => work.kind === 'contest' && ['open', 'scheduled'].includes(workPhase(work)))
    .sort((a, b) => (a.starts_at || '').localeCompare(b.starts_at || ''))[0];
  if (!contest) {
    return `<div class="side-panel next-contest">
      <div class="eyebrow">Практика с азартом</div><h3>Задачи. Время. Результат.</h3>
      <p class="contest-preview-copy">Учебный контест помогает закрепить материал и увидеть свой прогресс.</p>
      ${teacher() ? `<div class="contest-create">${button('create-contest', 'Создать контест', 'trophy', 'soft full')}</div>` : ''}
    </div>`;
  }
  return `<div class="side-panel next-contest">
    <span class="contest-decor">{ }</span>
    <div class="eyebrow">${workPhase(contest) === 'open' ? 'Контест уже идёт' : 'Следующий контест'}</div>
    <h3>${esc(contest.title)}</h3>
    <p>${plural(contest.problem_ids.length, 'задача', 'задачи', 'задач')} · ${esc(contest.description.slice(0, 80) || 'Время проверить свои силы')}</p>
    <div class="contest-time">${icon('calendar', 13)}${contest.starts_at ? esc(date(contest.starts_at, true)) : 'Открыт для участников'}</div>
    ${button('work', 'Перейти к контесту', 'arrow', 'soft full', `data-id="${esc(contest.id)}"`)}
  </div>`;
}

function topicsPreview(snapshot) {
  const symbols = ['{ }', '[ ]', '&lt;/&gt;'];
  const topics = snapshot.topics.slice(0, 3).map((topic, index) => `<div class="topic-mini">
    <span class="topic-symbol">${symbols[index % symbols.length]}</span>
    <div><div class="topic-mini-name">${esc(topic.title)}</div>
      <div class="topic-mini-caption">${plural(snapshot.works.filter(work => work.topic_id === topic.id).length, 'работа', 'работы', 'работ')}</div>
    </div>
    <button class="icon-btn" data-action="topic-detail" data-id="${esc(topic.id)}" aria-label="Открыть тему ${esc(topic.title)}">${icon('chevron', 13)}</button>
  </div>`).join('');
  return `<div class="side-panel">
    <div class="section-heading"><h2 class="flush-heading">Темы занятий</h2><a class="text-button" href="#topics">Все ${icon('arrow', 12)}</a></div>
    ${topics || '<p>Добавьте первую тему и соберите вокруг неё практику.</p>'}
  </div>`;
}

export function renderDashboard(snapshot) {
  const { teacher } = selectors(snapshot);
  const active = snapshot.works.filter(work => ['open', 'scheduled'].includes(workPhase(work)))
    .sort((a, b) => (a.ends_at || 'z').localeCompare(b.ends_at || 'z')).slice(0, 3);
  const recent = [...snapshot.submissions].sort((a, b) => b.created_at.localeCompare(a.created_at)).slice(0, 5);
  const heading = pageHeading(
    teacher() ? 'Хороший день для практики' : `Привет, ${snapshot.user.name.split(' ')[0]}!`,
    teacher() ? 'Здесь всё, что нужно для ваших занятий: задачи, прогресс и новые решения.'
      : 'Каждая решённая задача — ещё один шаг вперёд. Продолжим?',
    teacher() ? button('create-work', 'Создать работу') : '', 'Ваш учебный ритм',
  );
  return `${heading}${stats(snapshot)}
    <div class="dashboard-columns">
      <div>
        <div class="section-heading"><h2>${teacher() ? 'Сейчас в работе' : 'Ваша практика'}</h2>
          <a class="text-button" href="#homework">Все задания ${icon('arrow', 13)}</a>
        </div>
        <div class="work-list">${active.length ? active.map(work => workCard(work, snapshot)).join('')
          : `<div class="panel">${empty('Пока нет открытых работ', teacher()
            ? 'Создайте домашнее задание и откройте его группе после занятия.'
            : 'Когда преподаватель откроет домашку или контест, они появятся здесь.',
          teacher() ? button('create-work', 'Создать работу', 'plus', 'soft') : '')}</div>`}
        </div>
      </div>
      <aside class="dashboard-aside">
        ${contestPreview(snapshot)}${topicsPreview(snapshot)}
        <div class="note">${icon('info', 15)}<span>Сейчас решения проверяет преподаватель.
          Можно отправлять код на разных языках и получать обратную связь.</span>
        </div>
      </aside>
    </div>
    <section class="activity">
      <div class="section-heading"><h2>${teacher() ? 'Последние решения' : 'Ваши последние решения'}</h2>
        <a class="text-button" href="#submissions">Все посылки ${icon('arrow', 13)}</a>
      </div>
      <div class="panel">${recent.length ? submissionsTable(recent, snapshot)
        : empty('Решения появятся здесь', 'Посылки сохраняются вместе с исходным кодом и комментариями преподавателя.', '', true)}
      </div>
    </section>`;
}
