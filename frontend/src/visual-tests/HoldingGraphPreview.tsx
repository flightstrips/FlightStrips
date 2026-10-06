import {createRoot} from "react-dom/client";
import {TMTHoldingGraph} from "@/components/aman/TMTHoldingGraph";
import type {AMANHoldingEntry} from "@/api/aman";
import "@/index.css";

const now = new Date("2026-10-04T18:00:00Z");
function entries(levels: number[]): AMANHoldingEntry[] {
  return levels.map((level, index) => ({
    callsign: `SAS${1000 + index}`, holding: "OLPIB", cleared_altitude: level * 100,
    eat: new Date(now.valueOf() + (index + 1) * 60_000).toISOString(),
    source_status: "fresh", observed_at: now.toISOString(),
  }));
}

createRoot(document.getElementById("root")!).render(
  <div className="flex gap-2 bg-[#292929] p-2">
    <div data-testid="compact" style={{width: 210, height: 290}}>
      <TMTHoldingGraph compact holding="OLPIB" now={now} entries={entries([300, 300, 299, 120, 120, 119, 90, 90])} />
    </div>
    <div data-testid="crowded" style={{width: 128, height: 230}}>
      <TMTHoldingGraph compact holding="OLPIB" now={now} entries={entries(Array.from({length: 30}, () => 120))} />
    </div>
    <div data-testid="full" style={{width: 500, height: 400}}>
      <TMTHoldingGraph holding="OLPIB" now={now} entries={entries([300, 300, 120, 120, 120, 90, 90])} />
    </div>
    <div data-testid="release-order" style={{width: 210, height: 615}}>
      <TMTHoldingGraph compact holding="ROSBI" now={now} entries={[
        {callsign: "BAW822", holding: "ROSBI", cleared_altitude: 25000, eat: "2026-10-04T18:15:00Z", source_status: "fresh", observed_at: now.toISOString()},
        {callsign: "DLH10W", holding: "ROSBI", cleared_altitude: 25000, eat: "2026-10-04T18:45:00Z", source_status: "fresh", observed_at: now.toISOString()},
        {callsign: "SAS1871", holding: "ROSBI", cleared_altitude: 25000, eat: "2026-10-04T18:30:00Z", source_status: "fresh", observed_at: now.toISOString()},
        {callsign: "NOEAT", holding: "ROSBI", cleared_altitude: 25000, eat: null, source_status: "fresh", observed_at: now.toISOString()},
      ]} />
    </div>
    <div className="aman-tmt-dashboard" data-testid="dashboard" style={{width: 720, height: 900}}>
      <div className="aman-tmt-traffic bg-[#3c3c3c]">Traffic prediction</div>
      <div className="my-2 flex gap-1"><button type="button">Holdings</button><button type="button">Warnings (0)</button></div>
      <div className="aman-tmt-holdings">
        {["OLPIB", "LUGAS", "MONAK", "ROSBI", "TUDLO"].map(holding =>
          <TMTHoldingGraph compact key={holding} holding={holding} now={now} entries={entries([120, 110, 100])} />)}
      </div>
    </div>
  </div>,
);
