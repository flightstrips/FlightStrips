package pdc

import "context"

// NoopHoppie implements HoppieClientInterface when HOPPIE_LOGON is unset (web PDC only).
type NoopHoppie struct{}

func (NoopHoppie) Poll(_ context.Context, _ string) ([]Message, error) {
	return nil, nil
}

func (NoopHoppie) SendCPDLC(_ context.Context, _, _, _ string) error {
	return nil
}

func (NoopHoppie) SendTelex(_ context.Context, _, _, _ string) error {
	return nil
}
