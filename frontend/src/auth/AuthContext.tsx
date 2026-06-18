// AuthContext — состояние сессии врача на фронте (Этап 9, docs/08 §4.3, docs/09).
//
// Держит профиль текущего врача и статус загрузки; предоставляет login/register/
// logout. Источник истины по ТОКЕНАМ — модуль api/session (access в памяти,
// refresh в localStorage). При старте, если есть refresh-токен, пытаемся
// восстановить сессию через GET /auth/me (access добудется авто-refresh’ем в
// client.ts при 401). Подписка onSessionEnded ловит протухание сессии из любого
// запроса (перехват 401 → провал refresh) и сбрасывает профиль → ProtectedRoute
// уводит на /login.
import {
    createContext,
    useCallback,
    useContext,
    useEffect,
    useMemo,
    useState,
    type ReactNode,
} from "react";
import {
    fetchMe,
    login as loginRequest,
    logout as logoutRequest,
    register as registerRequest,
} from "../api/endpoints";
import {
    clearTokens,
    getRefreshToken,
    onSessionEnded,
    setTokens,
} from "../api/session";
import type { DoctorProfile } from "../api/types";

interface AuthContextValue {
  doctor: DoctorProfile | null;
  /** true пока идёт первичное восстановление сессии (bootstrap). */
  initializing: boolean;
  isAuthenticated: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (
    email: string,
    password: string,
    displayName: string,
  ) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [doctor, setDoctor] = useState<DoctorProfile | null>(null);
  const [initializing, setInitializing] = useState(true);

  // Bootstrap: если есть refresh-токен — пробуем подтянуть профиль.
  useEffect(() => {
    let cancelled = false;
    async function bootstrap() {
      if (!getRefreshToken()) {
        setInitializing(false);
        return;
      }
      try {
        const me = await fetchMe();
        if (!cancelled) setDoctor(me);
      } catch {
        // refresh недействителен/истёк — чистим и остаёмся разлогиненными.
        clearTokens();
        if (!cancelled) setDoctor(null);
      } finally {
        if (!cancelled) setInitializing(false);
      }
    }
    void bootstrap();
    return () => {
      cancelled = true;
    };
  }, []);

  // Реакция на завершение сессии из любого запроса (перехват 401 в client.ts).
  useEffect(() => {
    return onSessionEnded(() => setDoctor(null));
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    const tokens = await loginRequest({ email, password });
    setTokens(tokens.access_token, tokens.refresh_token);
    const me = await fetchMe();
    setDoctor(me);
  }, []);

  const register = useCallback(
    async (email: string, password: string, displayName: string) => {
      await registerRequest({ email, password, display_name: displayName });
      // Сразу логиним после регистрации (UX): выдаём токены и тянем профиль.
      const tokens = await loginRequest({ email, password });
      setTokens(tokens.access_token, tokens.refresh_token);
      const me = await fetchMe();
      setDoctor(me);
    },
    [],
  );

  const logout = useCallback(async () => {
    const refresh = getRefreshToken();
    try {
      if (refresh) await logoutRequest(refresh); // отзыв сессии на бэке
    } catch {
      /* даже если бэк недоступен — локально разлогиниваемся */
    } finally {
      clearTokens();
      setDoctor(null);
    }
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      doctor,
      initializing,
      isAuthenticated: doctor !== null,
      login,
      register,
      logout,
    }),
    [doctor, initializing, login, register, logout],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

/** Хук доступа к аутентификации. Бросает, если вызван вне AuthProvider. */
export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used within <AuthProvider>");
  }
  return ctx;
}
