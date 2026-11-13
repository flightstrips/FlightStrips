import {readFileSync} from "node:fs";
import {describe, expect, it} from "vitest";

import {canInsertAMANFlight, getAMANHeaderReadModel, isAMANStateEvent, replaceAMANState} from "../../src/api/aman";

const path = process.env.AMAN_CONTRACT_OUTPUT;
if (!path) throw new Error("Run npm run test:contract:aman to generate real Go publisher output first.");
const scenarios = JSON.parse(readFileSync(path, "utf8")) as {name: string; event: unknown}[];
function scenario(name: string) {
  const event = scenarios.find((item) => item.name === name)?.event;
  if (!isAMANStateEvent(event)) throw new Error(`Go publisher scenario ${name} was rejected by the frontend`);
  return event;
}

describe("Go AMAN publisher → TypeScript state replacement", () => {
  it("retains stale guidance while allowing STAR recovery from current surveillance", () => {
    const event = scenario("star_recovery_fresh_surveillance");
    expect(event.data.flights[0]).toMatchObject({data_status: "stale", surveillance_fresh: true, star_selection_required: true});
    expect(canInsertAMANFlight(event.data.flights[0])).toBe(true);
    const invalid = structuredClone(event);
    (invalid.data.flights[0] as unknown as Record<string, unknown>).surveillance_fresh = "fresh";
    expect(isAMANStateEvent(invalid)).toBe(false);
  });

  it("preserves fallback provenance and controller STAR choices", () => {
    const event = scenario("basic_terminal_fallback");
    expect(event.data.flights[0]).toMatchObject({basic_estimate: true, controller_star_family: "TESPI", prediction_reason: "basic_terminal_estimate:route provider unavailable"});
    expect(Object.values(event.data.insertion_star_families ?? {})).toContainEqual(["MONAK", "TESPI"]);
  });
  it.each(scenarios)("accepts the serialized $name state", ({name, event}) => {
    expect(isAMANStateEvent(event)).toBe(true);
    const result = replaceAMANState(null, event);
    expect(result).toMatchObject({accepted: true, error: null});
    expect(result.state).not.toBeNull();
    if (result.state) expect(getAMANHeaderReadModel(result.state).availability).toBe(name === "airport_source_outage" ? "degraded" : "ready");
  });

  it("keeps ground forecasts and the original spacing warning in the same replacement", () => {
    const event = scenario("ground_forecast_with_spacing_conflict");
    const result = replaceAMANState(null, event);
    expect(result.accepted).toBe(true);
    const forecasts = result.state?.traffic_prediction?.buckets.flatMap((bucket) => bucket.flights) ?? [];
    const ground = forecasts.find((flight) => flight.callsign === "GROUND");
    expect(ground).toMatchObject({airborne: false, timing_source: "vatsim_planned"});
    const held = event.data.flights.find((flight) => flight.callsign === "SAS123")!;
    expect(Date.parse(ground!.landing_at)).toBeGreaterThan(Date.parse(held.slot!.time));
    expect(result.state?.warnings).toContainEqual(expect.objectContaining({
      source: "sequence", code: "protected_same_star_spacing", callsign: "CONFLICT-B", related_callsign: "CONFLICT-A",
    }));
    expect(result.state?.warnings?.some((warning) => warning.code === "ground_arrival_behind_holding")).toBe(false);
  });

  it("keeps the airport and neighbouring guidance available during the production failure", () => {
    const event = scenario("unreliable_with_healthy_neighbour");
    const result = replaceAMANState(null, event);
    expect(result.state?.warnings).toContainEqual(expect.objectContaining({
      source: "traffic_prediction", component: "prediction", code: "prediction_unreliable", callsign: "CCA877Q",
    }));
    const unreliable = result.state?.flights.find((flight) => flight.callsign === "CCA877Q");
    expect(unreliable?.slot).not.toBeNull();
    expect(unreliable?.gain_loss_seconds).toBeNull();
    expect(result.state?.flights.find((flight) => flight.callsign === "SAS123")?.gain_loss_seconds).toBe(120);
    expect(result.state?.traffic_prediction?.buckets).toHaveLength(12);
  });

  it("recovers guidance and clears the warning on the next accepted replacement", () => {
    const degraded = replaceAMANState(null, scenario("production_unreliable"));
    const recovered = replaceAMANState(degraded.state, scenario("recovered"));
    expect(recovered).toMatchObject({accepted: true, error: null, status: "ready"});
    expect(recovered.state?.flights[0].gain_loss_seconds).not.toBeNull();
    expect(recovered.state?.warnings?.some((warning) => warning.code === "prediction_unreliable")).toBe(false);
  });

  it("shows an inferred STAR forecast and clears its warning after the assignment matches", () => {
    const inferred = replaceAMANState(null, scenario("inferred_star_runway"));
    expect(inferred).toMatchObject({accepted: true, error: null, status: "ready"});
    expect(inferred.state?.warnings).toContainEqual(expect.objectContaining({
      source: "traffic_prediction", component: "prediction", code: "star_runway_inferred", callsign: "SAS123",
    }));
    expect(inferred.state?.flights[0].gain_loss_seconds).not.toBeNull();
    const matched = replaceAMANState(inferred.state, scenario("matched_star_runway"));
    expect(matched).toMatchObject({accepted: true, error: null, status: "ready"});
    expect(matched.state?.warnings?.some((warning) => warning.code === "star_runway_inferred")).toBe(false);
  });

  it.each(scenarios)("still rejects corrupted required fields in $name", ({event}) => {
    const corrupt = structuredClone(event) as {data: {flights: unknown}};
    corrupt.data.flights = null;
    expect(replaceAMANState(null, corrupt)).toMatchObject({accepted: false, state: null, error: "invalid_aman_state"});
  });
});
