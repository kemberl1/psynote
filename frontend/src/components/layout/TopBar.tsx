// Верхняя панель: брендинг PsyNote + плейсхолдер профиля врача (docs/08 §4.3).
// Аутентификация — Этап 9, поэтому профиль здесь декоративный (без меню/выхода).
import { Link } from "react-router-dom";
import { Badge } from "../ui";

export function TopBar() {
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
        {/* Плейсхолдер профиля врача — реальные данные появятся на Этапе 9. */}
        <div className="profile" title="Профиль врача (появится на этапе авторизации)">
          <span className="profile__avatar" aria-hidden="true">
            🩺
          </span>
          <span className="profile__name">Врач</span>
        </div>
      </div>
    </header>
  );
}
