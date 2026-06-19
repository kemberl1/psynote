// Верхняя панель: брендинг PsyNote + профиль текущего врача и выход (docs/08 §4.3).
// Этап 10: ссылка на /admin видна только для role=admin.
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

  const name =
    doctor?.display_name?.trim() ||
    doctor?.email?.split("@")[0] ||
    "Врач";
  const initial = name.charAt(0).toUpperCase();
  const isAdmin = doctor?.role === "admin";

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
        {isAdmin && (
          <Link to="/admin" className="topbar__admin-link">
            <Button variant="ghost" size="sm">
              Админка
            </Button>
          </Link>
        )}
        <div className="profile" title={doctor?.email ?? "Профиль врача"}>
          <span className="profile__avatar" aria-hidden="true">
            {initial}
          </span>
          <span className="profile__name">{name}</span>
          {isAdmin && <Badge tone="accent" mono>admin</Badge>}
        </div>
        <Button variant="ghost" size="sm" onClick={onLogout} aria-label="Выйти из аккаунта">
          Выйти
        </Button>
      </div>
    </header>
  );
}
