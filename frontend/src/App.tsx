// PsyNote — корневой роутинг приложения врача (docs/08 §3).
// Этап 10: добавлен /admin для загрузки документов (admin-роль).
import { Navigate, Route, Routes } from "react-router-dom";
import { AdminRoute } from "./auth/AdminRoute";
import { ProtectedRoute } from "./auth/ProtectedRoute";
import { AppShell } from "./components/layout/AppShell";
import { EmptyState } from "./components/ui";
import { AdminFeedbackPage } from "./pages/AdminFeedbackPage";
import { AdminLayout } from "./pages/AdminLayout";
import { AdminPage } from "./pages/AdminPage";
import { AdminSupportPage } from "./pages/AdminSupportPage";
import { AuthPage } from "./pages/AuthPage";
import { BatchDiaryPage } from "./pages/BatchDiaryPage";
import { DiaryPage } from "./pages/DiaryPage";
import { RequestDetailPage } from "./pages/RequestDetailPage";
import { SettingsPage } from "./pages/SettingsPage";

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
          <Route path="diary/batch" element={<BatchDiaryPage />} />
          {/* Просмотр прошлого результата из истории. */}
          <Route path="requests/:id" element={<RequestDetailPage />} />
          <Route path="settings" element={<SettingsPage />} />
          {/* Админка: корпус, поддержка, отзывы. */}
          <Route element={<AdminRoute />}>
            <Route path="admin" element={<AdminLayout />}>
              <Route index element={<AdminPage />} />
              <Route path="support" element={<AdminSupportPage />} />
              <Route path="support/:threadId" element={<AdminSupportPage />} />
              <Route path="feedback" element={<AdminFeedbackPage />} />
            </Route>
          </Route>
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
    </Routes>
  );
}

export default App;
