export interface StandAssignment {
  airport: string;
  stand: string;
  standName: string;
  coord1: string;
  coord2: string;
  radius: number;
  engineType?: string;
  wtc?: string[];
  use?: string;
  wingspan?: number;
  priority?: number;
  schengen?: boolean;
  nonSchengen?: boolean;
  blocks?: string[];
  raw: Record<string, string>;
}

export interface AircraftStandMatchInput {
  airport: string;
  wingspan: number;
  use?: string;
  schengen?: boolean;
  wtc?: string;
  engineType?: string;
  latitude?: number;
  longitude?: number;
}

function parseCoordinate(value: string): number {
  const clean = value.trim().replace(/[<>]/g, "");
  if (!clean) {
    return 0;
  }

  const match = clean.match(/^([NSEW])(\d{3})\.(\d{2})\.(\d{2})\.(\d+)$/);
  if (!match) {
    return 0;
  }

  const direction = match[1];
  const degrees = Number.parseFloat(match[2]);
  const minutes = Number.parseFloat(match[3]);
  const seconds = Number.parseFloat(match[4]);
  const decimals = Number.parseFloat(`0.${match[5]}`);
  const decimal = degrees + minutes / 60 + (seconds + decimals) / 3600;

  return direction === "S" || direction === "W" ? -decimal : decimal;
}

function haversineDistanceMeters(
  lat1: number,
  lon1: number,
  lat2: number,
  lon2: number,
): number {
  const toRadians = (deg: number) => (deg * Math.PI) / 180;
  const earthRadiusMeters = 6_371_000;

  const dLat = toRadians(lat2 - lat1);
  const dLon = toRadians(lon2 - lon1);
  const a =
    Math.sin(dLat / 2) * Math.sin(dLat / 2) +
    Math.cos(toRadians(lat1)) *
      Math.cos(toRadians(lat2)) *
      Math.sin(dLon / 2) *
      Math.sin(dLon / 2);

  return 2 * earthRadiusMeters * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
}

export function parseStandAssignmentFile(rawText: string): StandAssignment[] {
  const stands: StandAssignment[] = [];
  let currentStand: StandAssignment | null = null;

  const flushCurrent = () => {
    if (currentStand) {
      stands.push(currentStand);
      currentStand = null;
    }
  };

  for (const rawLine of rawText.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line) {
      continue;
    }

    if (line.startsWith("STAND:") || line.startsWith("AIRPORT:")) {
      flushCurrent();

      const segments = line.split(":");
      const tag = segments[0].toUpperCase();
      if (tag !== "STAND" && tag !== "AIRPORT") {
        continue;
      }

      const airport = segments[1] ?? "";
      const stand = segments[2] ?? "";
      const coord1 = segments[3] ?? "";
      const coord2 = segments[4] ?? "";
      const radius = Number.parseFloat(segments[5] ?? "0") || 0;

      currentStand = {
        airport: airport.toUpperCase(),
        stand,
        standName: stand,
        coord1,
        coord2,
        radius,
        raw: {},
      };
      continue;
    }

    if (!currentStand) {
      continue;
    }

    const [key, ...rest] = line.split(":");
    const value = rest.join(":").trim();
    const upperKey = key.trim().toUpperCase();

    switch (upperKey) {
      case "ENGINETYPE":
        currentStand.engineType = value;
        currentStand.raw.engineType = value;
        break;
      case "WTC":
        currentStand.wtc = value.split("").filter(Boolean);
        currentStand.raw.wtc = value;
        break;
      case "USE":
        currentStand.use = value;
        currentStand.raw.use = value;
        break;
      case "WINGSPAN":
        currentStand.wingspan = Number.parseFloat(value);
        currentStand.raw.wingspan = value;
        break;
      case "PRIORITY":
        currentStand.priority = Number.parseInt(value, 10);
        currentStand.raw.priority = value;
        break;
      case "BLOCKS":
        currentStand.blocks = value
          .split(",")
          .map((part) => part.trim())
          .filter(Boolean);
        currentStand.raw.blocks = value;
        break;
      case "SCHENGEN":
        currentStand.schengen = true;
        currentStand.nonSchengen = false;
        currentStand.raw.schengen = "true";
        break;
      case "NON-SCHENGEN":
        currentStand.schengen = false;
        currentStand.nonSchengen = true;
        currentStand.raw.schengen = "false";
        break;
      default:
        currentStand.raw[upperKey] = value;
        break;
    }
  }

  flushCurrent();
  return stands;
}

