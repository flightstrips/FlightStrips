import {useState} from "react";
import type {AuthorizedSession, OwnMembership} from "@/api/sessions";
import {membershipKey} from "@/api/sessions";
import {Button} from "@/components/ui/button";

export function AirportSessionChooser({sessions, onChoose, onCancel}: {
  sessions: AuthorizedSession[];
  onChoose: (member: OwnMembership) => void;
  onCancel?: () => void;
}) {
  const [sessionID, setSessionID] = useState<number | null>(null);
  const session = sessions.find(s => s.session_id === sessionID) ?? (sessions.length === 1 && sessions[0].instances.length > 1 ? sessions[0] : null);
  return <div className="fixed inset-0 z-50 flex items-center justify-center bg-primary text-white p-6" role="dialog" aria-modal="true" aria-labelledby="airport-choice-title">
    <div className="w-full max-w-xl rounded-lg border border-white/20 bg-bay-dark p-6 shadow-xl">
      <h1 id="airport-choice-title" className="text-2xl font-semibold">{session ? "Choose your plugin instance" : "Choose airport session"}</h1>
      <p className="mt-2 text-white/75">This choice applies to this tab. Open another tab to work at another airport.</p>
      <div className="mt-6 flex flex-col gap-3">
        {session ? <>
          <p className="font-semibold">{session.airport} · {session.environment}</p>
          {session.instances.map(member => <Button key={membershipKey(member)} variant="secondary" className="h-auto whitespace-normal justify-start p-4 text-left" onClick={() => onChoose(member)}>
            <span>{member.callsign} · {member.position}<br/><span className="text-xs">{member.instance_id} · {member.role} · {member.ready ? (member.read_only ? "Read-only" : "Ready") : "Synchronizing"}</span></span>
          </Button>)}
          {sessions.length > 1 && <Button variant="outline" onClick={() => setSessionID(null)}>Back to airports</Button>}
        </> : sessions.map(s => <Button key={s.session_id} variant="secondary" className="h-auto justify-between p-4" onClick={() => s.instances.length === 1 ? onChoose(s.instances[0]) : setSessionID(s.session_id)}>
          <span>{s.airport} · {s.environment}</span><span className="text-xs">{s.instances.length === 1 ? s.instances[0].callsign : `${s.instances.length} own instances`}</span>
        </Button>)}
        {onCancel && <Button variant="outline" onClick={onCancel}>Keep current airport</Button>}
      </div>
    </div>
  </div>;
}
