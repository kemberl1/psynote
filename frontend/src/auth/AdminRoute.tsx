// AdminRoute — гейт доступа к /admin (Этап 10).
// Неавторизованный → /login. Не-admin → 403-страница.
import { Outlet } from "react-router-dom";
import { EmptyState, Spinner } from "../components/ui";
import { useAuth } from "./AuthContext";

export function AdminRoute() {
  const { doctor, isAuthenticated, initializing } = useAuth();

  if (initializing) {
    return (
      <div className="auth-bootstrap" role="status" aria-label="Загрузка сессии">
        <Spinner size="lg" />
      </div>
    );
  }

  if (!isAuthenticated) {
    return (
      <EmptyState
        icon="🔒"
        title="Требуется авторизация"
        text="Войдите в систему для доступа к админке."
      />
    );
  }

  if (doctor?.role !== "admin") {
    return (
      <EmptyState
        icon="🚫"
        title="Доступ запрещён"
        text="Этот раздел доступен только администраторам."
      />
    );
  }

  return <Outlet />;
}