export function isStandBlockedByRadius(
  stand: Pick<StandAssignment, "coord1" | "coord2" | "radius">,
  aircraftLatitude: number,
  aircraftLongitude: number,
): boolean {
  if (!stand.coord1 || !stand.coord2 || stand.radius <= 0) {
    return false;
  }

  const standLat = parseCoordinate(stand.coord1);
  const standLon = parseCoordinate(stand.coord2);
  const distanceMeters = haversineDistanceMeters(
    aircraftLatitude,
    aircraftLongitude,
    standLat,
    standLon,
  );

  return distanceMeters <= stand.radius;
}

export function findStandAtPosition(
  stands: StandAssignment[],
  airport: string,
  aircraftLatitude: number,
  aircraftLongitude: number,
): StandAssignment | undefined {
  return stands
    .filter(
      (stand) =>
        stand.airport === airport &&
        isStandBlockedByRadius(stand, aircraftLatitude, aircraftLongitude),
    )
    .sort((left, right) => {
      const leftDistance = haversineDistanceMeters(
        aircraftLatitude,
        aircraftLongitude,
        parseCoordinate(left.coord1),
        parseCoordinate(left.coord2),
      );
      const rightDistance = haversineDistanceMeters(
        aircraftLatitude,
        aircraftLongitude,
        parseCoordinate(right.coord1),
        parseCoordinate(right.coord2),
      );
      return leftDistance - rightDistance;
    })[0];
}

export function getStandCandidates(
  stands: StandAssignment[],
  aircraft: AircraftStandMatchInput,
): StandAssignment[] {
  return stands
    .filter((stand) => {
      if (
        stand.airport &&
        aircraft.airport &&
        stand.airport !== aircraft.airport
      ) {
        return false;
      }

      if (
        typeof stand.wingspan === "number" &&
        aircraft.wingspan > stand.wingspan
      ) {
        return false;
      }

      if (stand.use && aircraft.use) {
        const allowed = stand.use.toUpperCase();
        const aircraftUse = aircraft.use.toUpperCase();
        const matchesUse = [...aircraftUse].some((char) =>
          allowed.includes(char),
        );
        if (!matchesUse) {
          return false;
        }
      }

      if (typeof aircraft.schengen === "boolean") {
        if (
          stand.schengen !== undefined &&
          stand.schengen !== aircraft.schengen
        ) {
          return false;
        }
        if (
          stand.nonSchengen !== undefined &&
          stand.nonSchengen !== !aircraft.schengen
        ) {
          return false;
        }
      }

      if (aircraft.wtc && stand.wtc && stand.wtc.length > 0) {
        const aircraftWtc = aircraft.wtc.toUpperCase();
        const matchesWtc = stand.wtc.some((entry) =>
          aircraftWtc.includes(entry.toUpperCase()),
        );
        if (!matchesWtc) {
          return false;
        }
      }

      if (aircraft.engineType && stand.engineType) {
        const standEngines = stand.engineType.toUpperCase();
        const aircraftEngine = aircraft.engineType.toUpperCase();
        if (
          !standEngines.includes(aircraftEngine) &&
          !aircraftEngine.includes(standEngines)
        ) {
          return false;
        }
      }

      return true;
    })
    .sort((a, b) => (b.priority ?? 0) - (a.priority ?? 0));
}

export function selectBestStandForAircraft(
  stands: StandAssignment[],
  aircraft: AircraftStandMatchInput,
  blockedStandNumbers: string[] = [],
): StandAssignment | undefined {
  const candidates = getStandCandidates(stands, aircraft).filter(
    (stand) => !blockedStandNumbers.includes(stand.stand),
  );

  return candidates[0];
}
