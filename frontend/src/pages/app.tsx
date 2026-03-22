import { useAuth0 } from "@auth0/auth0-react";
import { WebSocketProvider } from "@/providers/websocket-provider";
import CommandBar from "@/components/commandbar/CommandBar";
import AppRouter from "@/routes/AppRouter";
import { StripContextMenu } from "@/components/strip/StripContextMenu";
import { useContextMenu, useCloseStripContextMenu } from "@/store/store-hooks";
import { CustomCursor } from "@/components/CustomCursor";

function ContextMenuOverlay() {
  const contextMenu = useContextMenu();
  const closeStripContextMenu = useCloseStripContextMenu();

  if (!contextMenu) return null;

  return (
    <StripContextMenu
      callsign={contextMenu.callsign}
      position={{ x: contextMenu.x, y: contextMenu.y }}
      onClose={closeStripContextMenu}
    />
  );
}

export default function AppPage() {
  const { isAuthenticated, isLoading, loginWithRedirect } = useAuth0();
  const wsUrl = window.__APP_CONFIG__?.wsUrl ?? "ws://localhost:8090/frontEndEvents";

  if (isLoading) {
    return (
      <div className="w-screen min-h-svh flex justify-center items-center bg-primary text-white text-4xl font-semibold">
        Loading...
      </div>
    );
  }

  if (!isAuthenticated) {
    loginWithRedirect({ appState: { returnTo: "/app" } });
    return null;
  }

  return (
    <WebSocketProvider url={wsUrl}>
      <div>
        <AppRouter />
        <CommandBar />
        <ContextMenuOverlay />
        <CustomCursor />
      </div>
    </WebSocketProvider>
  );
}
