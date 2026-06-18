// AuthPage — экраны входа и регистрации врача (Этап 9, docs/08 §4.3, docs/09).
// Тёмная тема в стиле Cursor (дизайн-токены index.css). Один компонент с двумя
// режимами (login | register), переключение без перезагрузки. Валидация на
// клиенте (email, длина пароля, совпадение паролей) + аккуратная обработка
// ошибок API (неверный логин, занятый email) через friendlyError.
//
// ПРИВАТНОСТЬ: это форма аккаунта ВРАЧА (его email/имя — не ПДн пациента).
// Пароль уходит на бэкенд по HTTPS и там хешируется Argon2id; на фронте не
// хранится. После успеха AuthContext тянет /me и роутер уводит в приложение.
import { useState, type FormEvent } from "react";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { ApiError, friendlyError } from "../api/errors";
import { useAuth } from "../auth/AuthContext";
import { Banner, Button, Spinner } from "../components/ui";
import "./auth.css";

type Mode = "login" | "register";

const MIN_PASSWORD = 8;

interface LocationState {
  from?: { pathname?: string };
}

export function AuthPage({ mode }: { mode: Mode }) {
  const { login, register, isAuthenticated, initializing } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();

  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [password2, setPassword2] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fieldError, setFieldError] = useState<string | null>(null);

  // Уже авторизован → не показываем формы, уводим в приложение.
  if (!initializing && isAuthenticated) {
    const from = (location.state as LocationState | null)?.from?.pathname;
    return <Navigate to={from && from !== "/login" ? from : "/diary"} replace />;
  }

  const isRegister = mode === "register";

  function validate(): string | null {
    if (!email.includes("@") || !email.includes(".")) {
      return "Введите корректный email.";
    }
    if (password.length < MIN_PASSWORD) {
      return `Пароль должен быть не короче ${MIN_PASSWORD} символов.`;
    }
    if (isRegister && password !== password2) {
      return "Пароли не совпадают.";
    }
    return null;
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const ve = validate();
    if (ve) {
      setFieldError(ve);
      return;
    }
    setFieldError(null);
    setSubmitting(true);
    try {
      if (isRegister) {
        await register(email.trim(), password, displayName.trim());
      } else {
        await login(email.trim(), password);
      }
      const from = (location.state as LocationState | null)?.from?.pathname;
      navigate(from && from !== "/login" ? from : "/diary", { replace: true });
    } catch (err) {
      // Специфичные сообщения для частых случаев.
      if (err instanceof ApiError) {
        if (err.code === "UNAUTHORIZED") {
          setError("Неверный email или пароль.");
        } else if (err.code === "EMAIL_TAKEN") {
          setError("Этот email уже зарегистрирован. Попробуйте войти.");
        } else if (err.code === "BAD_REQUEST") {
          setError(err.message || "Проверьте введённые данные.");
        } else {
          setError(friendlyError(err).detail);
        }
      } else {
        setError("Что-то пошло не так. Повторите попытку.");
      }
    } finally {
      setSubmitting(false);
    }
  }

  function switchMode(to: Mode) {
    setError(null);
    setFieldError(null);
    navigate(to === "login" ? "/login" : "/register", { replace: true });
  }

  return (
    <div className="auth">
      <div className="auth__card">
        <div className="auth__brand">
          <span className="auth__logo" aria-hidden="true">
            P
          </span>
          <span className="auth__brand-name">
            Psy<span className="accent">Note</span>
          </span>
        </div>

        <h1 className="auth__title">
          {isRegister ? "Создание аккаунта" : "Вход в систему"}
        </h1>
        <p className="auth__subtitle">
          {isRegister
            ? "Зарегистрируйте аккаунт врача — история запросов будет видна только вам."
            : "Войдите, чтобы продолжить работу с дневниками."}
        </p>

        {error && (
          <div className="auth__banner">
            <Banner tone="danger" title="Не удалось" text={error} />
          </div>
        )}

        <form className="auth__form" onSubmit={onSubmit} noValidate>
          <label className="auth__field">
            <span className="auth__label">Email</span>
            <input
              className="auth__input"
              type="email"
              autoComplete="email"
              placeholder="doctor@example.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              disabled={submitting}
              required
            />
          </label>

          {isRegister && (
            <label className="auth__field">
              <span className="auth__label">Имя / должность (необязательно)</span>
              <input
                className="auth__input"
                type="text"
                autoComplete="name"
                placeholder="Врач-психиатр Т."
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                disabled={submitting}
              />
            </label>
          )}

          <label className="auth__field">
            <span className="auth__label">Пароль</span>
            <input
              className="auth__input"
              type="password"
              autoComplete={isRegister ? "new-password" : "current-password"}
              placeholder="Минимум 8 символов"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              disabled={submitting}
              required
            />
          </label>

          {isRegister && (
            <label className="auth__field">
              <span className="auth__label">Повторите пароль</span>
              <input
                className="auth__input"
                type="password"
                autoComplete="new-password"
                placeholder="Ещё раз"
                value={password2}
                onChange={(e) => setPassword2(e.target.value)}
                disabled={submitting}
                required
              />
            </label>
          )}

          {fieldError && <div className="auth__hint-error">{fieldError}</div>}

          <Button type="submit" variant="primary" block size="lg" disabled={submitting}>
            {submitting ? <Spinner /> : isRegister ? "Зарегистрироваться" : "Войти"}
          </Button>
        </form>

        <div className="auth__switch">
          {isRegister ? (
            <>
              Уже есть аккаунт?{" "}
              <button type="button" className="auth__link" onClick={() => switchMode("login")}>
                Войти
              </button>
            </>
          ) : (
            <>
              Нет аккаунта?{" "}
              <button type="button" className="auth__link" onClick={() => switchMode("register")}>
                Зарегистрироваться
              </button>
            </>
          )}
        </div>
      </div>

      <p className="auth__privacy">
        Данные пациентов обезличиваются до обработки. Пароль хранится только в
        виде криптографического хеша (Argon2id).
      </p>
    </div>
  );
}
