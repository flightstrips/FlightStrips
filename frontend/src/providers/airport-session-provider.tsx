import {useCallback, useEffect, useState, useRef, type ReactNode} from "react";
import {useAuth0} from "@auth0/auth0-react";
import {discoverSessions, membershipKey, type AuthorizedSession, type OwnMembership} from "@/api/sessions";
import {requiresLoginPrompt} from "@/lib/authenticated-fetch";
import {AirportSessionChooser} from "@/components/AirportSessionChooser";
import {AirportSwitcher} from "@/components/AirportSwitcher";
import {AirportSessionContext} from "./airport-session-context";
import type {WebSocketState} from "@/store/store";
import {WebSocketProvider} from "./websocket-provider";

// Session storage is private to this tab. It is only an affinity hint; discovery
// still authorizes every choice. No storage event changes another tab's board.
const SELECTION_KEY = "flightstrips.airport-session.v1";
function readHint(): {session: number; instance: string} | null {
  const params = new URLSearchParams(window.location.search);
  if (params.has("session") && params.has("instance")) return {session: Number(params.get("session")), instance: params.get("instance")!};
  try { return JSON.parse(sessionStorage.getItem(SELECTION_KEY) ?? "null"); } catch { return null; }
}

export function AirportSessionProvider({children, url}: {children: ReactNode; url: string}) {
  const {getAccessTokenSilently, isAuthenticated, isLoading, logout} = useAuth0();
  const retainedState = useRef<WebSocketState | undefined>(undefined);
  const captureState = useCallback((state: WebSocketState) => {retainedState.current = state;}, []);
  const [sessions, setSessions] = useState<AuthorizedSession[]>([]);
  const [selected, setSelected] = useState<OwnMembership | null>(null);
  const [hint] = useState(readHint);
  const [choosing, setChoosing] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const choose = useCallback((member: OwnMembership) => {
    sessionStorage.setItem(SELECTION_KEY, JSON.stringify({session: member.session_id, instance: member.instance_id}));
    retainedState.current = undefined;
    setSelected(member);
    setChoosing(false);
  }, []);

  useEffect(() => {
    if (!isAuthenticated || isLoading) return;
    const abort = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const refresh = async () => {
      try {
        const next = await discoverSessions(getAccessTokenSilently, abort.signal);
        if (abort.signal.aborted) return;
        setSessions(next);
        setSelected(previous => {
          if (previous) {
            const own = next.find(s => s.session_id === previous.session_id)?.instances.filter(m => m.instance_id === previous.instance_id);
            // Only the same physical instance may rebind automatically. Other
            // instances and airports remain explicit user choices.
            return own?.length === 1 ? own[0] : previous;
          }
          const preferred = hint && next.find(s => s.session_id === hint.session)?.instances.filter(m => m.instance_id === hint.instance);
          if (preferred?.length === 1) return preferred[0];
          if (hint) return null; // Reload/reconnect must not silently enter another airport.
          return next.length === 1 && next[0].instances.length === 1 ? next[0].instances[0] : null;
        });
        setLoaded(true);
        setError(null);
      } catch (e) {
        if (abort.signal.aborted) return;
        setError(e instanceof Error ? e.message : "Unable to discover airport sessions");
        if (requiresLoginPrompt(e)) await logout({openUrl: false});
      } finally {
        if (!abort.signal.aborted) timer = setTimeout(() => void refresh(), 5000);
      }
    };
    void refresh();
    return () => {abort.abort(); clearTimeout(timer);};
  }, [getAccessTokenSilently, hint, isAuthenticated, isLoading, logout]);

  useEffect(() => {
    if (selected) sessionStorage.setItem(SELECTION_KEY, JSON.stringify({session:selected.session_id, instance:selected.instance_id}));
  }, [selected]);

  if (!selected) {
    if (loaded && sessions.length > 0) return <AirportSessionChooser sessions={sessions} onChoose={choose}/>;
    return <div className="w-screen min-h-svh flex flex-col justify-center items-center bg-primary text-white">
      <h1 className="text-4xl font-semibold">{loaded ? "Waiting for ES connection" : "Connecting to FlightStrips..."}</h1>
      {error && <p role="status" className="mt-4">{error}. Retrying...</p>}
    </div>;
  }
  const retained = retainedState.current;
  const canRetain = retained?.boardContext?.session_id === selected.session_id && retained.boardContext.instance_id === selected.instance_id && retained.boardContext.environment === selected.environment && retained.boardDefinition?.definition_revision === selected.definition_revision;
  const ownPluginAvailable = sessions.some(s => s.session_id === selected.session_id && s.instances.some(m => membershipKey(m) === membershipKey(selected) && m.ready));
  return <AirportSessionContext.Provider value={{selected, sessions, openChooser: () => setChoosing(true)}}>
    <WebSocketProvider key={membershipKey(selected)} url={url} selection={selected} retainedState={canRetain ? retained : undefined} onStateChanged={captureState} ownPluginAvailable={ownPluginAvailable} selectionControls={<AirportSwitcher/>}>{children}</WebSocketProvider>
    {choosing && <AirportSessionChooser sessions={sessions} onChoose={choose} onCancel={() => setChoosing(false)}/>}
  </AirportSessionContext.Provider>;
}
