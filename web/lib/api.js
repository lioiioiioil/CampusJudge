/** An HTTP failure that retains its status for session-expiry handling. */
export class ApiError extends Error {
  constructor(message, status = 0) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

/**
 * Same-origin JSON transport. Mutations are never automatically retried: a lost
 * response does not prove that the server failed to persist the change.
 * @param {typeof fetch} fetchRequest
 */
export function createAPI(fetchRequest = globalThis.fetch.bind(globalThis)) {
  async function request(path, { method = 'GET', body } = {}) {
    const headers = method === 'GET' ? {} : {
      'Content-Type': 'application/json',
      'X-Campus-Request': '1',
    };
    let response;
    try {
      response = await fetchRequest(path, {
        method,
        headers,
        credentials: 'same-origin',
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
    } catch {
      throw new ApiError('Нет связи с сервером. Проверьте подключение. Перед повторной отправкой решения обновите историю посылок.');
    }
    let result;
    try {
      result = await response.json();
    } catch {
      throw new ApiError('Сервер вернул неожиданный ответ. Попробуйте обновить страницу.', response.status);
    }
    if (!response.ok) {
      throw new ApiError(result.error || `Ошибка запроса: ${response.status}`, response.status);
    }
    return result;
  }

  return {
    request,
    session: () => request('/api/session'),
    snapshot: () => request('/api/state'),
    logout: () => request('/api/logout', { method: 'POST', body: {} }),
  };
}
