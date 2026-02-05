package models

// CdmData represents CDM (Collaborative Decision Making) data for a strip
type CdmData struct {
	Callsign  string
	Tobt      *string
	Tsat      *string
	Ttot      *string
	Ctot      *string
	Aobt      *string
	Asat      *string
	Eobt      *string
	CdmStatus *string
}
