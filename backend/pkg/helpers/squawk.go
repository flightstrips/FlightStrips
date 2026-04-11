package helpers

import "strings"

var reservedAssignedSquawks = map[string]struct{}{
	"1000": {},
	"1234": {},
	"2000": {},
	"2200": {},
	"7000": {},
}

// IsValidAssignedSquawk reports whether a squawk is suitable for assigned/
// generated departure use. Reserved VFR codes are rejected even though they are
// syntactically valid octal squawks.
func IsValidAssignedSquawk(squawk string) bool {
	squawk = strings.TrimSpace(squawk)
	if len(squawk) != 4 {
		return false
	}
	if _, reserved := reservedAssignedSquawks[squawk]; reserved {
		return false
	}

	for _, digit := range squawk {
		if digit < '0' || digit > '7' {
			return false
		}
	}

	return true
}
