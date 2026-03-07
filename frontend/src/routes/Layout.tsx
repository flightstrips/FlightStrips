import { Outlet } from "react-router";
import CommandBar from "@/components/commandbar/CommandBar";
import { WebSocketProvider } from "@/providers/websocket-provider";

export default function Dashboard() {
  const wsUrl = (window as any).__APP_CONFIG__?.wsUrl ?? "ws://localhost:2994/frontEndEvents";

  return (
    <WebSocketProvider url={wsUrl}>
      <div>
        <Outlet />
        <CommandBar />
      </div>
    </WebSocketProvider>
  );
}

