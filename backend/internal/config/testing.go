package config

// SetPositionsForTest replaces the package-level positions slice for testing.
// Returns a cleanup function that restores the original value.
func SetPositionsForTest(ps []Position) func() {
	old := positions
	positions = ps
	return func() { positions = old }
}

// SetAirborneOwnersForTest replaces the package-level airborneOwners slice for testing.
// Returns a cleanup function that restores the original value.
func SetAirborneOwnersForTest(owners []string) func() {
	old := airborneOwners
	airborneOwners = owners
	return func() { airborneOwners = old }
}

// SetMissedApproachHandoverForTest replaces the package-level missedApproachHandover map for testing.
// Returns a cleanup function that restores the original value.
func SetMissedApproachHandoverForTest(m map[string]string) func() {
	old := missedApproachHandover
	missedApproachHandover = m
	return func() { missedApproachHandover = old }
}
