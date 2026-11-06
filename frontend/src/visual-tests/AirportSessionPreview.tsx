/* eslint-disable react-refresh/only-export-components -- isolated browser fixture */
import {createRoot} from "react-dom/client";
import {Auth0Context, type Auth0ContextInterface} from "@auth0/auth0-react";
import {AirportSessionProvider} from "@/providers/airport-session-provider";
import {useWebSocketStore} from "@/store/store-hooks";
import AppRouter from "@/routes/AppRouter";
import CommandBar from "@/components/commandbar/CommandBar";
import "@/index.css";

const auth = {isAuthenticated: true, isLoading: false, getAccessTokenSilently: async () => "fixture-token", logout: async () => {}} as unknown as Auth0ContextInterface;
window.__APP_CONFIG__ = {...window.__APP_CONFIG__, wsUrl: `ws://${window.location.host}/frontEndEvents`};
function Board() {
  const view = useWebSocketStore(s => s.boardDefinition?.view);
  const session = useWebSocketStore(s => s.amanSessionID);
  const readOnly = useWebSocketStore(s => s.readOnly);
  return <div data-testid="selected-board" data-session={session} data-read-only={String(readOnly)}><AppRouter/>{view !== "radis" && <CommandBar/>}</div>;
}
createRoot(document.getElementById("root")!).render(<Auth0Context.Provider value={auth}><AirportSessionProvider url={window.__APP_CONFIG__.wsUrl!}><Board/></AirportSessionProvider></Auth0Context.Provider>);
