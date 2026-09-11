import {fireEvent, render, screen} from "@testing-library/react";
import {useState} from "react";
import {beforeEach, describe, expect, it} from "vitest";

import {AMANAircraftTarget, type AMANAircraftTargetField} from "./AMANAircraftTarget";
import {
  AMANAircraftTargetPreferenceControls,
} from "./AMANAircraftTargetPreferences";
import {
  AMAN_TARGET_PREFERENCES_KEY,
  defaultAMANAircraftTargetPreferences,
  fieldsForAMANAircraftTargetSide,
  readAMANAircraftTargetPreferences,
  useAMANAircraftTargetPreferences,
  type AMANAircraftTargetPreferences,
} from "./amanAircraftTargetPreferenceModel";

const fields: AMANAircraftTargetField[] = [
  {id: "feeder-fix-eta", label: "Feeder-fix ETA", value: "10:12"},
  {id: "total-delay", label: "Total delay", value: "D05"},
  {id: "runway", label: "Runway", value: "22L"},
];
const flight = {callsign: "SAS123", data_status: "fresh", freeze_reason: "none", gain_loss_seconds: -120, lifecycle_state: "unstable"} as const;
const guidance = {authoritative: true, connected: true};

function Harness({initial = defaultAMANAircraftTargetPreferences(), persist = false}: {initial?: AMANAircraftTargetPreferences; persist?: boolean}) {
  const local = useState(initial);
  const stored = useAMANAircraftTargetPreferences();
  const [preferences, setPreferences] = persist ? stored : local;
  return (
    <>
      <AMANAircraftTargetPreferenceControls onChange={setPreferences} preferences={preferences} />
      <AMANAircraftTarget
        flight={flight}
        guidance={guidance}
        leadingFields={fieldsForAMANAircraftTargetSide(fields, preferences, "feeder")}
        trailingFields={fieldsForAMANAircraftTargetSide(fields, preferences, "runway")}
      />
    </>
  );
}

describe("local AMAN target field preferences", () => {
  beforeEach(() => localStorage.clear());

  it("defaults both sides to the compact callsign and current-delay core", () => {
    render(<Harness />);
    expect(screen.getByRole("button", {name: /Select SAS123/})).toHaveTextContent("SAS123L02U");
    expect(screen.getAllByRole("checkbox")).toHaveLength(12);
  });

  it("keeps feeder and runway side visibility independent and persists changes", () => {
    render(<Harness persist />);
    fireEvent.click(screen.getByRole("checkbox", {name: "Show Feeder-fix ETA on feeder side"}));
    expect(screen.getByRole("button", {name: /Select SAS123/})).toHaveTextContent("10:12SAS123L02U");
    expect(screen.getByRole("checkbox", {name: "Show Feeder-fix ETA on runway side"})).not.toBeChecked();
    expect(JSON.parse(localStorage.getItem(AMAN_TARGET_PREFERENCES_KEY)!)).toMatchObject({feeder: ["feeder-fix-eta"], runway: []});
  });

  it("reorders visible fields and compacts hidden fields without placeholders", () => {
    render(<Harness initial={{version: 1, feeder: ["feeder-fix-eta", "total-delay", "runway"], runway: []}} />);
    fireEvent.click(screen.getByRole("button", {name: "Move Runway earlier on feeder side"}));
    fireEvent.click(screen.getByRole("checkbox", {name: "Show Total delay on feeder side"}));
    const target = screen.getByRole("button", {name: /Select SAS123/});
    expect(target).toHaveTextContent("10:1222LSAS123L02U");
    expect(target.querySelectorAll("[data-field]")).toHaveLength(2);
  });

  it.each([
    ["malformed JSON", "{"],
    ["old version", JSON.stringify({version: 0, feeder: ["runway"], runway: []})],
  ])("recovers defaults from %s", (_, stored) => {
    localStorage.setItem(AMAN_TARGET_PREFERENCES_KEY, stored);
    expect(readAMANAircraftTargetPreferences()).toEqual(defaultAMANAircraftTargetPreferences());
  });

  it("filters stale and duplicate field IDs while retaining valid side order", () => {
    localStorage.setItem(AMAN_TARGET_PREFERENCES_KEY, JSON.stringify({version: 1, feeder: ["runway", "retired", "runway"], runway: ["wtc"]}));
    expect(readAMANAircraftTargetPreferences()).toEqual({version: 1, feeder: ["runway"], runway: ["wtc"]});
  });

  it("labels each side group and every ordering control accessibly", () => {
    render(<Harness initial={{version: 1, feeder: ["runway", "wtc"], runway: []}} />);
    expect(screen.getByRole("group", {name: "Feeder-fix-side target fields"})).toBeInTheDocument();
    expect(screen.getByRole("group", {name: "Runway-side target fields"})).toBeInTheDocument();
    expect(screen.getByRole("button", {name: "Move WTC earlier on feeder side"})).toBeEnabled();
  });
});
