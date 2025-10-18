package events

type OutgoingMessage interface {
	Marshal() ([]byte, error)
}
