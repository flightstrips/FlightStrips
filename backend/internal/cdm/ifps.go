package cdm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type BulkIFPSData []IFPSData

type IFPSData struct {
	Callsign               string  `json:"callsign"`
	CID                    string  `json:"cid"`
	Departure              string  `json:"departure"`
	Arrival                string  `json:"arrival"`
	EOBT                   string  `json:"eobt"` // Estimated Off-Block Time
	TOBT                   string  `json:"tobt"` // Target Off-Block Time
	ReqTOBT                string  `json:"reqTobt"`
	Taxi                   int     `json:"taxi"`
	CTOT                   string  `json:"ctot"` // Calculated Take-Off Time
	AOBT                   string  `json:"aobt"` // Actual Off-Block Time
	ATOT                   string  `json:"atot"` // Actual Take-Off Time
	ETA                    string  `json:"eta"`
	MostPenalizingAirspace string  `json:"mostPenalizingAirspace"`
	CDMStatus              string  `json:"cdmSts"`
	ATFCMStatus            string  `json:"atfcmStatus"`
	CDMData                CDMData `json:"cdmData"`
}

type CDMData struct {
	TOBT        string `json:"tobt"`
	TSAT        string `json:"tsat"` // Target Startup Approval Time
	TTOT        string `json:"ttot"` // Target Take-Off Time
	CTOT        string `json:"ctot"`
	Reason      string `json:"reason"`
	ReqTOBT     string `json:"reqTobt"`
	ReqTOBTType string `json:"reqTobtType,omitempty"`
	ID          string `json:"_id"`
}

func (c *Client) IFPSSetCDMStatus(ctx context.Context, callsign, status string) error {
	bytes, err := c.doRequest(ctx, "POST", "/ifps/setCdmStatus",
		map[string]string{
			"callsign": callsign,
			"cdmSts":   status,
		},
		nil,
		nil,
	)

	if err != nil {
		return err
	}

	result := string(bytes)
	if strings.ToLower(result) != "true" {
		return fmt.Errorf("set CDM status '%s' failed for callsign: %s", status, callsign)
	}

	return err
}

func (c *Client) IFPSByCallsign(ctx context.Context, callsign string) ([]byte, error) {
	return c.doRequest(ctx, "GET", "/ifps/callsign",
		map[string]string{"callsign": callsign},
		nil,
		nil,
	)
}

func (c *Client) IFPSByDepartureAirport(ctx context.Context, airport string) (BulkIFPSData, error) {
	bytes, err := c.doRequest(ctx, "GET", "/ifps/depAirport",
		map[string]string{"airport": airport},
		nil,
		nil,
	)

	if err != nil {
		return nil, err
	}

	var result BulkIFPSData
	err = json.Unmarshal(bytes, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
