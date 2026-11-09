import { localDateTime } from './format.js';

export const sourceLimitBytes = 128 * 1024;

/**
 * A command is pure data. Reading controls, sending HTTP and updating the view
 * belong to the controller; these builders can be checked without a browser.
 * @typedef {{path:string, method:string, body:Object, message:string, page?:string, auth?:boolean}} FormCommand
 */
function text(fields, name) {
  return String(fields[name] ?? '').trim();
}

function required(fields, name, label) {
  const value = text(fields, name);
  if (!value) throw new Error(`Заполните поле «${label}».`);
  return value;
}

function positiveInteger(fields, name, label) {
  const value = Number(fields[name]);
  if (!Number.isSafeInteger(value) || value <= 0) throw new Error(`Укажите положительное целое число: ${label}.`);
  return value;
}

function selected(fields, name, label, required = true) {
  const values = Array.isArray(fields[name]) ? fields[name] : [];
  const result = [...new Set(values.map(String).filter(Boolean))];
  if (required && !result.length) throw new Error(`Выберите хотя бы одну ${label}.`);
  return result;
}

function instant(value, label) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) throw new Error(`Проверьте дату: ${label}.`);
  return parsed.toISOString();
}

function command(path, body, message, page, method = 'POST') {
  return { path, body, message, page, method };
}

function login(fields) {
  return {
    ...command('/api/login', {
      username: required(fields, 'username', 'Логин'),
      password: String(fields.password || ''),
    }, 'Добро пожаловать!'),
    auth: true,
  };
}

function register(fields) {
  const password = String(fields.password || '');
  if (password.length < 8) throw new Error('Пароль должен содержать не менее 8 символов.');
  return {
    ...command('/api/register', {
      username: required(fields, 'username', 'Логин'),
      name: required(fields, 'name', 'Ваше имя'),
      invite_code: required(fields, 'invite_code', 'Код группы'),
      password,
    }, 'Добро пожаловать!'),
    auth: true,
  };
}

function workDetails(fields, draft) {
  const body = {
    title: required(fields, 'title', 'Название работы'),
    description: draft ? String(fields.description ?? '') : text(fields, 'description'),
    topic_id: text(fields, 'topic_id'),
    group_ids: selected(fields, 'group_ids', 'группу', !draft),
    problem_ids: selected(fields, 'problem_ids', 'задачу', !draft),
  };
  for (const [name, label] of [['starts_at', 'начало доступа'], ['ends_at', 'окончание']]) {
    if (!fields[name]) continue;
    const original = draft?.[name];
    const value = instant(fields[name], label);
    // Preserve the exact stored instant when the displayed local time is unchanged.
    body[name] = original && localDateTime(original) === localDateTime(fields[name]) ? original : value;
  }
  if (!draft && body.starts_at && body.ends_at && body.ends_at <= body.starts_at) {
    throw new Error('Окончание работы должно быть позже начала.');
  }
  return body;
}

function work(fields) {
  const kind = text(fields, 'kind');
  if (!['homework', 'contest'].includes(kind)) throw new Error('Выберите формат работы.');
  const status = text(fields, 'status');
  if (!['draft', 'open'].includes(status)) throw new Error('Выберите доступ к работе.');
  return command('/api/works', { ...workDetails(fields), kind, status },
    status === 'open' ? 'Работа открыта ученикам' : 'Черновик сохранён',
    kind === 'contest' ? 'contests' : 'homework');
}

function editDraft(fields, { draft }) {
  if (!draft?.id) throw new Error('Черновик не найден. Откройте работу заново.');
  return command(`/api/works/${encodeURIComponent(draft.id)}/draft`, workDetails(fields, draft),
    'Черновик обновлён', draft.kind === 'contest' ? 'contests' : 'homework', 'PATCH');
}

function testCases(fields, name) {
  return (fields[name] || [])
    .filter(test => test.existing || String(test.input ?? '') !== '' || String(test.output ?? '') !== '')
    .map(test => ({ input: String(test.input ?? ''), output: String(test.output ?? '') }));
}

