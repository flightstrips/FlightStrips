package cdm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

func fetchRates(ctx context.Context, client *http.Client, rawURL string) ([]CdmRate, error) {
	var result []CdmRate
	if err := fetchJSON(ctx, client, rawURL, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func fetchSidIntervals(ctx context.Context, client *http.Client, rawURL string) ([]CdmSidInterval, error) {
	var result []CdmSidInterval
	if err := fetchJSON(ctx, client, rawURL, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func fetchTaxiZones(ctx context.Context, client *http.Client, rawURL string) ([]CdmTaxiZone, error) {
	var result []CdmTaxiZone
	if err := fetchJSON(ctx, client, rawURL, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func fetchJSON(ctx context.Context, client *http.Client, rawURL string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}
