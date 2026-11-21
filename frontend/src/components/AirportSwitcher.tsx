import {useContext} from "react";
import {AirportSessionContext} from "@/providers/airport-session-context";

export function AirportSwitcher({compact = false}: {compact?: boolean}) {
  const context = useContext(AirportSessionContext);
  if (!context) return null;
  return <button type="button" aria-label="Switch airport session" title={context.selected.airport} onClick={context.openChooser} className="px-2 py-1 rounded border border-white/30 text-sm text-white bg-bay-dark hover:bg-bay-light hover:text-black">
    {context.selected.airport}{!compact && " ▾"}
  </button>;
}
