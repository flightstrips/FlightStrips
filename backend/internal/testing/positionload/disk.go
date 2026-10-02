package positionload

import "fmt"

type DiskGate struct {
	UsedBytes, TotalBytes uint64
	UsedPercent           float64
	Alert, ReleaseBlocked bool
}

// EvaluateDisk applies the operations contract to a real filesystem sample.
// The threshold rehearsal supplies synthetic counters rather than filling a
// user's filesystem. Missing/invalid capacity cannot approve a release.
func EvaluateDisk(used, total uint64) (DiskGate, error) {
	gate := DiskGate{UsedBytes: used, TotalBytes: total, ReleaseBlocked: true}
	if total == 0 || used > total {
		return gate, fmt.Errorf("invalid disk sample")
	}
	gate.UsedPercent = 100 * float64(used) / float64(total)
	gate.Alert = gate.UsedPercent >= 70
	gate.ReleaseBlocked = gate.UsedPercent >= 85
	return gate, nil
}
