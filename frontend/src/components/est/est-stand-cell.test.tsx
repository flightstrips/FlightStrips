import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import EstStandCell from "./EstStandCell";
import { Bay, type FrontendStrip } from "@/api/models";

function makeStrip(overrides: Partial<FrontendStrip> = {}): FrontendStrip {
  return {
    callsign: "SAS123",
    origin: "EKCH",
    destination: "ENGM",
    alternate: "",
    route: "",
    remarks: "",
    runway: "22R",
    squawk: "1000",
    assigned_squawk: "1000",
    sid: "",
    cleared_altitude: 0,
    requested_altitude: 0,
    heading: 0,
    aircraft_type: "A320",
    aircraft_category: "",
    stand: "A18",
    capabilities: "",
    communication_type: "",
    eobt: "1420",
    tobt: "1420",
    tsat: "1425",
    ctot: "",
    eldt: "",
    bay: Bay.NotCleared,
    release_point: "",
    version: 1,
    sequence: 0,
    next_controllers: [],
    previous_controllers: [],
    owner: "",
    pdc_state: "NONE",
    start_req: false,
    marked: false,
    runway_cleared: false,
    runway_confirmed: false,
    registration: "",
    ...overrides,
  } as FrontendStrip;
}

const stand = { label: "A18", left: 0, top: 0 };

describe("EstStandCell", () => {
  it("renders stand label", () => {
    render(
      <EstStandCell
        stand={stand}
        blocked={false}
        selected={false}
        actionActive={false}
        blinking={false}
        startReqActive={false}
        ctotImproved={false}
        nowMs={Date.now()}
        onClick={() => {}}
      />,
    );
    expect(screen.getByText("A18")).toBeDefined();
  });

  it("renders callsign when strip is present", () => {
    render(
      <EstStandCell
        stand={stand}
        strip={makeStrip()}
        blocked={false}
        selected={false}
        actionActive={false}
        blinking={false}
        startReqActive={false}
        ctotImproved={false}
        nowMs={Date.now()}
        onClick={() => {}}
      />,
    );
    expect(screen.getByText("SAS123")).toBeDefined();
  });

  it("renders backend assignment metadata without a matching strip", () => {
    render(
      <EstStandCell
        stand={stand}
        assignment={{ callsign: "NAX456", stand: "A18", direction: "ARRIVAL", stage: "CONFIRMED", source: "AUTOMATIC", eta: "2026-07-12T14:20:00Z", expires_at: "2026-07-12T15:20:00Z" }}
        blocked={false}
        selected={false}
        actionActive={false}
        blinking={false}
        startReqActive={false}
        ctotImproved={false}
        nowMs={Date.now()}
        onClick={() => {}}
      />,
    );
    expect(screen.getByText("NAX456")).toBeDefined();
    expect(screen.getByText("CONFIRMED · AUTOMATIC")).toBeDefined();
    expect(screen.getByText("ETA 1420")).toBeDefined();
    expect(screen.getByText("EXP 1520")).toBeDefined();
  });

  it("retains the assigned callsign when the stand is also blocked", () => {
    render(
      <EstStandCell
        stand={stand}
        assignment={{ callsign: "SAS123", stand: "A18", direction: "DEPARTURE", stage: "OCCUPIED", source: "AUTOMATIC" }}
        blocked
        blockReason="Blocked by stand A17"
        selected={false}
        actionActive={false}
        blinking={false}
        startReqActive={false}
        ctotImproved={false}
        nowMs={Date.now()}
        onClick={() => {}}
      />,
    );
    expect(screen.getByText("SAS123")).toBeDefined();
  });

  it("hides callsign when blocked", () => {
    render(
      <EstStandCell
        stand={stand}
        strip={makeStrip()}
        blocked={true}
        selected={false}
        actionActive={false}
        blinking={false}
        startReqActive={false}
        ctotImproved={false}
        nowMs={Date.now()}
        onClick={() => {}}
      />,
    );
    expect(screen.queryByText("SAS123")).toBeNull();
  });

  it("shows blocked style when blocked", () => {
    render(
      <EstStandCell
        stand={stand}
        blocked={true}
        selected={false}
        actionActive={false}
        blinking={false}
        startReqActive={false}
        ctotImproved={false}
        nowMs={Date.now()}
        onClick={() => {}}
      />,
    );
    const button = screen.getByRole("button");
    expect(button.className).toContain("bg-[#4A4A4A]");
  });

  it("shows departure style for not-cleared strip", () => {
    render(
      <EstStandCell
        stand={stand}
        strip={makeStrip({ bay: Bay.NotCleared })}
        blocked={false}
        selected={false}
        actionActive={false}
        blinking={false}
        startReqActive={false}
        ctotImproved={false}
        nowMs={Date.now()}
        onClick={() => {}}
      />,
    );
    const button = screen.getByRole("button");
    expect(button.className).toContain("bg-[#D9D9D9]");
  });

  it("shows arrival style for stand bay", () => {
    render(
      <EstStandCell
        stand={stand}
        strip={makeStrip({ bay: Bay.Stand })}
        blocked={false}
        selected={false}
        actionActive={false}
        blinking={false}
        startReqActive={false}
        ctotImproved={false}
        nowMs={Date.now()}
        onClick={() => {}}
      />,
    );
    const button = screen.getByRole("button");
    expect(button.className).toContain("bg-[#FFF28E]");
  });

  it("shows push style for push bay", () => {
    render(
      <EstStandCell
        stand={stand}
        strip={makeStrip({ bay: Bay.Push })}
        blocked={false}
        selected={false}
        actionActive={false}
        blinking={false}
        startReqActive={false}
        ctotImproved={false}
        nowMs={Date.now()}
        onClick={() => {}}
      />,
    );
    const button = screen.getByRole("button");
    expect(button.className).toContain("bg-[#DD6A12]");
  });

  it("shows action active style", () => {
    render(
      <EstStandCell
        stand={stand}
        strip={makeStrip()}
        blocked={false}
        selected={false}
        actionActive={true}
        blinking={false}
        startReqActive={false}
        ctotImproved={false}
        nowMs={Date.now()}
        onClick={() => {}}
      />,
    );
    const button = screen.getByRole("button");
    expect(button.className).toContain("bg-[#131376]");
  });

  it("calls onClick with stand label and strip", () => {
    let clicked = false;
    const onClick = (stand: string, strip: FrontendStrip | undefined) => {
      expect(stand).toBe("A18");
      expect(strip?.callsign).toBe("SAS123");
      clicked = true;
    };
    render(
      <EstStandCell
        stand={stand}
        strip={makeStrip()}
        blocked={false}
        selected={false}
        actionActive={false}
        blinking={false}
        startReqActive={false}
        ctotImproved={false}
        nowMs={Date.now()}
        onClick={onClick}
      />,
    );
    screen.getByRole("button").click();
    expect(clicked).toBe(true);
  });
});
