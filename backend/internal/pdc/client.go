package pdc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const HoppieBaseURL = "http://www.hoppie.nl/acars/system/connect.html"

type Client struct {
	baseURL    string
	logon      string
	httpClient *http.Client
}

type Message struct {
	From   string
	To     string
	Type   string
	Packet string
	Raw    string
}

func NewClient(logon string) *Client {
	return &Client{
		baseURL: HoppieBaseURL,
		logon:   logon,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// RandomPollInterval returns a random duration between 25 and 45 seconds
func RandomPollInterval() time.Duration {
	return time.Duration(25+rand.Intn(21)) * time.Second
}

// Poll retrieves pending messages from Hoppie
func (c *Client) Poll(ctx context.Context, callsign string) ([]Message, error) {
	params := url.Values{}
	params.Set("logon", c.logon)
	params.Set("from", callsign)
	params.Set("to", "SERVER")
	params.Set("type", "poll")
	params.Set("packet", "")

	body, err := c.request(ctx, params)
	if err != nil {
		return nil, err
	}

	return c.parseResponse(body)
}

// SendTelex sends a telex message
func (c *Client) SendTelex(ctx context.Context, from, to, packet string) error {
	return c.sendMessage(ctx, from, to, "telex", packet)
}

// SendCPDLC sends a CPDLC message
func (c *Client) SendCPDLC(ctx context.Context, from, to, packet string) error {
	return c.sendMessage(ctx, from, to, "cpdlc", packet)
}

func (c *Client) sendMessage(ctx context.Context, from, to, msgType, packet string) (err error) {
	// Acceptance by Hoppie does not establish receipt by the pilot.
	logger := slog.With("from", from, "to", to, "channel", "CPDLC", "message_type", msgType)
	if parts := strings.SplitN(packet, "/", 6); len(parts) == 6 && parts[1] == "data2" {
		logger = logger.With("message_sequence", parts[2], "response_to", parts[3])
	}
	defer func() {
		if err != nil {
			logger.WarnContext(ctx, "PDC Hoppie: Message send failed", "error", err, "outcome", "failed")
		} else {
			logger.InfoContext(ctx, "PDC Hoppie: Message accepted", "outcome", "accepted")
		}
	}()

	params := url.Values{}
	params.Set("logon", c.logon)
	params.Set("from", from)
	params.Set("to", to)
	params.Set("type", msgType)
	params.Set("packet", packet)

	body, err := c.request(ctx, params)
	if err != nil {
		return err
	}
	if body != "ok" {
		return fmt.Errorf("unexpected Hoppie send response: %s", c.responseSummary(body))
	}
	return nil
}

const maxHoppieResponseBytes = 1024 * 1024

// request validates HTTP status and the provider's application-level status.
// Never expose the request URL: its query includes the secret logon code.
func (c *Client) request(ctx context.Context, params url.Values) (string, error) {
	reqURL := fmt.Sprintf("%s?%s", c.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create Hoppie request: %s", c.responseSummary(err.Error()))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		for {
			var urlErr *url.Error
			if !errors.As(err, &urlErr) {
				break
			}
			err = urlErr.Err
		}
		return "", &hoppieTransportError{cause: err, summary: c.responseSummary(err.Error())}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHoppieResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("failed to read Hoppie response: %s", c.responseSummary(err.Error()))
	}
	if len(body) > maxHoppieResponseBytes {
		return "", fmt.Errorf("Hoppie response exceeds %d bytes", maxHoppieResponseBytes)
	}
	text := strings.TrimSpace(string(body))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Hoppie HTTP status %d: %s", resp.StatusCode, c.responseSummary(text))
	}
	if text != "ok" && (len(text) < 3 || text[:2] != "ok" || !strings.ContainsAny(text[2:3], " \t\r\n")) {
		return "", fmt.Errorf("Hoppie rejected request or returned an invalid response: %s", c.responseSummary(text))
	}
	return text, nil
}

type hoppieTransportError struct {
	cause   error
	summary string
}

func (e *hoppieTransportError) Error() string { return "Hoppie request failed: " + e.summary }
func (e *hoppieTransportError) Unwrap() error { return e.cause }

func (c *Client) responseSummary(body string) string {
	if c.logon != "" {
		body = strings.ReplaceAll(body, c.logon, "[REDACTED]")
		body = strings.ReplaceAll(body, url.QueryEscape(c.logon), "[REDACTED]")
	}
	if len(body) > 512 {
		body = body[:512] + "..."
	}
	return fmt.Sprintf("%q", body)
}

// parseResponse parses the poll response into messages
// Format: "ok {MESSAGE1} {MESSAGE2} ..."
// Each MESSAGE is "{FROM TYPE {CONTENT}}"
func (c *Client) parseResponse(body string) ([]Message, error) {
	var messages []Message

	body = strings.TrimSpace(body)
	if body == "ok" {
		return messages, nil
	}
	if len(body) < 3 || body[:2] != "ok" || !strings.ContainsAny(body[2:3], " \t\r\n") {
		return nil, fmt.Errorf("invalid Hoppie poll response: %s", c.responseSummary(body))
	}
	body = strings.TrimSpace(body[2:])

	// Extract top-level brace-enclosed blocks
	var blocks []string
	start := -1
	depth := 0
	for i, char := range body {
		if depth == 0 && char != '{' && !strings.ContainsRune(" \t\r\n", char) {
			return nil, errors.New("invalid Hoppie poll response: unexpected text outside message")
		}
		switch char {
		case '{':

			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start != -1 {
				blocks = append(blocks, body[start+1:i])
				start = -1
			}
		}
	}
	if depth != 0 {
		return nil, errors.New("invalid Hoppie poll response: incomplete message")
	}

	for _, block := range blocks {
		msg := c.parseMessage(block)
		if msg == nil {
			return nil, errors.New("invalid Hoppie poll response: malformed message")
		}
		messages = append(messages, *msg)
	}

	return messages, nil
}

func (c *Client) parseMessage(content string) *Message {
	// The content is what's inside the outer braces: "NOZ938 telex {REQUEST PREDEP CLEARANCE ...}"

	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	// Find the first occurrence of '{' which marks the start of the packet content
	idx := strings.Index(content, "{")
	if idx == -1 {
		return nil
	}

	header := strings.TrimSpace(content[:idx])
	remaining := content[idx:]

	// Header contains FROM and message type.
	headerParts := strings.Fields(header)
	if len(headerParts) != 2 {
		return nil
	}

	from := headerParts[0]
	to := headerParts[1]

	// The message type is usually the second field in the header or "telex" by default
	// In the new format: "NOZ938 telex {CONTENT}" -> from=NOZ938, to=telex, type=telex
	msgType := to

	// Extract packet content from the first brace block
	var packet string
	depth := 0
	start := -1
	for i, char := range remaining {
		if char == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if char == '}' {
			depth--
			if depth == 0 && start != -1 {
				packet = remaining[start+1 : i]
				if strings.TrimSpace(remaining[i+1:]) != "" {
					return nil
				}
				break
			}
		}
	}

	return &Message{
		From:   from,
		To:     to,
		Type:   msgType,
		Packet: packet,
		Raw:    content,
	}
}
