package main

import (
	"fmt"
	"net/http"
	"net/url"
)

// Only explicitly supplied external hosts are redirected. Other live requests
// fail closed; loopback requests (including Testcontainers) pass through.
type fixtureTransport struct {
	upstream  http.RoundTripper
	redirects map[string]string
}

func (t fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Hostname() == "127.0.0.1" || r.URL.Hostname() == "localhost" {
		return t.upstream.RoundTrip(r)
	}
	destination, allowed := t.redirects[r.URL.Hostname()]
	if !allowed {
		return nil, fmt.Errorf("E2E forbids live HTTP request to %s", r.URL.Hostname())
	}
	target, err := url.Parse(destination)
	if err != nil {
		return nil, err
	}
	clone := r.Clone(r.Context())
	endpoint := *r.URL
	clone.URL = &endpoint
	clone.URL.Host, clone.URL.Scheme, clone.Host = target.Host, target.Scheme, target.Host
	return t.upstream.RoundTrip(clone)
}
