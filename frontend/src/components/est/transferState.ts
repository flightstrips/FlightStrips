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
