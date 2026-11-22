export interface FlightMenuPlacement {
  left: number;
  top: number;
}

/** Space kept between the flight menu and the window edges. */
export const FLIGHT_MENU_MARGIN = 8;

/**
 * Places the flight menu with its top-left corner over the clicked callsign,
 * then pushes it back inside the window so it never extends past an edge.
 * A menu taller than the window is pinned to the top margin; the caller caps
 * its height so it scrolls instead of running below the screen.
 */
export function placeFlightMenu(
  anchor: {left: number; top: number},
  size: {width: number; height: number} | null,
  viewport: {width: number; height: number},
): FlightMenuPlacement {
  const clamp = (value: number, extent: number, available: number) =>
    Math.max(FLIGHT_MENU_MARGIN, Math.min(value, available - extent - FLIGHT_MENU_MARGIN));
  if (size === null) return {left: anchor.left, top: anchor.top};
  return {
    left: clamp(anchor.left, size.width, viewport.width),
    top: clamp(anchor.top, size.height, viewport.height),
  };
}
