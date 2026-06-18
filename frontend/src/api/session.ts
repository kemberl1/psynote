// Управление токенами сессии врача (Этап 9, docs/09 §1.3).
//
// ⭐ РЕШЕНИЕ ПО ХРАНЕНИЮ + ОТКЛОНЕНИЕ ОТ docs/09 (обосновано):
// docs/09 §1.3 рекомендует refresh-токен в httpOnly-cookie. У нас — чистая
// SPA + JSON-API (без серверного рендеринга и без общего домена cookie в dev,
// где фронт :5174 и gateway :8081 — это cross-site), поэтому:
//   - ACCESS-токен живёт ТОЛЬКО в памяти (этот модуль) — не в localStorage:
//     самый чувствительный токен не переживает перезагрузку и недоступен из
//     других вкладок/после закрытия — минимизируем окно XSS-кражи;
//   - REFRESH-токен кладём в localStorage, чтобы переживать перезагрузку
//     страницы (UX: не логиниться заново). Это осознанный компромисс.
//
// РИСКИ (честно, docs/09 §5 Threat Model):
//   - localStorage доступен JS ⇒ при XSS refresh-токен можно украсть. Митигизация:
//     refresh РОТИРУЕТСЯ при каждом использовании и ОТЗЫВАЕТСЯ на бэке (старый
//     становится бесполезен); CSP/экранирование на фронте; короткий TTL access.
//   - httpOnly-cookie была бы строже против XSS, но добавляет CSRF-поверхность и
//     требует same-site/CORS-cookie настройки; для дипломного MVP принят
//     localStorage-вариант с ротацией. Точка усиления на будущее — перейти на
//     httpOnly-cookie, бэк к этому готов (refresh уже opaque и отзывной).
//
// Access в памяти + подписка на разлогин позволяют AuthContext среагировать на
// протухшую сессию (редирект на /login).

const REFRESH_STORAGE_KEY = "psynote.refresh_token";

let accessToken: string | null = null;
let refreshToken: string | null = readStoredRefresh();

/** Слушатели события «сессия завершена» (refresh не удался / logout). */
type Listener = () => void;
const sessionEndedListeners = new Set<Listener>();

function readStoredRefresh(): string | null {
  try {
    return localStorage.getItem(REFRESH_STORAGE_KEY);
  } catch {
    return null;
  }
}

/** Текущий access-токен (в памяти) или null. */
export function getAccessToken(): string | null {
  return accessToken;
}

/** Текущий refresh-токен (localStorage) или null. */
export function getRefreshToken(): string | null {
  return refreshToken;
}

/** Сохраняет выданную пару токенов (после login/refresh). */
export function setTokens(access: string, refresh: string): void {
  accessToken = access;
  refreshToken = refresh;
  try {
    localStorage.setItem(REFRESH_STORAGE_KEY, refresh);
  } catch {
    /* приватный режим/квота — переживём, access всё равно в памяти */
  }
}

/** Полностью очищает сессию (logout / провал refresh). */
export function clearTokens(): void {
  accessToken = null;
  refreshToken = null;
  try {
    localStorage.removeItem(REFRESH_STORAGE_KEY);
  } catch {
    /* ignore */
  }
}

/** Подписка на завершение сессии. Возвращает функцию отписки. */
export function onSessionEnded(fn: Listener): () => void {
  sessionEndedListeners.add(fn);
  return () => sessionEndedListeners.delete(fn);
}

/** Уведомляет подписчиков, что сессия завершилась (нужен повторный логин). */
export function notifySessionEnded(): void {
  for (const fn of sessionEndedListeners) fn();
}
