package replay

import "fmt"

// ReplayError represents errors that occur during replay
type ReplayError struct {
	Message    string
	EventIndex int
	EventType  string
	Err        error
}

func (e *ReplayError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("replay error at event %d (%s): %s: %v", e.EventIndex, e.EventType, e.Message, e.Err)
	}
	return fmt.Sprintf("replay error at event %d (%s): %s", e.EventIndex, e.EventType, e.Message)
}

func (e *ReplayError) Unwrap() error {
	return e.Err
}

// ErrInvalidConfig is returned when replay config is invalid
type ErrInvalidConfig string

func (e ErrInvalidConfig) Error() string {
	return fmt.Sprintf("invalid replay config: %s", string(e))
}

// ErrConnectionFailed is returned when WebSocket connection fails
type ErrConnectionFailed struct {
	URL string
	Err error
}

func (e *ErrConnectionFailed) Error() string {
	return fmt.Sprintf("failed to connect to %s: %v", e.URL, e.Err)
}

func (e *ErrConnectionFailed) Unwrap() error {
	return e.Err
}
