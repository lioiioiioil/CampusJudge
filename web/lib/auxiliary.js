import { esc } from './format.js';
import { sourceLimitBytes } from './forms.js';
import { verdictNames } from './state.js';

const inputLimitBytes = 64 * 1024;

function sourceFields(form, snapshot) {
  const source = form.elements.source.value;
  const language = form.elements.language.value;
  if (!source.trim()) throw new Error('Добавьте исходный код.');
  if (new TextEncoder().encode(source).length > sourceLimitBytes) throw new Error('Исходный код превышает 128 КБ.');
  if (!snapshot.languages.some(item => item.id === language && item.automatic)) {
    throw new Error('Для выбранного языка автоматический запуск недоступен.');
  }
  return { actor_id: snapshot.user.id, language, source };
}

export function trialRequest(form, snapshot) {
  const body = {
    ...sourceFields(form, snapshot),
    work_id: form.dataset.work,
    problem_id: form.dataset.problem,
  };
  const selection = form.elements.trial_example.value;
  if (selection === 'custom') {
    const input = form.elements.trial_input.value;
    if (new TextEncoder().encode(input).length > inputLimitBytes) throw new Error('Ввод превышает 64 КБ.');
    return { ...body, input };
  }
  const sampleIndex = Number(selection);
  if (selection === '' || !Number.isSafeInteger(sampleIndex) || sampleIndex < 0) {
    throw new Error('Выберите открытый пример.');
  }
  return { ...body, sample_index: sampleIndex, test_revision: Number(form.dataset.testRevision) };
}

export function trialControls(samples = []) {
  return `<details class="activity">
    <summary>Пробный запуск</summary>
    <label class="form-field"><span class="form-label">Входные данные</span>
      <select class="input" name="trial_example">
        ${samples.map((_, index) => `<option value="${index}">Пример ${index + 1}</option>`).join('')}
        <option value="custom">Свой ввод</option>
      </select>
    </label>
    <label class="form-field" data-trial-input${samples.length ? ' hidden' : ''}>
      <span class="form-label">Свой ввод</span>
      <textarea class="input" name="trial_input" rows="3" spellcheck="false"></textarea>
    </label>
    <button type="button" class="btn" data-action="run-trial">Запустить код</button>
    <div data-auxiliary-result role="status" aria-live="polite"></div>
  </details>`;
}

export function renderAuxiliaryResult(result) {
  if (result.status === 'running') return '<p class="help-text">Код выполняется…</p>';
  if (result.status === 'error') return `<p class="form-error">${esc(result.error)}</p>`;
  const title = result.verdict === 'completed' ? 'Выполнено' : verdictNames[result.verdict] || result.verdict;
  const hasOutput = result.output || ['completed', 'accepted', 'wrong_answer'].includes(result.verdict);
  return `<p><strong>${esc(title)}</strong> · ${esc(result.time_ms)} мс</p>
    ${hasOutput ? `<p class="help-text">Фактический вывод</p><pre class="source-preview">${esc(result.output || '(пусто)')}</pre>` : ''}
    ${result.output_truncated ? '<p class="help-text">Показана только первая часть вывода.</p>' : ''}
    ${result.compile_output ? `<pre class="judge-diagnostics">${esc(result.compile_output)}</pre>` : ''}`;
}

const pause = () => new Promise(resolve => setTimeout(resolve, 1200));

// Opening another modal or ending the session prevents both further polls and
// writes to a result panel that may have been replaced by another request.
export async function pollAuxiliary(api, path, body, publish, isCurrent, wait = pause) {
  try {
    let result = await api.request(path, { method: 'POST', body });
    for (let count = 0; count < 120; count++) {
      if (!isCurrent()) return;
      publish(result);
      if (result.status !== 'running') return;
      await wait();
      if (!isCurrent()) return;
      result = await api.request(`/api/auxiliary-runs/${encodeURIComponent(result.id)}`);
    }
    if (isCurrent()) publish(result.status === 'running'
      ? { status: 'error', error: 'Истекло время ожидания результата.' } : result);
  } catch (error) {
    if (isCurrent()) throw error;
  }
}
