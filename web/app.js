import { createAPI } from './lib/api.js';
import { createViewState } from './lib/state.js';
import { createController } from './lib/actions.js';
import { footer } from './lib/components.js';
import { renderAuth } from './views/auth.js';
import { renderLayout } from './views/layout.js';
import { renderDashboard } from './views/dashboard.js';
import { renderWorks, renderTopics, renderSubmissions, renderStudents } from './views/pages.js';

const root = document.getElementById('app');
const view = createViewState();
const pages = {
  overview: snapshot => renderDashboard(snapshot),
  homework: (snapshot, state) => renderWorks('homework', snapshot, state),
  contests: (snapshot, state) => renderWorks('contest', snapshot, state),
  topics: renderTopics,
  submissions: renderSubmissions,
  students: renderStudents,
};

function pageContent() {
  return (pages[view.page] || pages.overview)(view.snapshot, view);
}

function render() {
  root.innerHTML = view.snapshot
    ? renderLayout(view.snapshot, view, pageContent())
    : renderAuth(view);
}

function renderContent() {
  const content = document.getElementById('page-content');
  if (content && view.snapshot) content.innerHTML = pageContent() + footer();
}

const controller = createController({
  view,
  api: createAPI(),
  root,
  dialog: document.getElementById('modal'),
  toastNode: document.getElementById('toast'),
  render,
  renderContent,
});
controller.attach();
controller.start();
