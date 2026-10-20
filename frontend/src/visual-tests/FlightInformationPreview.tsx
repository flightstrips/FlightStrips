import {Auth0Context, initialContext, type Auth0ContextInterface} from "@auth0/auth0-react";
import {useEffect, useState} from "react";
import {createRoot} from "react-dom/client";
import {AMANFlightDetailDialog} from "@/components/aman/AMANFlightDetailDialog";
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
  const [open, setOpen] = useState(true);
  const [missing, setMissing] = useState(false);
  const [revision, setRevision] = useState(17);
  useEffect(() => {
    const refresh = () => setRevision((value) => value + 1);
    window.addEventListener("aman-preview-refresh", refresh);
    return () => window.removeEventListener("aman-preview-refresh", refresh);
  }, []);
  return <Auth0Context.Provider value={previewAuth}><main className="min-h-screen bg-[#292929] p-5 text-white">
    <div className="flex gap-4 text-sm">
      <button className="rounded border px-3 py-2" onClick={() => setOpen(true)}>Open flight information</button>
      <label className="flex items-center gap-2"><input type="checkbox" checked={missing} onChange={(event) => {missingEAT = event.target.checked; setMissing(missingEAT);}} />EAT unavailable</label>
    </div>
    {open && <AMANFlightDetailDialog airport="EKCH" callsign="SAS1778" revision={revision} onClose={() => setOpen(false)} />}
  </main></Auth0Context.Provider>;
}

createRoot(document.getElementById("root")!).render(<Preview />);
