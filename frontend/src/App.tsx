// PsyNote — корневой роутинг приложения врача (docs/08 §3).
// Этап 10: добавлен /admin для загрузки документов (admin-роль).
import { Navigate, Route, Routes } from "react-router-dom";
import { AdminRoute } from "./auth/AdminRoute";
import { ProtectedRoute } from "./auth/ProtectedRoute";
import { AppShell } from "./components/layout/AppShell";
import { EmptyState } from "./components/ui";
import { AdminPage } from "./pages/AdminPage";
import { AuthPage } from "./pages/AuthPage";
import { DiaryPage } from "./pages/DiaryPage";
import { RequestDetailPage } from "./pages/RequestDetailPage";

function App() {
  return (
    <Routes>
      {/* Публичные экраны аутентификации (Этап 9). */}
      <Route path="/login" element={<AuthPage mode="login" />} />
      <Route path="/register" element={<AuthPage mode="register" />} />

      {/* Защищённое приложение: только для авторизованного врача. */}
      <Route element={<ProtectedRoute />}>
        <Route element={<AppShell />}>
          {/* Главная → генерация дневников (ядро MVP). */}
          <Route index element={<Navigate to="/diary" replace />} />
          <Route path="diary" element={<DiaryPage />} />
          {/* Просмотр прошлого результата из истории. */}
          <Route path="requests/:id" element={<RequestDetailPage />} />
          {/* Фолбэк. */}
          <Route
            path="*"
            element={
              <EmptyState
                icon="🧭"
                title="Страница не найдена"
                text="Такого раздела пока нет. Вернитесь к созданию нового дневника."
              />
            }
          />
        </Route>
      </Route>

      {/* Админка: защищена admin-ролей (Этап 10). */}
      <Route element={<ProtectedRoute />}>
        <Route element={<AppShell />}>
          <Route element={<AdminRoute />}>
            <Route path="admin" element={<AdminPage />} />
          </Route>
        </Route>
      </Route>
    </Routes>
  );
}

export default App;
