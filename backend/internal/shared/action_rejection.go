package shared

// ActionRejection marks an expected policy or validation rejection. Transports
// still return the reason to the client, but should not report a server failure.
type ActionRejection struct {
	Reason string
}

func (e *ActionRejection) Error() string { return e.Reason }

func RejectAction(reason string) error { return &ActionRejection{Reason: reason} }
