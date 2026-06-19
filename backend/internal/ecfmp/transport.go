package ecfmp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type httpTransport struct {
	client *http.Client
}

func (t *httpTransport) Do(ctx context.Context, method string, path string, query map[string]string) ([]byte, error) {
	u, err := url.Parse(path)
	if err != nil {
		return nil, err
	}

	if len(query) > 0 {
		q := u.Query()
		for k, v := range query {
			if v != "" {
				q.Set(k, v)
			}
		}
		u.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ecfmp api error (%d): %s", resp.StatusCode, string(data))
	}

	return data, nil
}