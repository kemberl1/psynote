// AppShell — каркас приложения (docs/08 §6): топбар + сайдбар истории +
// рабочая область с роут-контентом (через <Outlet/>).
import { Outlet } from "react-router-dom";
import { HistorySidebar } from "./HistorySidebar";
import { TopBar } from "./TopBar";
import "./layout.css";

export function AppShell() {
  return (
    <div className="shell">
      <TopBar />
      <HistorySidebar />
      <main className="main">
        <div className="main__inner">
          <Outlet />
        </div>
      </main>
    </div>
  );
}
