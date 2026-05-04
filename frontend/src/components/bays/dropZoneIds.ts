const BAY_CONTAINER_DROP_ZONE_PREFIX = "bay-container:";

export function makeBayContainerDropZoneId(bayId: string) {
  return `${BAY_CONTAINER_DROP_ZONE_PREFIX}${bayId}`;
}

export function parseBayContainerDropZoneId(id: string) {
  return id.startsWith(BAY_CONTAINER_DROP_ZONE_PREFIX) ? id.slice(BAY_CONTAINER_DROP_ZONE_PREFIX.length) : null;
}
