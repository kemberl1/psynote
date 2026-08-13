// AppShell — каркас приложения (docs/08 §6): топбар + сайдбар истории +
// рабочая область с роут-контентом (через <Outlet/>).
import { Outlet, useLocation } from "react-router-dom";
import { SupportWidget } from "../support/SupportWidget";
import { HistorySidebar } from "./HistorySidebar";
import { TopBar } from "./TopBar";
import "./layout.css";

export function AppShell() {
  const { pathname } = useLocation();
  const wide = pathname.startsWith("/admin");

  return (
    <div className="shell">
      <TopBar />
      <HistorySidebar />
      <main className="main">
        <div className={`main__inner${wide ? " main__inner--wide" : ""}`}>
          <Outlet />
        </div>
      </main>
      <SupportWidget />
    </div>
  );
}
