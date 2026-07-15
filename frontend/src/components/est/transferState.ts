interface CoordinationTransferState {
  from: string;
  to: string;
  isTagRequest: boolean;
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
