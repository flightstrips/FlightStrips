package cdm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const DefaultBaseURL = "https://viff-system.network"

const defaultAirportMasterCacheTTL = 5 * time.Second

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	isValid    bool

	airportMasterCacheMu      sync.Mutex
	airportMasterCacheTTL     time.Duration
	airportMasterCacheUntil   time.Time
	airportMasterCacheEntries []AirportMaster
}

type Option func(*Client)

type AirportMaster struct {
	ICAO     string `json:"icao"`
	Position string `json:"position"`
}

func WithAPIKey(key string) Option {
	return func(c *Client) {
		c.apiKey = key
		c.isValid = key != ""
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = url
	}
}

func WithAirportMasterCacheTTL(ttl time.Duration) Option {
	return func(c *Client) {
		c.airportMasterCacheTTL = ttl
	}
}

func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		airportMasterCacheTTL: defaultAirportMasterCacheTTL,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Client) airportMastersFromCache(now time.Time) ([]AirportMaster, bool) {
	c.airportMasterCacheMu.Lock()
	defer c.airportMasterCacheMu.Unlock()

	if c.airportMasterCacheTTL <= 0 || now.After(c.airportMasterCacheUntil) {
		return nil, false
	}

	return append([]AirportMaster(nil), c.airportMasterCacheEntries...), true
}

func (c *Client) storeAirportMasters(now time.Time, masters []AirportMaster) {
	if c.airportMasterCacheTTL <= 0 {
		return
	}

	c.airportMasterCacheMu.Lock()
	defer c.airportMasterCacheMu.Unlock()

	c.airportMasterCacheEntries = append([]AirportMaster(nil), masters...)
	c.airportMasterCacheUntil = now.Add(c.airportMasterCacheTTL)
}

func (c *Client) AirportMasterByICAO(ctx context.Context, icao string) (*AirportMaster, error) {
	masters, err := c.AirportMasters(ctx)
	if err != nil {
		return nil, err
	}

	for _, master := range masters {
		if strings.EqualFold(master.ICAO, icao) {
			match := master
			return &match, nil
		}
	}

	return nil, nil
}

func (c *Client) doRequest(
	ctx context.Context,
	method string,
	path string,
	query map[string]string,
	headers map[string]string,
	body any,
) ([]byte, error) {

	u, err := url.Parse(c.baseURL + path)
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

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewBuffer(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cdm api error (%d): %s", resp.StatusCode, string(data))
	}

	return data, nil
}
