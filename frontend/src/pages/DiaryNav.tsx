// Переключатель режимов генерации: один день / пакет за период (docs/08 §4.3).
import { NavLink } from "react-router-dom";

export function DiaryNav() {
  return (
    <nav className="diary-nav" role="tablist" aria-label="Режим генерации дневников">
      <NavLink
        to="/diary"
        end
        className={({ isActive }) =>
          `diary-nav__btn${isActive ? " diary-nav__btn--active" : ""}`
        }
        role="tab"
      >
        Один день
      </NavLink>
      <NavLink
        to="/diary/batch"
        className={({ isActive }) =>
          `diary-nav__btn${isActive ? " diary-nav__btn--active" : ""}`
        }
        role="tab"
      >
        Период
      </NavLink>
    </nav>
  );
}
