import { Outlet } from "react-router";
import CommandBar from "@/components/commandbar/CommandBar";
import { WebSocketProvider } from "@/api/websocket-provider";

export default function Dashboard() {
  // The WebSocket server URL - replace with your actual WebSocket server URL
  // TODO move
  const wsUrl = "ws://localhost:2994/frontEndEvents";

  return (
    <WebSocketProvider url={wsUrl}>
      <div>
        <Outlet />
        <CommandBar />
      </div>
    </WebSocketProvider>
  );
}
