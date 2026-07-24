import {describe, expect, it} from "vitest";

import {mapRouteLegs} from "./aman-route-map";

describe("mapRouteLegs", () => {
  it("keeps a route continuous across the antimeridian", () => {
    const legs = mapRouteLegs([
      {id: "A", to: "DATELINE", start_latitude: 70, start_longitude: 179, end_latitude: 71, end_longitude: -175},
      {id: "B", to: "NEXT", start_latitude: 71, start_longitude: -175, end_latitude: 72, end_longitude: -160},
    ]);

    expect(legs[0].start[1]).toBe(179);
    expect(legs[0].end[1]).toBe(185);
    expect(legs[1].start[1]).toBe(185);
    expect(legs[1].end[1]).toBe(200);
  });
});
