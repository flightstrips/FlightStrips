import { useMemo, useState } from 'react';
import { computeArrivalRoute, computeTaxiRoute, findReleasePoint, impliedDepartureRunway } from './taxiRoute';
import { standReleasePoint } from './standReleasePoints';
import { defaultArrivalClearance } from './arrivalRoutes';

/** Where the release point in use came from. */
export type ReleasePointSource = 'atc' | 'stand' | 'pilot' | null;

interface Options {
  callsign: string;
  /** Departures start at a release point; arrivals start where they vacate the landing runway. */
  mode?: 'departure' | 'arrival';
  /** Release point ATC assigned on the strip, if any (departures). */
  atcReleasePoint: string | null;
  /** Assigned stand: the arrival destination, or for departures a stand that may fix the release point. */
  stand?: string | null;
  /** Landing runway (arrivals). */
  runway?: string | null;
}

const normalise = (value: string | null | undefined) => (value ?? '').trim().toUpperCase();

/**
 * Taxi clearance state for one flight. Values from the flight (release point, landing runway,
 * stand) are followed until the pilot types their own; after that a changed release point is
 * offered instead of overwriting it.
 */
export function useTaxiClearance({ callsign, mode = 'departure', atcReleasePoint, stand, runway }: Options) {
  const arrival = mode === 'arrival';
  const stripPoint = normalise(atcReleasePoint);
  const standDefault = arrival ? null : standReleasePoint(stand);
  const assigned: { point: string; from: 'atc' | 'stand' } | null = arrival ? null : stripPoint
    ? { point: stripPoint, from: 'atc' }
    : standDefault ? { point: standDefault, from: 'stand' } : null;
  const assignedKey = assigned ? `${assigned.from}:${assigned.point}` : null;
  const flightRunway = normalise(runway), flightStand = normalise(stand);
  const flightKey = `${mode}:${callsign}`;

  const [releasePoint, setReleasePointValue] = useState(assigned?.point ?? '');
  const [source, setSource] = useState<ReleasePointSource>(assigned?.from ?? null);
  const [landingRunway, setLandingRunwayValue] = useState(flightRunway);
  const [arrivalStand, setArrivalStandValue] = useState(flightStand);
  const [edited, setEdited] = useState({ runway: false, stand: false });
  const [typedClearance, setTypedClearance] = useState('');
  const [seen, setSeen] = useState({ flightKey, assignedKey, flightRunway, flightStand });

  // New flight: start over from the flight's values. Same flight, new values: follow them unless
  // the pilot has typed their own.
  if (seen.flightKey !== flightKey || seen.assignedKey !== assignedKey || seen.flightRunway !== flightRunway || seen.flightStand !== flightStand) {
    const newFlight = seen.flightKey !== flightKey;
    setSeen({ flightKey, assignedKey, flightRunway, flightStand });
    if (newFlight) {
      setReleasePointValue(assigned?.point ?? '');
      setSource(assigned?.from ?? null);
      setLandingRunwayValue(flightRunway);
      setArrivalStandValue(flightStand);
      setEdited({ runway: false, stand: false });
      setTypedClearance('');
    } else {
      if (source !== 'pilot') {
        setReleasePointValue(assigned?.point ?? '');
        setSource(assigned?.from ?? null);
      }
      if (!edited.runway) setLandingRunwayValue(flightRunway);
      if (!edited.stand) setArrivalStandValue(flightStand);
    }
  }

  // Arrivals start from the standard routing for the runway and stand until the pilot types their own.
  const standardClearance = arrival ? defaultArrivalClearance(landingRunway, arrivalStand) : null;
  // Until then the standard routing is only drawn as a preliminary route; it is not written in the field.
  const preliminary = !typedClearance.trim() && Boolean(standardClearance);
  const routedClearance = preliminary ? standardClearance! : typedClearance;

  const route = useMemo(
    () => (arrival ? computeArrivalRoute(landingRunway, arrivalStand, routedClearance) : computeTaxiRoute(releasePoint, routedClearance, { departureRunway: landingRunway || null })),
    [arrival, landingRunway, arrivalStand, releasePoint, routedClearance],
  );

  return {
    mode,
    /** Departure runway from the flight, marked on the map as the destination (departures). */
    /** Departure runway marked on the map: typed or from the flight, otherwise implied by the clearance. */
    departureRunway: arrival ? null : landingRunway || impliedDepartureRunway(route.parsed) || null,
    /** Where the departure runway came from. */
    departureRunwaySource: arrival ? null : landingRunway ? (!edited.runway && landingRunway === flightRunway ? 'flight' : 'pilot') : impliedDepartureRunway(route.parsed) ? 'clearance' : null,
    releasePoint,
    source,
    landingRunway,
    arrivalStand,
    /** Whether the landing runway / stand shown are the flight's own values. */
    fromFlight: { runway: !edited.runway && landingRunway === flightRunway && Boolean(flightRunway), stand: !edited.stand && arrivalStand === flightStand && Boolean(flightStand) },
    /** What the pilot typed (the input value). */
    clearance: typedClearance,
    /** The route drawn is the standard arrival routing (preliminary), not a clearance the pilot typed. */
    preliminary,
    route,
    /** The assigned release point when it differs from the one in use (only after the pilot typed their own). */
    suggestion: assigned && source === 'pilot' && assigned.point !== normalise(releasePoint) ? { ...assigned, stand: normalise(stand) } : null,
    /** ATC assigned a release point that is not on the taxi map. */
    atcUnknown: assigned?.from === 'atc' && !findReleasePoint(assigned.point) ? assigned.point : null,
    setReleasePoint(value: string) {
      setReleasePointValue(value.toUpperCase());
      setSource(value.trim() ? 'pilot' : null);
    },
    acceptSuggestion() {
      if (!assigned) return;
      setReleasePointValue(assigned.point);
      setSource(assigned.from);
    },
    setLandingRunway(value: string) {
      setLandingRunwayValue(value.toUpperCase());
      setEdited((current) => ({ ...current, runway: true }));
    },
    setArrivalStand(value: string) {
      setArrivalStandValue(value.toUpperCase());
      setEdited((current) => ({ ...current, stand: true }));
    },
    setClearance(value: string) {
      // An empty field returns to the preliminary standard routing (arrivals).
      setTypedClearance(value);
    },
  };
}

export type TaxiClearanceState = ReturnType<typeof useTaxiClearance>;
