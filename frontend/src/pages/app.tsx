import { useAuth0 } from "@auth0/auth0-react";
import { Navigate } from "react-router";
import { WebSocketProvider } from "@/providers/websocket-provider";
import CommandBar from "@/components/commandbar/CommandBar";
import AppRouter from "@/routes/AppRouter";

export default function AppPage() {
  const { isAuthenticated, isLoading } = useAuth0();
  const wsUrl = window.__APP_CONFIG__?.wsUrl ?? "ws://localhost:8090/frontEndEvents";

  if (isLoading) {
    return (
      <div className="w-screen min-h-svh flex justify-center items-center bg-primary text-white text-4xl font-semibold">
        Loading...
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ returnTo: "/app" }} />;
  }

  return (
    <WebSocketProvider url={wsUrl}>
      <div>
        <AppRouter />
        <CommandBar />
      </div>
    </WebSocketProvider>
  );
}
