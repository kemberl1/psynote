// Каркас админки: вкладки «Корпус / Поддержка / Отзывы».
import { NavLink, Outlet } from "react-router-dom";
import { useAdminSupportSummary } from "../api/queries";
import "./admin.css";

export function AdminLayout() {
  const summary = useAdminSupportSummary(true);
  const unread = summary.data?.unread_threads ?? 0;

  return (
    <div className="admin-shell">
      <nav className="admin-tabs" aria-label="Разделы админки">
        <NavLink to="/admin" end className={tabClass}>
          Корпус
        </NavLink>
        <NavLink to="/admin/support" className={tabClass}>
          Поддержка
          {unread > 0 && (
            <span className="admin-tabs__badge">{unread > 9 ? "9+" : unread}</span>
          )}
        </NavLink>
        <NavLink to="/admin/feedback" className={tabClass}>
          Отзывы
        </NavLink>
      </nav>
      <Outlet />
    </div>
  );
}

function tabClass({ isActive }: { isActive: boolean }) {
  return `admin-tabs__link${isActive ? " admin-tabs__link--active" : ""}`;
}
