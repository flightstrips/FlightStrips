export function effectiveStripMarked(serverMarked: boolean | undefined, localMarked: boolean | undefined): boolean {
  return localMarked ?? serverMarked ?? false;
}

export function toggleEffectiveStripMark(
  callsign: string,
  serverMarked: boolean | undefined,
  localMarked: boolean | undefined,
  setLocalMarked: (callsign: string, marked: boolean) => void,
  toggleMarked: (callsign: string, marked: boolean) => void,
): void {
  const nextMarked = !effectiveStripMarked(serverMarked, localMarked);
  if (localMarked !== undefined) {
    setLocalMarked(callsign, nextMarked);
  } else {
    toggleMarked(callsign, nextMarked);
  }
}
