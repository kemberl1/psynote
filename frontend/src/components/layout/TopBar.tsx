// Верхняя панель: брендинг PsyNote + профиль текущего врача и выход (docs/08 §4.3).
// Этап 9: профиль показывает email/имя из /auth/me (AuthContext); «Выйти»
// отзывает refresh-сессию на бэке, чистит токены и уводит на /login.
import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../../auth/AuthContext";
import { Badge, Button } from "../ui";

export function TopBar() {
  const { doctor, logout } = useAuth();
  const navigate = useNavigate();

  async function onLogout() {
    await logout();
    navigate("/login", { replace: true });
  }

  // Имя для показа: display_name, иначе локальная часть email.
  const name =
    doctor?.display_name?.trim() ||
    doctor?.email?.split("@")[0] ||
    "Врач";
  const initial = name.charAt(0).toUpperCase();

  return (
    <header className="topbar">
      <Link to="/" className="topbar__brand" aria-label="PsyNote — на главную">
        <span className="topbar__logo" aria-hidden="true">
          P
        </span>
        <span className="topbar__name">
          Psy<span className="accent">Note</span>
        </span>
        <span className="topbar__tag">
          <Badge>генерация дневников</Badge>
        </span>
      </Link>

      <div className="topbar__right">
        <div className="profile" title={doctor?.email ?? "Профиль врача"}>
          <span className="profile__avatar" aria-hidden="true">
            {initial}
          </span>
          <span className="profile__name">{name}</span>
        </div>
        <Button variant="ghost" size="sm" onClick={onLogout} aria-label="Выйти из аккаунта">
          Выйти
        </Button>
      </div>
    </header>
  );
}
