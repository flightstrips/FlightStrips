// Package connectionprobe compares the public route with the direct origin.
// Both use the public hostname for HTTP Host, TLS SNI and certificate validation.
package connectionprobe

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

type Probe struct {
	base    *url.URL
	origin  string
	timeout time.Duration
	// Set only by tests that use a private certificate authority. Production
	// always uses the system trust store and verifies the public hostname.
	tlsConfig *tls.Config
}

type Result struct {
	Target       string        `json:"target"`
	Check        string        `json:"check"`
	Success      bool          `json:"success"`
	Status       int           `json:"status"`
	CFRay        string        `json:"cf_ray,omitempty"`
	ConnectionID string        `json:"connection_id,omitempty"`
	Error        string        `json:"error,omitempty"`
	Duration     time.Duration `json:"-"`
	CheckedAt    time.Time     `json:"checked_at"`
}

func New(publicURL, originAddress string, timeout time.Duration) (*Probe, error) {
	base, err := url.Parse(publicURL)
	if err != nil || base == nil || base.Hostname() == "" || base.Scheme != "https" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") {
		return nil, fmt.Errorf("public URL must be an HTTPS origin without credentials, path or query")
	}
	if _, port, err := net.SplitHostPort(originAddress); err != nil || port == "" {
		return nil, fmt.Errorf("origin address must be host:port")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("timeout must be positive")
	}
	return &Probe{base: base, origin: originAddress, timeout: timeout}, nil
}

func (p *Probe) Check(ctx context.Context, target, check string) (result Result) {
	started := time.Now()
	result = Result{Target: target, Check: check, CheckedAt: started}
	defer func() { result.Duration = time.Since(started) }()
	if (target != "public" && target != "origin") || (check != "http" && check != "websocket") {
		result.Error = "invalid probe target or check"
		return
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	dial := (&net.Dialer{Timeout: p.timeout}).DialContext
	if target == "origin" {
		dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: p.timeout}).DialContext(ctx, network, p.origin)
		}
	}
	endpoint := *p.base
	headers := http.Header{"User-Agent": {"FlightStrips-Connection-Probe/1"}}
	var response *http.Response
	var err error
	if check == "http" {
		endpoint.Path = "/healthz"
		transport := &http.Transport{DialContext: dial, TLSClientConfig: p.tlsConfig, TLSHandshakeTimeout: p.timeout}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: p.timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		request.Header = headers
		response, err = client.Do(request)
	} else {
		endpoint.Scheme, endpoint.Path = "wss", "/frontEndEvents"
		dialer := websocket.Dialer{NetDialContext: dial, TLSClientConfig: p.tlsConfig, HandshakeTimeout: p.timeout}
		var conn *websocket.Conn
		conn, response, err = dialer.DialContext(ctx, endpoint.String(), headers)
		if conn != nil {
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "network probe"), time.Now().Add(time.Second))
			_ = conn.Close()
		}
	}
	if response != nil {
		result.Status = response.StatusCode
		result.CFRay = response.Header.Get("CF-Ray")
		result.ConnectionID = response.Header.Get("X-Flightstrips-Connection-Id")
		if response.Body != nil {
			_ = response.Body.Close()
		}
	}
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = check == "http" && result.Status == http.StatusOK || check == "websocket" && result.Status == http.StatusSwitchingProtocols
	if !result.Success {
		result.Error = fmt.Sprintf("unexpected HTTP status %d", result.Status)
	}
	return
}
