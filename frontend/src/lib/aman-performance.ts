export const AMAN_STATE_RECEIVED_MARK = "aman-state-received";
export const AMAN_STATE_PAINTED_MARK = "aman-state-painted";
export const AMAN_STATE_TO_PAINT_MEASURE = "aman-state-to-paint";

function markName(prefix: string, revision: number): string {
  return `${prefix}-${revision}`;
}

export function markAMANStateReceived(revision: number): void {
  if (typeof performance?.mark !== "function") return;
  const name = markName(AMAN_STATE_RECEIVED_MARK, revision);
  performance.clearMarks?.(name);
  performance.mark(name);
}

/** Measures the complete replacement after the browser has had a paint frame. */
export function measureAMANStatePaint(revision: number): () => void {
  if (typeof requestAnimationFrame !== "function" || typeof performance?.mark !== "function" || typeof performance?.measure !== "function") {
    return () => undefined;
  }

  let cancelled = false;
  const frame = requestAnimationFrame(() => {
    if (cancelled) return;
    const received = markName(AMAN_STATE_RECEIVED_MARK, revision);
    if (performance.getEntriesByName(received, "mark").length === 0) return;
    const painted = markName(AMAN_STATE_PAINTED_MARK, revision);
    performance.mark(painted);
    performance.measure(markName(AMAN_STATE_TO_PAINT_MEASURE, revision), received, painted);
  });
  return () => {
    cancelled = true;
    cancelAnimationFrame(frame);
  };
}

export function percentile95(samples: number[]): number | null {
  if (samples.length === 0) return null;
  const ordered = [...samples].sort((left, right) => left - right);
  return ordered[Math.ceil(ordered.length * 0.95) - 1];
}

export function readAMANPaintP95(): number | null {
  if (typeof performance?.getEntriesByType !== "function") return null;
  const samples = performance.getEntriesByType("measure")
    .filter((entry) => entry.name.startsWith(`${AMAN_STATE_TO_PAINT_MEASURE}-`))
    .map((entry) => entry.duration);
  return percentile95(samples);
}
