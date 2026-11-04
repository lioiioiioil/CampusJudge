import { esc, icon, initials } from '../lib/format.js';
import { pageNames, selectors, canVisit } from '../lib/state.js';
import { brand, footer } from '../lib/components.js';

const navIcons = {
  overview: 'grid', homework: 'book', contests: 'trophy', topics: 'layers',
  submissions: 'terminal', students: 'users',
};

export function renderLayout(snapshot, view, content) {
  const { teacher } = selectors(snapshot);
  const pending = snapshot.submissions.filter(item => item.verdict === 'pending').length;
  const navigation = Object.entries(pageNames)
    .filter(([page]) => canVisit(snapshot, page))
    .map(([page, title]) => `<a href="#${page}" class="nav-item${view.page === page ? ' active' : ''}"
      ${view.page === page ? 'aria-current="page"' : ''}>
      ${icon(navIcons[page], 18)}<span>${esc(title)}</span>
      ${page === 'submissions' && pending ? `<span class="nav-count">${pending}</span>` : ''}
    </a>`).join('');
  const today = new Date().toLocaleDateString('ru-RU', {
    weekday: 'long', day: 'numeric', month: 'long',
  });
  return `<div class="shell">
    <button class="mobile-overlay" data-action="menu" aria-label="Закрыть меню"></button>
    <aside class="sidebar">
      ${brand()}<div class="workspace-label">Учебное пространство</div>
      <nav class="nav" aria-label="Основная навигация">${navigation}</nav>
      <div class="sidebar-bottom">
        <div class="offline-card">
          <div class="offline-title"><i class="dot"></i> Всё в вашем пространстве</div>
          Материалы и решения сохраняются на сервере преподавателя.
        </div>
        <div class="profile">
          <span class="avatar">${esc(initials(snapshot.user.name))}</span>
          <div><div class="profile-name">${esc(snapshot.user.name)}</div>
            <div class="profile-role">${teacher() ? 'Преподаватель' : 'Ученик'}</div>
          </div>
          <button class="icon-btn" data-action="logout" title="Выйти" aria-label="Выйти">${icon('logout', 16)}</button>
        </div>
      </div>
    </aside>
    <main class="main">
      ${view.demo ? '<div class="demo-strip">Тестовое пространство · данные сохраняются локально</div>' : ''}
      <header class="topbar">
        <button class="icon-btn mobile-menu" data-action="menu" aria-label="Открыть меню">${icon('menu', 18)}</button>
        <div class="breadcrumb">Пространство ${icon('chevron', 10)} <b>${esc(pageNames[view.page])}</b></div>
        <div class="topbar-right">
          <span class="topbar-date">${esc(today)}</span>
          <label class="search-wrap">${icon('search', 15)}
            <input id="global-search" class="search-input" type="search" placeholder="Найти в пространстве…"
              value="${esc(view.search)}" aria-label="Поиск в текущем разделе">
          </label>
          <button class="icon-btn" data-action="refresh" title="Обновить данные" aria-label="Обновить данные">${icon('refresh', 17)}</button>
        </div>
      </header>
      <div class="content" id="page-content">${content}${footer()}</div>
    </main>
  </div>`;
}
