import { brand, field } from '../lib/components.js';

export function renderAuth({ authTab, demo }) {
  const register = authTab === 'register';
  return `<main class="auth-layout"><section class="auth-form-wrap">
    ${brand()}
    <h1>${register ? 'Регистрация ученика' : 'Вход'}</h1>
    <p class="muted">${register ? 'Для регистрации нужен код группы от преподавателя.' : 'Войдите в учебное пространство.'}</p>
    <nav class="auth-tabs" aria-label="Вход и регистрация">
      <button class="auth-tab${!register ? ' active' : ''}" data-action="auth-tab" data-value="login">Вход</button>
      <button class="auth-tab${register ? ' active' : ''}" data-action="auth-tab" data-value="register">Регистрация</button>
    </nav>
    <form data-form="${register ? 'register' : 'login'}">
      ${register ? field('Имя', 'name', { required: true }) : ''}
      <label class="form-field"><span class="form-label">Логин</span>
        <input class="input" name="username" required minlength="3" maxlength="64" autocomplete="username">
      </label>
      <label class="form-field"><span class="form-label">Пароль</span>
        <input class="input" name="password" type="password" required ${register ? 'minlength="8"' : ''}
          autocomplete="${register ? 'new-password' : 'current-password'}">
        ${register ? '<span class="form-hint">Не менее 8 символов.</span>' : ''}
      </label>
      ${register ? field('Код группы', 'invite_code', { required: true }) : ''}
      <p class="form-error" role="alert"></p>
      <button type="submit" class="btn primary">${register ? 'Зарегистрироваться' : 'Войти'}</button>
    </form>
    ${demo ? `<aside class="demo-box">
      <h2>Тестовый доступ</h2>
      <p>Преподаватель: <code>teacher / campus-demo</code></p>
      <button class="text-button" data-action="demo-login">Войти преподавателем</button>
      <p>Для регистрации ученика код группы: <code>DEMOCLASS</code>.</p>
    </aside>` : ''}
  </section></main>`;
}
