import {describe, expect, it} from "vitest";
import {authorizedSessions, membershipKey, type OwnMembership, type SessionDiscovery} from "./sessions";

function membership(overrides: Partial<OwnMembership> = {}): OwnMembership {
  return {session_id: 1, airport: "AAAA", environment: "LIVE", instance_id: "own", membership_id: "a", transport_generation: "18446744073709551615", generation: "18446744073709551614", master_epoch: "1", role: "slave", read_only: false, ready: true, view: "ekch", definition_revision: "revision", actions: ["strip.move"], capabilities: {cdm: true, stands: false, text_pilot: true, pdc: false, aman: false}, callsign: "OWN", position: "118.000", ...overrides};
}
function discovery(...members: OwnMembership[]): SessionDiscovery {
  return {transports: members.map(m => ({instance_id: m.instance_id, transport_generation: m.transport_generation, memberships: [m]}))};
}

describe("authorized own membership discovery", () => {
  it("handles zero, one and multiple sessions without merging environments", () => {
    expect(authorizedSessions({transports: []})).toEqual([]);
    expect(authorizedSessions(discovery(membership()))).toHaveLength(1);
    const sessions = authorizedSessions(discovery(membership(), membership({session_id: 2, airport: "BBBB"}), membership({session_id: 3, environment: "SWEATBOX_test"})));
    expect(sessions).toHaveLength(3);
    expect(sessions.map(s => s.session_id)).toEqual([1, 3, 2]);
  });
  it("preserves each compatible instance and scoped observer/readiness/capabilities", () => {
    const observer = membership({instance_id: "second", role: "observer", ready: false, read_only: true});
    const [session] = authorizedSessions(discovery(membership(), observer));
    expect(session.instances).toHaveLength(2);
    expect(session.instances[1]).toEqual(observer);
    expect(membershipKey(session.instances[0])).not.toBe(membershipKey(session.instances[1]));
    expect(membershipKey(observer)).toContain("18446744073709551615");
  });
  it("does not fall back to EKCH for unknown views or incomplete definitions", () => {
    expect(authorizedSessions(discovery(membership({view: "radis"}), membership({definition_revision: ""})))).toEqual([]);
  });
});
