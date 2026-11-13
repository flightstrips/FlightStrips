import {Auth0Context, initialContext, type Auth0ContextInterface} from "@auth0/auth0-react";
import {useEffect, useState} from "react";
import {createRoot} from "react-dom/client";
import {AMANFlightDetailDialog} from "@/components/aman/AMANFlightDetailDialog";
import {AMANAircraftTarget} from "@/components/aman/AMANAircraftTarget";
import {flightInformationDetail} from "./flight-information-fixture";
import "@/index.css";

const previewAuth = {
  ...initialContext, isAuthenticated: true, isLoading: false,
  getAccessTokenSilently: (async () => "visual-preview") as Auth0ContextInterface["getAccessTokenSilently"],
};

let missingEAT = false;
// The review page supplies fixture responses. Playwright's ?api mode exercises
// the production fetch path with intercepted HTTP responses instead.
if (!new URLSearchParams(location.search).has("api")) {
  const originalFetch = window.fetch.bind(window);
  window.fetch = (input, init) => {
    const url = input instanceof Request ? input.url : String(input);
    if (new URL(url, location.href).pathname === "/api/aman/airports/EKCH/flights/SAS1778/detail") {
      return Promise.resolve(Response.json({...flightInformationDetail, holding_plan: missingEAT ? null : flightInformationDetail.holding_plan}));
    }
    return originalFetch(input, init);
  };
}

export function Preview() {
  const [open, setOpen] = useState(!new URLSearchParams(location.search).has("targets"));
  const [basic, setBasic] = useState(new URLSearchParams(location.search).has("basic"));
  const [missing, setMissing] = useState(false);
  const [revision, setRevision] = useState(17);
  const estimate = {basic_estimate: basic, prediction_reason: basic ? "Prediction provider unavailable; using surveillance and terminal geometry." : undefined};
  const target = {callsign: "SAS1778", data_status: "fresh" as const, lifecycle_state: "stable" as const, freeze_reason: "none" as const, gain_loss_seconds: -120, ...estimate};
  useEffect(() => {
    const refresh = () => setRevision((value) => value + 1);
    window.addEventListener("aman-preview-refresh", refresh);
    return () => window.removeEventListener("aman-preview-refresh", refresh);
  }, []);
  return <Auth0Context.Provider value={previewAuth}><main className="min-h-screen bg-[#292929] p-5 text-white">
    <div className="flex flex-wrap gap-4 text-sm">
      <button className="rounded border px-3 py-2" onClick={() => setOpen(true)}>Open flight information</button>
      <label className="flex items-center gap-2"><input type="checkbox" checked={missing} onChange={(event) => {missingEAT = event.target.checked; setMissing(missingEAT);}} />EAT unavailable</label>
      <label className="flex items-center gap-2"><input type="checkbox" checked={basic} onChange={(event) => setBasic(event.target.checked)} />Basic estimate</label>
    </div>
    <section aria-label="Aircraft target previews" className="mt-5 flex flex-wrap items-center gap-5 bg-[#555355] p-4">
      <AMANAircraftTarget flight={target} guidance={{authoritative: true, connected: true}} onSelect={() => setOpen(true)} />
      <AMANAircraftTarget compact flight={target} guidance={{authoritative: true, connected: true}} onSelect={() => setOpen(true)} />
      <AMANAircraftTarget compact delayFirst emphasis="primary" flight={target} guidance={{authoritative: true, connected: true}} onSelect={() => setOpen(true)} />
    </section>
    {open && <AMANFlightDetailDialog airport="EKCH" callsign="SAS1778" estimate={estimate} revision={revision} onClose={() => setOpen(false)} />}
  </main></Auth0Context.Provider>;
}

createRoot(document.getElementById("root")!).render(<Preview />);
