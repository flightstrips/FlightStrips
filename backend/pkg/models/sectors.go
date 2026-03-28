package models

type ActiveRunways struct {
	DepartureRunways []string          `json:"departure_runways"`
	ArrivalRunways   []string          `json:"arrival_runways"`
	RunwayStatus     map[string]string `json:"runway_status,omitempty"` // pair → "OPEN"|"LOW_VIS"|"CLOSED"
}

func (active ActiveRunways) GetAllActiveRunways() []string {
	var runways = make([]string, 0)
	for _, runway := range active.DepartureRunways {
		runways = append(runways, runway)
	}
	for _, runway := range active.ArrivalRunways {
		runways = append(runways, runway)
	}
	return runways
}
