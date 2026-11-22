// Fixed release points for stands in areas without published release points. Pushbacks there are
// not reliably logged by ATC, so these defaults apply whatever ATC has set on the strip.
const STAND_RELEASE_POINTS: Array<{ prefix: string; from: number; to: number; releasePoint: string }> = [
  { prefix: 'G', from: 110, to: 115, releasePoint: 'I' },
  { prefix: 'G', from: 117, to: 124, releasePoint: 'G6' },
  { prefix: 'G', from: 125, to: 137, releasePoint: 'G' },
  { prefix: 'G', from: 15, to: 19, releasePoint: 'G' },
];

/** The fixed release point for a stand, or null when the stand has none. */
export function standReleasePoint(stand: string | null | undefined): string | null {
  const match = (stand ?? '').trim().toUpperCase().match(/^([A-Z]+)(\d+)$/);
  if (!match) return null;
  const [, prefix, digits] = match, number = Number(digits);
  return STAND_RELEASE_POINTS.find((rule) => rule.prefix === prefix && number >= rule.from && number <= rule.to)?.releasePoint ?? null;
}
