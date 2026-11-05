import { esc, icon } from '../lib/format.js';
import { pageNames, selectors, canVisit } from '../lib/state.js';
import { brand, footer } from '../lib/components.js';

export function renderLayout(snapshot, view, content) {
  const { teacher } = selectors(snapshot);
  const role = teacher() ? 'Преподаватель' : 'Ученик';
  const pending = snapshot.submissions.filter(item => item.verdict === 'pending').length;
  const navigation = Object.entries(pageNames)
    .filter(([page]) => canVisit(snapshot, page))
    .map(([page, title]) => `<a href="#${page}" class="nav-item${view.page === page ? ' active' : ''}"
      ${view.page === page ? 'aria-current="page"' : ''}>${esc(title)}
      ${page === 'submissions' && pending ? `<span class="nav-count">${pending}</span>` : ''}
    </a>`).join('');
  return `<div class="shell">
    <header class="app-header">
      <div class="header-main">
        ${brand()}
        <div class="account">
          ${snapshot.user.name !== role ? `<span class="account-role">${role}</span>` : ''}
          <span class="account-name">${esc(snapshot.user.name)}</span>
          <button class="text-button" data-action="logout">Выйти</button>
          <button class="icon-btn mobile-menu" data-action="menu" aria-label="Открыть меню">${icon('menu', 19)}</button>
        </div>
      </div>
      <nav class="nav" aria-label="Основная навигация">${navigation}</nav>
    </header>
    <main class="main">
      <div class="page-tools">
        ${view.demo ? '<span class="demo-label">Тестовое пространство</span>' : '<span></span>'}
        <div class="page-tools-actions">
          ${view.page !== 'overview' ? `<label class="search-wrap">
            <span class="sr-only">Поиск в текущем разделе</span>
            <input id="global-search" class="search-input" type="search" placeholder="Поиск в разделе"
              value="${esc(view.search)}">
          </label>` : ''}
          <button class="text-button" data-action="refresh">${icon('refresh', 14)} Обновить</button>
        </div>
      </div>
      <div id="page-content">${content}${footer()}</div>
    </main>
  </div>`;
}
