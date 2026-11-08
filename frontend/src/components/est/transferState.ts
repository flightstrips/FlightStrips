interface CoordinationTransferState {
  from: string;
  to: string;
  isTagRequest: boolean;
}

export function getEstDepartureTransferTarget(
  strip: { next_controllers: string[] } | undefined,
  sourcePosition: string,
): string {
  return strip?.next_controllers.find((position) => position !== sourcePosition) ?? "";
}

export function isEstDepartureTransferActive(
  transfer: CoordinationTransferState | undefined,
  sourcePosition: string,
  targetPosition: string,
): boolean {
  return Boolean(
    transfer &&
      !transfer.isTagRequest &&
      sourcePosition &&
      targetPosition &&
      transfer.from === sourcePosition &&
      transfer.to === targetPosition,
  );
}

export function isEstDepartureTransferred(
  strip: { owner: string; previous_controllers: string[] | null } | undefined,
  sourcePosition: string,
): boolean {
  return Boolean(
    strip &&
      sourcePosition &&
      strip.owner &&
      strip.owner !== sourcePosition &&
      strip.previous_controllers?.includes(sourcePosition),
  );
}
