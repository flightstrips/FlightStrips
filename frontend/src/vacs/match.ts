import type { FrontendController } from "@/api/models";
import type { ClientInfo } from "./types";

export function findVacsClientForController(
  controller: FrontendController,
  clients: ClientInfo[],
): ClientInfo | undefined {
  return clients.find(
    (c) =>
      c.positionId === controller.position ||
      c.positionId === controller.callsign ||
      c.displayName === controller.callsign ||
      c.displayName === controller.position,
  );
}

export function isSelfOnVacs(
  client: ClientInfo,
  ownClientId: string | null,
  ownPositionId: string,
  myPosition: string,
  myCallsign: string,
): boolean {
  if (ownClientId && client.id === ownClientId) {
    return true;
  }
  if (ownPositionId && client.positionId === ownPositionId) {
    return true;
  }
  if (client.positionId === myPosition || client.displayName === myCallsign) {
    return true;
  }
  return false;
}
