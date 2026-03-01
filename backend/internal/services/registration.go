package services

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"unicode"
)

// remarksRegPattern matches the REG/ token in ICAO remarks.
// Example: "PBN/A1B1 DOF/260301 REG/N320SB EET/EKDK0027"
var remarksRegPattern = regexp.MustCompile(`\bREG/([A-Z0-9-]+)`)

// ParseRegistration derives an aircraft registration from the available data.
// Priority:
//  1. REG/ token in remarks
//  2. Callsign if it is exactly 5 letters (no digits)
//  3. Random placeholder
func ParseRegistration(callsign, remarks string) string {
	// 1. Try remarks
	if remarks != "" {
		if m := remarksRegPattern.FindStringSubmatch(strings.ToUpper(remarks)); len(m) == 2 {
			return m[1]
		}
	}

	// 2. Try callsign — exactly 5 characters, all letters
	cs := strings.ToUpper(strings.TrimSpace(callsign))
	if len(cs) == 5 && isAllLetters(cs) {
		return cs
	}

	// 3. Random placeholder (two letters + three digits, e.g. "OY123")
	// TODO: replace with a lookup from a real registration file in a future task.
	return randomRegistration()
}

func isAllLetters(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func randomRegistration() string {
	letters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits := "0123456789"
	return fmt.Sprintf("%c%c%c%c%c",
		letters[rand.Intn(len(letters))],
		letters[rand.Intn(len(letters))],
		digits[rand.Intn(len(digits))],
		digits[rand.Intn(len(digits))],
		digits[rand.Intn(len(digits))],
	)
}
