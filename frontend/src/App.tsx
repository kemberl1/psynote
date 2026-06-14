// PsyNote — корневой роутинг приложения врача (docs/08 §3).
// Каждый тип документа — отдельный роут (масштабируемость, docs/08 §3, AD-8).
// Этап 6 (MVP): / → /diary (генерация дневников) + /requests/:id (просмотр
// истории). Auth-роуты (/login, /register) и будущие типы документов
// (/primary-exam, /anamnesis, /discharge) — следующие этапы.
import { Navigate, Route, Routes } from "react-router-dom";
import { AppShell } from "./components/layout/AppShell";
import { EmptyState } from "./components/ui";
import { DiaryPage } from "./pages/DiaryPage";
import { RequestDetailPage } from "./pages/RequestDetailPage";

function App() {
  return (
    <Routes>
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
    </Routes>
  );
}

export default App;
