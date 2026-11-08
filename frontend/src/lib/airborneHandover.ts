const FREQUENCY_PATTERN = /^\d{3}\.\d{3}$/;

const AIRBORNE_HANDOVER_LABELS: Record<string, string> = {
  "119.805": "W",
  "118.455": "O",
};

// These already resolve their own identifier, so they must not fall back to DK.
const DEFAULT_LABEL_FREQUENCIES = new Set(["124.980", "120.255", "120.205"]);

/** Identifier for an airborne handover frequency, or undefined to keep the default label. */
export function airborneHandoverLabel(frequency?: string): string | undefined {
  const value = frequency?.trim();
  if (!value || !FREQUENCY_PATTERN.test(value)) return undefined;
  if (value in AIRBORNE_HANDOVER_LABELS) return AIRBORNE_HANDOVER_LABELS[value];
  if (DEFAULT_LABEL_FREQUENCIES.has(value)) return undefined;
  return "DK";
}
