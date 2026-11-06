package cdm

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// AirportConfigInput binds typed defaults and resources once at startup. The
// store owns one refresh worker and one provider client across all inputs.
type AirportConfigInput struct {
	Config                               *CdmAirportConfig
	RateURI, SIDIntervalURI, TaxiZoneURI string
}

func NewAirportConfigStore(inputs []AirportConfigInput, interval time.Duration, client *http.Client) *CdmConfigStore {
	s := NewCdmConfigStore("", "", "", interval, CdmConfigDefaults{}, client)
	s.airportInputs = make(map[string]AirportConfigInput, len(inputs))
	for _, input := range inputs {
		if input.Config == nil {
			continue
		}
		key := normalizeToken(input.Config.Airport)
		input.Config = input.Config.Clone()
		input.Config.Airport = key
		s.airportInputs[key] = input
		s.configs[key] = input.Config.Clone()
	}
	return s
}

func (s *CdmConfigStore) refreshAirportInputs(ctx context.Context) {
	// Referenced endpoints can serve multiple selected airports. Fetch once per
	// cycle, then apply only the explicitly bound airport's rows.
	type resource struct {
		data []byte
		err  error
	}
	resources := make(map[string]resource)
	read := func(uri string) ([]byte, error) {
		if r, ok := resources[uri]; ok {
			return r.data, r.err
		}
		data, err := s.fetchBytes(ctx, uri)
		resources[uri] = resource{data, err}
		return data, err
	}
	for airport, input := range s.airportInputs {
		next := s.ConfigForAirport(airport)
		for kind, uri := range map[string]string{"rates": input.RateURI, "sid": input.SIDIntervalURI, "taxi": input.TaxiZoneURI} {
			if uri == "" {
				continue
			}
			data, err := read(uri)
			if err == nil {
				switch kind {
				case "rates":
					var rows []CdmRate
					rows, err = parseRateData(data)
					if err == nil {
						next.Rates = nil
						for _, row := range rows {
							if normalizeToken(row.Airport) == airport {
								next.Rates = append(next.Rates, row)
							}
						}
					}
				case "sid":
					var rows []CdmSidInterval
					rows, err = parseSidIntervalData(data)
					if err == nil {
						next.SidIntervals = nil
						for _, row := range rows {
							if normalizeToken(row.Airport) == airport {
								next.SidIntervals = append(next.SidIntervals, row)
							}
						}
					}
				case "taxi":
					var rows []CdmTaxiZone
					rows, err = parseTaxiZoneData(data)
					if err == nil {
						next.TaxiZones = nil
						for _, row := range rows {
							if normalizeToken(row.Airport) == airport {
								next.TaxiZones = append(next.TaxiZones, row)
							}
						}
					}
				}
			}
			if err != nil {
				slog.WarnContext(ctx, "CDM airport input refresh failed", "airport", airport, "input", kind, "error", err)
			}
		}
		// Preserve immutable defaults (provider restrictions are a read overlay).
		next.DefaultRate, next.DefaultRateLvo = input.Config.DefaultRate, input.Config.DefaultRateLvo
		s.mu.Lock()
		s.configs[airport] = next
		s.mu.Unlock()
	}
}

func (s *Service) airportEnabled(airport string) bool {
	if store, ok := s.configProvider.(*CdmConfigStore); ok && store.airportInputs != nil {
		return store.ConfigForAirport(airport) != nil
	}
	return true
}

func (s *SequenceService) configForSession(sessionAirport string, arrivals, departures []string, runwayStatus map[string]string) *CdmAirportConfig {
	cfg := s.configForActiveRunways(sessionAirport, arrivals, departures)
	if store, ok := s.configProvider.(*CdmConfigStore); ok && store.airportInputs != nil && cfg != nil {
		cfg.LvoActive = hasLowVisRunwayStatus(runwayStatus)
	}
	return cfg
}
