import { selectors } from '../lib/state.js';
import { pageHeading, button, empty, workTable, submissionsTable } from '../lib/components.js';
import { reviewStatuses, submissionStatus, submissionStatusNames } from '../lib/judging.js';

export function renderDashboard(snapshot) {
  const { teacher } = selectors(snapshot);
  const works = [...snapshot.works].sort((a, b) => b.created_at.localeCompare(a.created_at));
  const reviewCounts = Object.fromEntries(reviewStatuses.map(status => [status, 0]));
  const submissions = [];
  for (const item of snapshot.submissions) {
    const status = submissionStatus(item);
    if (teacher() && reviewStatuses.includes(status)) reviewCounts[status]++;
    if (!teacher() || status === 'manual' || status === 'judge_error') submissions.push(item);
  }
  submissions.sort((a, b) => teacher() ? a.created_at.localeCompare(b.created_at) : b.created_at.localeCompare(a.created_at));
  return `${pageHeading('Обзор', teacher()
    ? 'Работы групп и состояние проверки решений.' : 'Ваши задания, контесты и последние результаты.',
    teacher() ? button('create-work', 'Создать работу') : '')}
    <section class="page-section">
      <div class="section-heading"><h2>Работы</h2></div>
      ${works.length ? workTable(works, snapshot) : empty('Работ пока нет', teacher()
        ? 'Создайте домашнее задание или контест и откройте доступ группе.'
        : 'Здесь появятся работы, которые откроет преподаватель.')}
    </section>
    <section class="page-section">
      <div class="section-heading"><h2>${teacher() ? 'Требуют внимания' : 'Последние посылки'}</h2>
        <a class="text-button" href="#submissions">Все посылки</a>
      </div>
      ${teacher() ? `<dl class="review-counts">${reviewStatuses.map(status => `
        <div><dt>${submissionStatusNames[status]}</dt><dd>${reviewCounts[status]}</dd></div>
      `).join('')}</dl>` : ''}
      ${submissions.length ? submissionsTable(submissions.slice(0, 10), snapshot)
        : empty(teacher() ? 'Нет решений, требующих внимания' : 'Вы ещё не отправляли решения',
          teacher() ? 'Здесь появятся решения для ручной проверки и сбои автопроверки.'
            : 'Откройте работу, выберите задачу и отправьте исходный код.')}
    </section>`;
}
