package pdc

import (
	"context"
	"fmt"
	"io"
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

	reqURL := fmt.Sprintf("%s?%s", c.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create poll request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to poll Hoppie: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read poll response: %w", err)
	}

	return c.parseResponse(string(body)), nil
}

// SendTelex sends a telex message
func (c *Client) SendTelex(ctx context.Context, from, to, packet string) error {
	return c.sendMessage(ctx, from, to, "telex", packet)
}

// SendCPDLC sends a CPDLC message
func (c *Client) SendCPDLC(ctx context.Context, from, to, packet string) error {
	return c.sendMessage(ctx, from, to, "cpdlc", packet)
}

func (c *Client) sendMessage(ctx context.Context, from, to, msgType, packet string) error {
	params := url.Values{}
	params.Set("logon", c.logon)
	params.Set("from", from)
	params.Set("to", to)
	params.Set("type", msgType)
	params.Set("packet", packet)

	reqURL := fmt.Sprintf("%s?%s", c.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create send request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Hoppie returned error: %s", string(body))
	}

	return nil
}

// parseResponse parses the poll response into messages
// Format: "ok {MESSAGE1} {MESSAGE2} ..."
// Each MESSAGE is "{FROM TYPE {CONTENT}}"
func (c *Client) parseResponse(body string) []Message {
	var messages []Message

	body = strings.TrimSpace(body)
	if body == "ok" || body == "" {
		return messages
	}

	// Remove "ok " prefix if present
	if strings.HasPrefix(body, "ok ") {
		body = strings.TrimPrefix(body, "ok ")
	}

	// Extract top-level brace-enclosed blocks
	var blocks []string
	start := -1
	depth := 0
	for i, char := range body {
		if char == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if char == '}' {
			depth--
			if depth == 0 && start != -1 {
				blocks = append(blocks, body[start+1:i])
				start = -1
			}
		}
	}

	for _, block := range blocks {
		msg := c.parseMessage(block)
		if msg != nil {
			messages = append(messages, *msg)
		}
	}

	return messages
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

	// Header should contain FROM and TO
	headerParts := strings.Fields(header)
	if len(headerParts) < 2 {
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
