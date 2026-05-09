package cdm

import (
	"testing"
	"time"
)

func TestCompareClockForSort_TreatsJustAfterMidnightAsLaterThanJustBeforeMidnight(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, 3, 25, 23, 55, 0, 0, time.UTC)

	if cmp := compareClockForSort("000500", "235800", anchor); cmp <= 0 {
		t.Fatalf("expected 00:05 to sort after 23:58 around midnight, got %d", cmp)
	}
	if cmp := compareClockForSort("235800", "000500", anchor); cmp >= 0 {
		t.Fatalf("expected 23:58 to sort before 00:05 around midnight, got %d", cmp)
	}
}

func TestCompareClockForSort_TreatsLateEveningAsBeforeEarlyMorningWhenAnchorIsAfterMidnight(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, 3, 26, 2, 0, 0, 0, time.UTC)

	if cmp := compareClockForSort("230000", "021500", anchor); cmp >= 0 {
		t.Fatalf("expected 23:00 to sort before 02:15 when the anchor is 02:00, got %d", cmp)
	}
	if cmp := compareClockForSort("021500", "230000", anchor); cmp <= 0 {
		t.Fatalf("expected 02:15 to sort after 23:00 when the anchor is 02:00, got %d", cmp)
	}
}
