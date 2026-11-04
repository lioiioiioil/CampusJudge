/**
 * Server data is replaced as one snapshot. Only this module normalizes missing
 * collections; views receive snapshots explicitly and never fetch or mutate them.
 * @typedef {{id:string, name:string, username:string, role:'teacher'|'student', group_id?:string}} User
 * @typedef {{id:string, name:string, invite_code?:string}} Group
 * @typedef {{id:string, title:string, description:string, lesson_url?:string}} Topic
 * @typedef {{input:string, output:string}} TestCase
 * @typedef {{id:string, title:string, statement:string, input_format:string, output_format:string, time_limit_ms:number, memory_limit_mb:number, samples:TestCase[], tests?:TestCase[]}} Problem
 * @typedef {{id:string, title:string, description:string, kind:'homework'|'contest', status:'draft'|'open'|'closed', topic_id:string, group_ids:string[], problem_ids:string[], starts_at?:string, ends_at?:string, created_at:string}} Work
 * @typedef {{id:string, user_id:string, work_id:string, problem_id:string, language:string, source:string, verdict:string, feedback:string, created_at:string, judged_at?:string}} Submission
 * @typedef {{user:User, users:User[], groups:Group[], topics:Topic[], problems:Problem[], works:Work[], submissions:Submission[], languages:{id:string,name:string}[], demo:boolean}} Snapshot
 */
export const pageNames = Object.freeze({
  overview: 'Обзор',
  homework: 'Домашние задания',
  contests: 'Контесты',
  topics: 'Темы и задачи',
  submissions: 'Посылки',
  students: 'Ученики',
});

export const verdictNames = Object.freeze({
  pending: 'На проверке',
  accepted: 'Зачтено',
  wrong_answer: 'Неверный ответ',
  compilation_error: 'Ошибка компиляции',
  runtime_error: 'Ошибка выполнения',
  time_limit: 'Лимит времени',
  memory_limit: 'Лимит памяти',
});

export function createViewState() {
  return {
    snapshot: null,
    page: 'overview',
    search: '',
    filter: 'all',
    groupFilter: '',
    demo: false,
    authTab: 'login',
  };
}

export function resetFilters(view) {
  view.search = '';
  view.filter = 'all';
  view.groupFilter = '';
}

export function resetSession(view) {
  view.snapshot = null;
  view.page = 'overview';
  view.authTab = 'login';
  resetFilters(view);
}

/** A login always begins at the dashboard; teacher filters cannot leak to pupils. */
export function acceptSnapshot(view, snapshot, { newSession = false } = {}) {
  const collections = ['users', 'groups', 'topics', 'problems', 'works', 'submissions', 'languages'];
  const normalized = { ...snapshot };
  for (const name of collections) normalized[name] = Array.isArray(snapshot[name]) ? snapshot[name] : [];
  if (newSession || view.snapshot?.user.id !== normalized.user.id) {
    view.page = 'overview';
    resetFilters(view);
  }
  view.snapshot = normalized;
  view.demo = Boolean(snapshot.demo);
}

export function canVisit(snapshot, page) {
  return Boolean(pageNames[page]) && (page !== 'students' || snapshot?.user.role === 'teacher');
}

export function navigate(view, page) {
  if (!canVisit(view.snapshot, page)) return false;
  view.page = page;
  resetFilters(view);
  return true;
}

export function findByID(list, id) {
  return (list || []).find(item => item.id === id);
}

export function workPhase(work, at = Date.now()) {
  if (work.status === 'draft') return 'draft';
  if (work.status === 'closed') return 'closed';
  if (work.starts_at && new Date(work.starts_at).getTime() > at) return 'scheduled';
  if (work.ends_at && new Date(work.ends_at).getTime() <= at) return 'finished';
  return 'open';
}

/** Pure selectors bound to one immutable server response, not global UI state. */
export function selectors(snapshot) {
  return {
    teacher: () => snapshot.user.role === 'teacher',
    user: id => findByID(snapshot.users, id) || { name: 'Участник', username: '' },
    work: id => findByID(snapshot.works, id),
    problem: id => findByID(snapshot.problems, id),
    topic: id => findByID(snapshot.topics, id),
    language: id => findByID(snapshot.languages, id)?.name || id,
    mine: () => snapshot.submissions.filter(item => item.user_id === snapshot.user.id),
  };
}

export function matchesSearch(value, search) {
  return String(value || '').toLocaleLowerCase('ru').includes(search.toLocaleLowerCase('ru'));
}

/** attempts is the count of wrong attempts BEFORE the first accepted submission. */
export function standingsMark(result) {
  if (result?.solved) return `+${result.attempts > 0 ? result.attempts : ''}`;
  return result?.attempts ? `−${result.attempts}` : '·';
}
