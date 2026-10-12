import type { FrontendController } from "@/api/models";

export interface ControllerStation {
  frequency: string;
  sectors: string[];
}

const normalizeSector = (sector: string) => sector === "GWA" || sector === "GWD" ? "GW" : sector;

export function findStationController(
  station: ControllerStation,
  controllers: FrontendController[],
  ownPosition: string,
  ownCallsign: string,
): FrontendController | undefined {
  const candidates = controllers.filter(controller =>
    controller.observer !== true &&
    !controller.callsign.toUpperCase().endsWith("_OBS") &&
    controller.position !== ownPosition &&
    controller.callsign.toUpperCase() !== ownCallsign.toUpperCase(),
  );
  return candidates.find(controller => controller.position === station.frequency) ??
    candidates.find(controller => (controller.owned_sectors ?? []).some(sector => station.sectors.includes(normalizeSector(sector))));
}
