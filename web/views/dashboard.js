import { selectors } from '../lib/state.js';
import { pageHeading, button, empty, workTable, submissionsTable } from '../lib/components.js';

export function renderDashboard(snapshot) {
  const { teacher } = selectors(snapshot);
  const works = [...snapshot.works].sort((a, b) => b.created_at.localeCompare(a.created_at));
  const submissions = [...snapshot.submissions]
    .filter(item => !teacher() || item.verdict === 'pending')
    .sort((a, b) => teacher() ? a.created_at.localeCompare(b.created_at) : b.created_at.localeCompare(a.created_at));
  return `${pageHeading('Обзор', teacher()
    ? 'Работы групп и решения, ожидающие проверки.' : 'Ваши задания, контесты и последние результаты.',
    teacher() ? button('create-work', 'Создать работу') : '')}
    <section class="page-section">
      <div class="section-heading"><h2>Работы</h2></div>
      ${works.length ? workTable(works, snapshot) : empty('Работ пока нет', teacher()
        ? 'Создайте домашнее задание или контест и откройте доступ группе.'
        : 'Здесь появятся работы, которые откроет преподаватель.')}
    </section>
    <section class="page-section">
      <div class="section-heading"><h2>${teacher() ? 'Ожидают проверки' : 'Последние посылки'}</h2>
        <a class="text-button" href="#submissions">Все посылки</a>
      </div>
      ${submissions.length ? submissionsTable(submissions.slice(0, 10), snapshot)
        : empty(teacher() ? 'Нет решений на проверке' : 'Вы ещё не отправляли решения',
          teacher() ? 'Новые посылки учеников появятся здесь.' : 'Откройте работу, выберите задачу и отправьте исходный код.')}
    </section>`;
}
