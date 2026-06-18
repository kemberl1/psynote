// ProtectedRoute — гейт доступа к приложению (Этап 9, docs/08 §3, docs/09 §3).
// Неавторизованный → редирект на /login (с запоминанием, куда он шёл).
// Пока идёт первичное восстановление сессии — показываем спиннер, чтобы не
// «мигать» логином при валидном refresh-токене.
import { Navigate, Outlet, useLocation } from "react-router-dom";
import { Spinner } from "../components/ui";
import { useAuth } from "./AuthContext";

export function ProtectedRoute() {
  const { isAuthenticated, initializing } = useAuth();
  const location = useLocation();

  if (initializing) {
    return (
      <div className="auth-bootstrap" role="status" aria-label="Загрузка сессии">
        <Spinner size="lg" />
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  return <Outlet />;
}
