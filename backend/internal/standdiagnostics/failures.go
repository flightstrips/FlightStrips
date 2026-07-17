package standdiagnostics

import (
	"slices"
	"sync"
	"time"
)

// AllocationFailure is one failed attempt to allocate or reallocate a stand.
// It is operational diagnostic state, not part of the authoritative assignment
// model, and is intentionally retained only in memory.
type AllocationFailure struct {
	ID             uint64    `json:"id"`
	OccurredAt     time.Time `json:"occurred_at"`
	SessionID      int32     `json:"session_id"`
	Airport        string    `json:"airport"`
	Callsign       string    `json:"callsign"`
	Command        string    `json:"command"`
	Outcome        string    `json:"outcome"`
	Reason         string    `json:"reason"`
	Direction      string    `json:"direction"`
	Stage          string    `json:"stage"`
	AttemptedStand string    `json:"attempted_stand,omitempty"`
	AircraftType   string    `json:"aircraft_type,omitempty"`
	EngineType     string    `json:"engine_type,omitempty"`
	WTC            string    `json:"wtc,omitempty"`
	BorderStatus   string    `json:"border_status,omitempty"`
	Attempts       int       `json:"attempts"`
}

// AllocationFailureLog retains the newest failures up to a fixed capacity.
type AllocationFailureLog struct {
	mu      sync.RWMutex
	limit   int
	nextID  uint64
	entries []AllocationFailure
}

func NewAllocationFailureLog(limit int) *AllocationFailureLog {
	if limit <= 0 {
		limit = 100
	}
	return &AllocationFailureLog{limit: limit}
}

func (l *AllocationFailureLog) Record(failure AllocationFailure) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.nextID++
	failure.ID = l.nextID
	l.entries = append(l.entries, failure)
	if overflow := len(l.entries) - l.limit; overflow > 0 {
		copy(l.entries, l.entries[overflow:])
		l.entries = l.entries[:l.limit]
	}
}

// List returns a copy ordered newest first.
func (l *AllocationFailureLog) List() []AllocationFailure {
	if l == nil {
		return []AllocationFailure{}
	}
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := slices.Clone(l.entries)
	slices.Reverse(result)
	return result
}
