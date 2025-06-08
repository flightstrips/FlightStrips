import { Outlet } from "react-router";
import CommandBar from "@/components/commandbar/CommandBar";

export default function Dashboard() {
  return (
    <div>
      <Outlet />
      <CommandBar />
    </div>
  );
}