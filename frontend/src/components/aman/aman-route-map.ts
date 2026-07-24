import type {LatLngTuple} from "leaflet";

import type {AMANCalculationLeg} from "@/api/aman-detail";

export type RouteGeometryLeg = Pick<AMANCalculationLeg, "id" | "to" | "start_latitude" | "start_longitude" | "end_latitude" | "end_longitude">;
export type MapRouteLeg = {end: LatLngTuple; id: string; start: LatLngTuple; to: string};

// Leaflet connects raw -180° and +180° longitude values through the long side
// of the world. Keep each consecutive endpoint in the nearest world copy so
// polar/oceanic routes crossing the antimeridian take their actual short path.
function nearestLongitude(reference: number, longitude: number): number {
  let result = longitude;
  while (result-reference > 180) result -= 360;
  while (result-reference < -180) result += 360;
  return result;
}

export function mapRouteLegs(legs: RouteGeometryLeg[], previousEndLongitude?: number): MapRouteLeg[] {
  let prior = previousEndLongitude;
  return legs.map((leg) => {
    const startLongitude = prior === undefined ? leg.start_longitude : nearestLongitude(prior, leg.start_longitude);
    const endLongitude = nearestLongitude(startLongitude, leg.end_longitude);
    prior = endLongitude;
    return {id: leg.id, to: leg.to, start: [leg.start_latitude, startLongitude], end: [leg.end_latitude, endLongitude]};
  });
}

export function mapLongitude(reference: number | undefined, longitude: number): number {
  return reference === undefined ? longitude : nearestLongitude(reference, longitude);
}
