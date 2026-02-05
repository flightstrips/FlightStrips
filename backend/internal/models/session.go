package models

import "FlightStrips/pkg/models"

type Session struct {
	ID                 int32
	Name               string
	Airport            string
	ActiveRunways      models.ActiveRunways
	PdcSequence        int32
	PdcMessageSequence int32
}
