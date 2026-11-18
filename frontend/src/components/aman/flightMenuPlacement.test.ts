import {describe, expect, it} from "vitest";

import {placeFlightMenu} from "./flightMenuPlacement";

const viewport = {width: 1200, height: 800};
const size = {width: 160, height: 330};

describe("flight menu placement", () => {
  it("opens over the callsign when it fits", () => {
    expect(placeFlightMenu({left: 400, top: 200}, size, viewport)).toEqual({left: 400, top: 200});
  });

  it("is pushed up so it never extends below the screen", () => {
    expect(placeFlightMenu({left: 400, top: 700}, size, viewport)).toEqual({left: 400, top: 462});
  });

  it("stays inside the left, right and top edges", () => {
    expect(placeFlightMenu({left: 1150, top: -20}, size, viewport)).toEqual({left: 1032, top: 8});
    expect(placeFlightMenu({left: -50, top: 100}, size, viewport)).toEqual({left: 8, top: 100});
  });

  it("pins a menu taller than the window to the top margin", () => {
    expect(placeFlightMenu({left: 400, top: 300}, {width: 160, height: 900}, viewport)).toEqual({left: 400, top: 8});
  });

  it("uses the callsign position until the menu has been measured", () => {
    expect(placeFlightMenu({left: 400, top: 700}, null, viewport)).toEqual({left: 400, top: 700});
  });
});
