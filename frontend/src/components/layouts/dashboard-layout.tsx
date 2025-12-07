import { Outlet } from "react-router";

export default function DashboardLayout() {
  return (
    <div>
      <header>Dashboard Header</header>
      <main>
        <Outlet />
      </main>
      <footer>Dashboard Footer</footer>
    </div>
  );
}
