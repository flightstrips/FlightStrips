import {useState} from "react";
import type {AuthorizedSession, OwnMembership} from "@/api/sessions";
import {membershipKey} from "@/api/sessions";
import {Button} from "@/components/ui/button";

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
  return <div className="fixed inset-0 z-50 flex items-center justify-center bg-primary text-white p-6" role="dialog" aria-modal="true" aria-labelledby="airport-choice-title">
    <div className="w-full max-w-xl rounded-lg border border-white/20 bg-bay-dark p-6 shadow-xl">
      <h1 id="airport-choice-title" className="text-2xl font-semibold">{choosingInstance ? "Choose your plugin instance" : "Choose airport session"}</h1>
      <p className="mt-2 text-white/75">This choice applies to this tab. Open another tab to work at another airport.</p>
      <div className="mt-6 flex flex-col gap-3">
        {airports.size === 0 && <p role="status">No airports available on this connection.</p>}
        {choosingInstance ? <>
          <p className="font-semibold">{selectedAirport}</p>
          {instances.map(member => <Button key={membershipKey(member)} variant="secondary" className="h-auto whitespace-normal justify-start p-4 text-left" onClick={() => onChoose(member)}>
            <span>{member.callsign} · {member.position}<br/><span className="text-xs">{member.instance_id} · {member.role} · {member.ready ? (member.read_only ? "Read-only" : "Ready") : "Synchronizing"}</span></span>
          </Button>)}
          {(airports.size > 1 || hint) && <Button variant="outline" onClick={() => navigate(null)}>Back to airports</Button>}
        </> : Array.from(airports, ([code, members]) => <Button key={code} variant="secondary" className="h-auto justify-start p-4" onClick={() => members.length === 1 ? onChoose(members[0]) : navigate(code)}>
          {code}
        </Button>)}
        {onCancel && <Button variant="outline" onClick={onCancel}>Keep current airport</Button>}
      </div>
    </div>
  </div>;
}
