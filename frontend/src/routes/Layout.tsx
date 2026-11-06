import { Outlet } from "react-router";
import CommandBar from "@/components/commandbar/CommandBar";
import { AirportSessionProvider } from "@/providers/airport-session-provider";
import { Toaster } from "sonner";

export default function Dashboard() {
  const wsUrl = window.__APP_CONFIG__?.wsUrl ?? "ws://localhost:8090/frontEndEvents";

  return (
    <AirportSessionProvider url={wsUrl}>
      <div>
        <Outlet />
        <CommandBar />
        <Toaster richColors position="top-right" />
      </div>
    </AirportSessionProvider>
  );
}
