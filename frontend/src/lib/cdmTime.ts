const COMPACT_CDM_TIME_PATTERN = /^\d{1,4}$/;

/**
 * Normalizes compact CDM time strings to four-digit HHMM format.
 *
 * Pads 1-4 digit numeric values with leading zeroes, and otherwise returns the
 * trimmed original value unchanged. Empty values normalize to an empty string.
 */
export function normalizeCdmTime(value: string | null | undefined): string {
  const trimmedValue = value?.trim() ?? "";

  if (!trimmedValue || !COMPACT_CDM_TIME_PATTERN.test(trimmedValue)) {
    return trimmedValue;
  }

  return trimmedValue.padStart(4, "0");
}
