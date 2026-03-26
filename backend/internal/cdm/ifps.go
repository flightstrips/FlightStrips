package cdm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
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
	ReqASRT     string `json:"reqAsrt,omitempty"`
	ID          string `json:"_id"`
}

func (c *Client) IFPSDpi(ctx context.Context, callsign, value string) error {
	_, err := c.doRequest(ctx, "POST", "/ifps/dpi",
		map[string]string{
			"callsign": callsign,
			"value":    value,
		},
		nil,
		nil,
	)
	return err
}

type SetCdmDataParams struct {
	Callsign string
	Tobt     string
	Tsat     string
	Ttot     string
	Ctot     string
	Reason   string // ECFMP flow reason ID
	Asrt     string
	DepInfo  string // e.g. departure runway
}

func (c *Client) IFPSSetCdmData(ctx context.Context, p SetCdmDataParams) error {
	_, err := c.doRequest(ctx, "POST", "/ifps/setCdmData",
		map[string]string{
			"callsign": p.Callsign,
			"tobt":     p.Tobt,
			"tsat":     p.Tsat,
			"ttot":     p.Ttot,
			"ctot":     p.Ctot,
			"reason":   p.Reason,
			"asrt":     p.Asrt,
			"depInfo":  p.DepInfo,
		},
		nil,
		nil,
	)
	return err
}

func (c *Client) SetMasterAirport(ctx context.Context, airport, position string) error {
	_, err := c.doRequest(ctx, "POST", "/airport/setMaster",
		map[string]string{
			"airport":  airport,
			"position": position,
		},
		nil,
		nil,
	)
	return err
}

type DepartureRestriction struct {
	Airport string `json:"airport"`
	Rate    int    `json:"rate"`
	RateLvo int    `json:"rateLvo,omitempty"`
}

func (c *Client) ClearMasterAirport(ctx context.Context, airport, position string) error {
	_, err := c.doRequest(ctx, "POST", "/airport/clearMaster",
		map[string]string{
			"airport":  airport,
			"position": position,
		},
		nil,
		nil,
	)
	return err
}

func (c *Client) GetDepartureRestrictions(ctx context.Context) ([]DepartureRestriction, error) {
	bytes, err := c.doRequest(ctx, "GET", "/etfms/restrictions",
		map[string]string{"type": "DEP"},
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}
	var result []DepartureRestriction
	if err := json.Unmarshal(bytes, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) IFPSSetTobt(ctx context.Context, callsign, tobt string, taxiMinutes int) error {
	_, err := c.doRequest(ctx, "POST", "/ifps/dpi",
		map[string]string{
			"callsign": callsign,
			"value":    fmt.Sprintf("TOBT/%s/%d", tobt, taxiMinutes),
		},
		nil,
		nil,
	)
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

func (c *Client) AirportMasters(ctx context.Context) ([]AirportMaster, error) {
	now := time.Now()
	if masters, ok := c.airportMastersFromCache(now); ok {
		return masters, nil
	}

	bytes, err := c.doRequest(ctx, "GET", "/airport", nil, nil, nil)
	if err != nil {
		return nil, err
	}

	var result []AirportMaster
	if err := json.Unmarshal(bytes, &result); err != nil {
		return nil, err
	}

	c.storeAirportMasters(now, result)
	return append([]AirportMaster(nil), result...), nil
}
