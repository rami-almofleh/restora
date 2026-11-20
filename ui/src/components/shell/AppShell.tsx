import { Outlet } from "react-router-dom";
import { BottomNav } from "./BottomNav";
import { Sidebar } from "./Sidebar";
import { TopBar } from "./TopBar";

export function AppShell() {
  return (
    <div className="shell">
      <Sidebar />
      <div className="shell__main">
        <TopBar />
        <main className="content">
          <div className="content__inner">
            <Outlet />
          </div>
        </main>
      </div>
      <BottomNav />
    </div>
  );
}