function problem(fields) {
  return command('/api/problems', {
    title: required(fields, 'title', 'Название'),
    statement: required(fields, 'statement', 'Условие'),
    input_format: text(fields, 'input_format'),
    output_format: text(fields, 'output_format'),
    time_limit_ms: positiveInteger(fields, 'time_limit_ms', 'лимит времени'),
    memory_limit_mb: positiveInteger(fields, 'memory_limit_mb', 'лимит памяти'),
    samples: testCases(fields, 'samples'),
    tests: testCases(fields, 'tests'),
  }, 'Задача добавлена в банк', 'topics');
}

function testRevision(fields, context) {
  if (!context.id) throw new Error('Задача не найдена.');
  const samples = testCases(fields, 'samples');
  const tests = testCases(fields, 'tests');
  if (!samples.length && !tests.length) throw new Error('Добавьте хотя бы один тест.');
  return command(`/api/problems/${encodeURIComponent(context.id)}/test-revisions`, { samples, tests }, 'Версия тестов сохранена');
}

function workTestRevision(fields, context) {
  if (!context.id || !context.problemID) throw new Error('Работа или задача не найдена.');
  return command(`/api/works/${encodeURIComponent(context.id)}/problem-revisions`, {
    problem_id: context.problemID,
    revision: positiveInteger(fields, 'revision', 'версия тестов'),
  }, 'Версия применяется к новым посылкам', undefined, 'PATCH');
}

function submission(fields, context) {
  const source = String(fields.source || '');
  if (!source.trim()) throw new Error('Добавьте исходный код решения.');
  if (new TextEncoder().encode(source).length > sourceLimitBytes) throw new Error('Исходный код превышает 128 КБ.');
  if (!context.workID || !context.problemID) throw new Error('Не удалось определить задачу. Откройте работу заново.');
  return command('/api/submissions', {
    work_id: context.workID,
    problem_id: context.problemID,
    language: required(fields, 'language', 'Язык решения'),
    source,
  }, 'Решение отправлено', 'submissions');
}

function topic(fields) {
  const lessonURL = text(fields, 'lesson_url');
  if (lessonURL) {
    let url;
    try { url = new URL(lessonURL); } catch { throw new Error('Укажите полную HTTPS-ссылку на занятие.'); }
    if (url.protocol !== 'https:') throw new Error('Ссылка на занятие должна начинаться с https://.');
  }
  return command('/api/topics', {
    title: required(fields, 'title', 'Название темы'),
    description: text(fields, 'description'),
    lesson_url: lessonURL,
  }, 'Тема добавлена', 'topics');
}

function review(fields, context) {
  if (!context.id) throw new Error('Посылка не найдена. Откройте её заново.');
  const verdict = text(fields, 'verdict');
  const allowed = ['accepted', 'wrong_answer', 'compilation_error', 'runtime_error', 'time_limit', 'memory_limit', 'output_limit'];
  if (!allowed.includes(verdict)) throw new Error('Выберите результат проверки.');
  return command(`/api/submissions/${encodeURIComponent(context.id)}`, {
    verdict, feedback: text(fields, 'feedback'),
  }, 'Результат сохранён', undefined, 'PATCH');
}

const builders = {
  login,
  register,
  work,
  'work-draft': editDraft,
  problem,
  'test-revision': testRevision,
  'work-test-revision': workTestRevision,
  submission,
  topic,
  review,
  group: fields => command('/api/groups', { name: required(fields, 'name', 'Название группы') },
    'Группа создана. Код приглашения доступен в разделе «Ученики».', 'students'),
};

/** @returns {FormCommand} */
export function buildFormCommand(name, fields, context = {}) {
  const build = builders[name];
  if (!build) throw new Error('Неизвестная форма. Обновите страницу.');
  return build(fields, context);
}
