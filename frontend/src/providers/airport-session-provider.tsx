import {useCallback, useEffect, useRef, useState, type ReactNode} from "react";
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
const SESSION_LOSS_GRACE_MS = 5 * 60_000;
function readHint(): {session: number; instance: string} | null {
  const params = new URLSearchParams(window.location.search);
  if (params.has("session") && params.has("instance")) return {session: Number(params.get("session")), instance: params.get("instance")!};
  try { return JSON.parse(sessionStorage.getItem(SELECTION_KEY) ?? "null"); } catch { return null; }
}

export function AirportSessionProvider({children, url}: {children: ReactNode; url: string}) {
  const {getAccessTokenSilently, isAuthenticated, isLoading, logout} = useAuth0();
  const [retainedState, setRetainedState] = useState<WebSocketState | undefined>(undefined);
  const captureState = useCallback((state: WebSocketState) => {setRetainedState(state);}, []);
  const [{sessions, discoveredAt}, setDiscovery] = useState<{sessions: AuthorizedSession[]; discoveredAt: number}>({sessions: [], discoveredAt: 0});
  const [selected, setSelected] = useState<OwnMembership | null>(null);
  const [hint] = useState(readHint);
  const [choosing, setChoosing] = useState(false);
  const [manualChoice, setManualChoice] = useState(false);
  const missingSession = useRef<{id: number; since: number} | null>(null);
  const fallbackSuppressed = useRef(false);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const choose = useCallback((member: OwnMembership) => {
    sessionStorage.setItem(SELECTION_KEY, JSON.stringify({session: member.session_id, instance: member.instance_id}));
    setRetainedState(undefined);
    setSelected(member);
    setChoosing(false);
    setManualChoice(false);
    missingSession.current = null;
    fallbackSuppressed.current = false;
  }, []);

  useEffect(() => {
    if (!isAuthenticated || isLoading) return;
    const abort = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const refresh = async () => {
      try {
        const next = await discoverSessions(getAccessTokenSilently, abort.signal);
        if (abort.signal.aborted) return;
        setDiscovery({sessions: next, discoveredAt: Date.now()});
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
  }, [getAccessTokenSilently, isAuthenticated, isLoading, logout]);

  useEffect(() => {
    if (!loaded) return;
    if (selected) {
      const currentSession = sessions.find(s => s.session_id === selected.session_id);
      const own = currentSession?.instances.filter(m => m.instance_id === selected.instance_id) ?? [];
      if (own.length > 0) {
        // Only this physical instance's membership counts as recovery.
        missingSession.current = null;
        fallbackSuppressed.current = false;
        // Temporary plugin loss never rebinds to a different physical instance.
        if (own.length === 1) setSelected(own[0]);
        return;
      }
      // Only a withdrawn airport membership may fall back. A disconnected
      // physical plugin keeps its board until that plugin recovers.
      const instanceOnline = sessions.some(s => s.instances.some(m => m.instance_id === selected.instance_id));
      if (!instanceOnline) {
        missingSession.current = null;
        return;
      }
      if (missingSession.current?.id !== selected.session_id) {
        missingSession.current = {id: selected.session_id, since: discoveredAt};
      }
      // Use a successful discovery after the grace period before falling back.
      // Explicit chooser navigation always takes precedence over automation.
      if (!manualChoice && !fallbackSuppressed.current && discoveredAt - missingSession.current.since >= SESSION_LOSS_GRACE_MS && sessions.length === 1 && sessions[0].instances.length === 1) {
        choose(sessions[0].instances[0]);
      }
      return;
    }
    if (manualChoice) return;
    // A saved session narrows the choice only while it is authorized.
    // Stale hints must not turn an unambiguous discovery into a prompt.
    const session = (hint && sessions.find(s => s.session_id === hint.session)) || (sessions.length === 1 ? sessions[0] : undefined);
    const preferred = hint && session?.instances.filter(m => m.instance_id === hint.instance && m.session_id === hint.session);
    if (preferred?.length === 1) choose(preferred[0]);
    else if (session?.instances.length === 1) choose(session.instances[0]);
  }, [choose, discoveredAt, hint, loaded, manualChoice, selected, sessions]);

  useEffect(() => {
    if (selected) sessionStorage.setItem(SELECTION_KEY, JSON.stringify({session:selected.session_id, instance:selected.instance_id}));
  }, [selected]);

  if (!selected) {
    if (loaded && sessions.length > 0) return <AirportSessionChooser sessions={sessions} initialSessionID={hint?.session} onChoose={choose} onNavigate={() => setManualChoice(true)}/>;
    return <div className="w-screen min-h-svh flex flex-col justify-center items-center bg-primary text-white">
      <h1 className="text-4xl font-semibold">{loaded ? "Waiting for ES connection" : "Connecting to FlightStrips..."}</h1>
      {error && <p role="status" className="mt-4">{error}. Retrying...</p>}
    </div>;
  }
  const retained = retainedState;
  const canRetain = retained?.boardContext?.session_id === selected.session_id && retained.boardContext.instance_id === selected.instance_id && retained.boardContext.environment === selected.environment && retained.boardDefinition?.definition_revision === selected.definition_revision;
  const ownPluginAvailable = sessions.some(s => s.session_id === selected.session_id && s.instances.some(m => membershipKey(m) === membershipKey(selected) && m.ready));
  const keepCurrentAirport = () => {
    // Cancellation applies to this loss episode, including a plugin outage.
    // Recovery or an explicit selection restores normal automatic fallback.
    fallbackSuppressed.current = !sessions.some(s => s.session_id === selected.session_id && s.instances.some(m => m.instance_id === selected.instance_id));
    setChoosing(false);
    setManualChoice(false);
  };
  return <AirportSessionContext.Provider value={{selected, sessions, openChooser: () => {setManualChoice(true); setChoosing(true);}}}>
    <WebSocketProvider key={membershipKey(selected)} url={url} selection={selected} retainedState={canRetain ? retained : undefined} onStateChanged={captureState} ownPluginAvailable={ownPluginAvailable} selectionControls={<AirportSwitcher/>}>{children}</WebSocketProvider>
    {choosing && <AirportSessionChooser sessions={sessions} onChoose={choose} onCancel={keepCurrentAirport}/>}
  </AirportSessionContext.Provider>;
}
