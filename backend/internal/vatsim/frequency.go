package vatsim

import (
	"fmt"
	"strconv"
	"strings"
)

// NormalizeFrequency converts both decimal frequencies (118.105) and VATSIM
// transceiver feed frequencies in Hz (118105000) into a comparable 3-decimal
// MHz string.
func NormalizeFrequency(frequency string) string {
	normalized := strings.TrimSpace(frequency)
	if normalized == "" {
		return ""
	}

	if strings.Contains(normalized, ".") {
		parts := strings.SplitN(normalized, ".", 2)
		whole, err := strconv.Atoi(parts[0])
		if err != nil || whole <= 0 {
			return ""
		}

		fraction := strings.TrimSpace(parts[1])
		for len(fraction) < 3 {
			fraction += "0"
		}
		if len(fraction) > 3 {
			fraction = fraction[:3]
		}

		return fmt.Sprintf("%d.%s", whole, fraction)
	}

	hz, err := strconv.ParseInt(normalized, 10, 64)
	if err != nil || hz <= 0 {
		return ""
	}

	return formatFrequencyHz(hz)
}

func formatFrequencyHz(frequencyHz int64) string {
	mhz := frequencyHz / 1_000_000
	khz := (frequencyHz % 1_000_000) / 1_000
	return fmt.Sprintf("%d.%03d", mhz, khz)
}
