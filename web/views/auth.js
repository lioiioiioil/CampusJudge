import { icon } from '../lib/format.js';
import { brand, field } from '../lib/components.js';

export function renderAuth({ authTab, demo }) {
  const register = authTab === 'register';
  return `<div class="auth-layout">
    <section class="auth-story">
      ${brand()}
      <div class="auth-story-body">
        <div class="eyebrow">Меньше рутины. Больше практики.</div>
        <h1>Знания растут,<br>когда есть<br><em>интересная задача.</em></h1>
        <p>Темы, домашние задания и контесты — в одном пространстве преподавателя и учеников.</p>
        <div class="auth-illustration" aria-hidden="true">
          <div class="auth-illustration-top"><span>Путь к следующему открытию</span><span class="badge green">Шаг за шагом</span></div>
          <h3>Тема → практика → обратная связь</h3>
          <div class="auth-flow"><span>01 · Тема</span><span>02 · Практика</span><span>03 · Разбор</span></div>
          <div class="auth-illustration-bottom"><span>Понять. Попробовать. Разобраться.</span>${icon('code', 15)}</div>
        </div>
      </div>
      <div class="auth-story-footer">${icon('server', 13)} Ваши материалы. Ваше учебное пространство.</div>
    </section>
    <section class="auth-main"><div class="auth-form-wrap">
      <h2>${register ? 'Присоединиться к группе' : 'С возвращением'}</h2>
      <p class="muted">${register ? 'Введите код, который дал преподаватель, и создайте свой аккаунт.' : 'Войдите, чтобы продолжить учить или учиться.'}</p>
      <div class="auth-tabs">
        <button class="auth-tab${!register ? ' active' : ''}" data-action="auth-tab" data-value="login">Вход</button>
        <button class="auth-tab${register ? ' active' : ''}" data-action="auth-tab" data-value="register">Регистрация ученика</button>
      </div>
      <form data-form="${register ? 'register' : 'login'}">
        ${register ? field('Ваше имя', 'name', { required: true, placeholder: 'Как к вам обращаться' }) : ''}
        <label class="form-field"><span class="form-label">Логин</span>
          <input class="input" name="username" required minlength="3" maxlength="64" autocomplete="username" placeholder="Ваш логин">
        </label>
        <label class="form-field"><span class="form-label">Пароль</span>
          <input class="input" name="password" type="password" required ${register ? 'minlength="8"' : ''}
            autocomplete="${register ? 'new-password' : 'current-password'}" placeholder="${register ? 'Не менее 8 символов' : 'Ваш пароль'}">
        </label>
        ${register ? field('Код группы', 'invite_code', { required: true, placeholder: 'Код от преподавателя' }) : ''}
        <p class="form-error" role="alert"></p>
        <button type="submit" class="btn primary full auth-submit">
          ${register ? 'Создать аккаунт' : 'Войти в пространство'} ${icon('arrow', 15)}
        </button>
      </form>
      <p class="auth-caption">${register ? 'Уже есть аккаунт? Вернитесь на вкладку «Вход».' : 'Аккаунт преподавателя создаётся при первом запуске сервера.'}</p>
      ${demo ? `<div class="demo-box">
        <strong>Тестовое пространство</strong><br>
        Преподаватель: <code>teacher / campus-demo</code><br>
        <button class="demo-account" data-action="demo-login">Войти преподавателем</button><br>
        Ученик: зарегистрируйтесь с кодом <code>DEMOCLASS</code>.
      </div>` : ''}
    </div></section>
  </div>`;
}
