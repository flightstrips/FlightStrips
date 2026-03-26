package cdm

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const dayMinutes = 24 * 60

func toHHMMSS(hhmm string) string {
	value := strings.TrimSpace(hhmm)
	switch len(value) {
	case 4:
		return value + "00"
	case 6:
		return value
	default:
		return value
	}
}

func addMinutes(hhmmss string, minutes float64) string {
	return minuteOffset(hhmmss, minutes)
}

func subtractMinutes(hhmmss string, minutes float64) string {
	return minuteOffset(hhmmss, -minutes)
}

func minuteOffset(hhmmss string, minutes float64) string {
	base, ok := parseClock(hhmmss)
	if !ok {
		return hhmmss
	}
	seconds := int(math.Round(minutes * 60))
	result := (base + seconds) % (dayMinutes * 60)
	if result < 0 {
		result += dayMinutes * 60
	}
	return formatClock(result)
}

func isAfterOrEqual(a, b string) bool {
	return minutesBetween(b, a) >= 0
}

func minutesBetween(from, to string) float64 {
	fromSec, okFrom := parseClock(from)
	toSec, okTo := parseClock(to)
	if !okFrom || !okTo {
		return 0
	}
	diff := float64(toSec-fromSec) / 60.0
	if diff <= -720 {
		diff += 1440
	}
	if diff > 720 {
		diff -= 1440
	}
	return diff
}

func isMoreThanMinutesPast(value, reference string, threshold float64) bool {
	return minutesBetween(value, reference) > threshold
}

func timeToClock(now time.Time) string {
	return now.UTC().Format("150405")
}

func normalizeCalculationClock(value string) string {
	value = strings.TrimSpace(value)
	if toHHMMSS(value) == "000000" {
		return ""
	}
	return value
}

func parseClock(value string) (int, bool) {
	value = toHHMMSS(value)
	if len(value) != 6 {
		return 0, false
	}
	hh, err := strconv.Atoi(value[0:2])
	if err != nil {
		return 0, false
	}
	mm, err := strconv.Atoi(value[2:4])
	if err != nil {
		return 0, false
	}
	ss, err := strconv.Atoi(value[4:6])
	if err != nil {
		return 0, false
	}
	return hh*3600 + mm*60 + ss, true
}

func formatClock(totalSeconds int) string {
	hh := totalSeconds / 3600
	mm := (totalSeconds % 3600) / 60
	ss := totalSeconds % 60
	return fmt.Sprintf("%02d%02d%02d", hh, mm, ss)
}
