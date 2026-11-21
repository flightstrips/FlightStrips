import {useState} from "react";
import type {AuthorizedSession, OwnMembership} from "@/api/sessions";
import {membershipKey} from "@/api/sessions";
import {Button} from "@/components/ui/button";
import {FlightStripsPanel} from "@/components/ui/flightstrips-dialog";

export function AirportSessionChooser({sessions, initialSessionID, onChoose, onCancel, onNavigate}: {
  sessions: AuthorizedSession[];
  initialSessionID?: number;
  onChoose: (member: OwnMembership) => void;
  onCancel?: () => void;
  onNavigate?: () => void;
}) {
  const [airport, setAirport] = useState<string | null>();
  const navigate = (code: string | null) => {
    onNavigate?.();
    setAirport(code);
  };

  const airports = new Map<string, OwnMembership[]>();
  for (const session of sessions) {
    airports.set(session.airport, [...(airports.get(session.airport) ?? []), ...session.instances]);
  }
  const hint = airport === undefined ? sessions.find(s => s.session_id === initialSessionID) : undefined;
  const selectedAirport = hint?.airport ?? airport ?? (airports.size === 1 ? airports.keys().next().value : undefined);
  const instances = hint?.instances ?? (selectedAirport ? airports.get(selectedAirport) : undefined);
  const choosingInstance = instances && (instances.length > 1 || Boolean(hint || airport));
  return <div className="fixed inset-0 z-50 flex items-center justify-center bg-bay-dark/90 p-6" role="dialog" aria-modal="true" aria-labelledby="airport-choice-title">
    <FlightStripsPanel className="max-h-[90svh] w-full max-w-xl overflow-y-auto">
      <h1 id="airport-choice-title" className="text-center text-lg font-semibold">{choosingInstance ? "Choose your plugin instance" : "Choose airport session"}</h1>
      <p className="text-sm">This choice applies to this tab. Open another tab to work at another airport.</p>
      <div className="flex flex-col gap-3">
        {airports.size === 0 && <p role="status">No airports available on this connection.</p>}
        {choosingInstance ? <>
          <p className="font-semibold">{selectedAirport}</p>
          {instances.map(member => <Button key={membershipKey(member)} variant="trf" className="h-auto items-start whitespace-normal p-3 text-left text-black" onClick={() => onChoose(member)}>
            <span>{member.callsign} · {member.position}<br/><span className="text-xs">{member.instance_id} · {member.role} · {member.ready ? (member.read_only ? "Read-only" : "Ready") : "Synchronizing"}</span></span>
          </Button>)}
          {(airports.size > 1 || hint) && <Button variant="darkaction" onClick={() => navigate(null)}>Back to airports</Button>}
        </> : Array.from(airports, ([code, members]) => <Button key={code} variant="trf" className="h-10 text-black" onClick={() => members.length === 1 ? onChoose(members[0]) : navigate(code)}>
          {code}
        </Button>)}
        {onCancel && <Button variant="darkaction" onClick={onCancel}>Keep current airport</Button>}
      </div>
    </FlightStripsPanel>
  </div>;
}
