package ecfmp

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

const DefaultBaseURL = "https://ecfmp.vatsim.net/api/v1"

const defaultCacheTTL = 60 * time.Second

type Client struct {
	baseURL    string
	httpClient httpClient

	cacheMu      sync.Mutex
	cacheTTL     time.Duration
	cacheUntil   time.Time
	cacheEntries []FlowMeasure
}

type httpClient interface {
	Do(ctx context.Context, method string, path string, query map[string]string) ([]byte, error)
}

type Option func(*Client)

func WithCacheTTL(ttl time.Duration) Option {
	return func(c *Client) {
		c.cacheTTL = ttl
	}
}

func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = url
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = &httpTransport{client: client}
	}
}

func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL:    DefaultBaseURL,
		httpClient: &httpTransport{client: &http.Client{Timeout: 15 * time.Second}},
		cacheTTL:   defaultCacheTTL,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Client) FlowMeasures(ctx context.Context) ([]FlowMeasure, error) {
	now := time.Now()
	if cached, ok := c.fromCache(now); ok {
		return cached, nil
	}

	data, err := c.httpClient.Do(ctx, "GET", c.baseURL+"/flow-measure", map[string]string{"active": "1"})
	if err != nil {
		return nil, err
	}

	var measures []FlowMeasure
	if err := json.Unmarshal(data, &measures); err != nil {
		return nil, err
	}

	c.storeCache(now, measures)
	return append([]FlowMeasure(nil), measures...), nil
}

func (c *Client) fromCache(now time.Time) ([]FlowMeasure, bool) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	if c.cacheTTL <= 0 || now.After(c.cacheUntil) {
		return nil, false
	}

	return append([]FlowMeasure(nil), c.cacheEntries...), true
}

func (c *Client) storeCache(now time.Time, measures []FlowMeasure) {
	if c.cacheTTL <= 0 {
		return
	}

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	c.cacheEntries = append([]FlowMeasure(nil), measures...)
	c.cacheUntil = now.Add(c.cacheTTL)
}

func (c *Client) SetTestMeasures(measures []FlowMeasure) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	c.cacheEntries = append([]FlowMeasure(nil), measures...)
	c.cacheUntil = time.Now().Add(365 * 24 * time.Hour)
}