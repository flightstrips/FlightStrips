const COMPACT_CDM_TIME_PATTERN = /^\d{1,4}$/;

/**
 * Normalizes compact CDM time strings to four-digit HHMM format.
 *
 * Pads 1-4 digit numeric values with leading zeroes, and otherwise returns the
 * trimmed original value unchanged. ISO timestamps normalize to UTC HHMM. Empty values normalize to an empty string.
 */
export function normalizeCdmTime(value: string | null | undefined): string {
  const trimmedValue = value?.trim() ?? "";

  // HTTP inspection endpoints may supply an explicit ISO instant. Strip
  // clocks always use UTC, independent of the browser timezone.
  if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}.*(?:Z|[+-]\d{2}:\d{2})$/.test(trimmedValue)) {
    const instant = new Date(trimmedValue);
    if (Number.isFinite(instant.getTime())) return instant.toISOString().slice(11, 16).replace(":", "");
  }

  if (!trimmedValue || !COMPACT_CDM_TIME_PATTERN.test(trimmedValue)) {
    return trimmedValue;
  }

  return trimmedValue.padStart(4, "0");
}
