package pdc

import "context"

type NoopHoppieClient struct{}

func (NoopHoppieClient) Poll(_ context.Context, _ string) ([]Message, error) {
	return nil, nil
}

func (NoopHoppieClient) SendCPDLC(_ context.Context, _ string, _ string, _ string) error {
	return nil
}

func (NoopHoppieClient) SendTelex(_ context.Context, _ string, _ string, _ string) error {
	return nil
}
