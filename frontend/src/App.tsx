// PsyNote — корневой роутинг приложения врача (docs/08 §3).
// Каждый тип документа — отдельный роут (масштабируемость, docs/08 §3, AD-8).
// Этап 9 (аутентификация): публичные /login и /register; всё приложение
// (AppShell + /diary + /requests/:id) — под ProtectedRoute (неавторизованный →
// /login). Изоляция истории по врачу обеспечивается бэкендом (docs/09 §3).
import { Navigate, Route, Routes } from "react-router-dom";
import { ProtectedRoute } from "./auth/ProtectedRoute";
import { AppShell } from "./components/layout/AppShell";
import { EmptyState } from "./components/ui";
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
    </Routes>
  );
}

export default App;
