package config

// DomainConfig projects the combined contract onto the existing domain types.
// It does not install a definition or select a current airport.
func (d AirportDocument) DomainConfig() Config {
	cfg := Config{
		Latitude: d.Geography.Latitude, Longitude: d.Geography.Longitude,
		TransitionAltitude: d.Geography.TransitionAltitudeFT, RunwayInitialCFL: d.Geography.InitialCFLFT,
		Positions: d.Positions, OwnerCallsignPrefixes: d.Ownership.CallsignPrefixes,
		AirborneOwners: d.Coverage.AirborneFallback, AirborneRoutes: d.Routing.Airborne,
		AirborneFallbackLayout: d.View.AirborneFallbackLayout, Layouts: d.View.Layouts,
		Runways: d.Geography.Runways, MessageAreas: d.View.MessageAreas, MissedApproachHandover: d.Routing.MissedApproach,
	}
	if settings, ok := d.Workflow.Settings.(EKCHSettings); ok {
		cfg.AirborneAltitudeAGL = settings.AirborneAltitudeAGLFT
	}
	for _, duty := range d.Coverage.Responsibilities {
		cfg.Sectors = append(cfg.Sectors, Sector{Name: duty.Name, Key: duty.Key, DisplayName: duty.DisplayName, NamePriority: duty.NamePriority, Region: duty.Region, Constraints: duty.Constraints, Active: duty.Active, RequireAll: duty.RequireAll, Owner: duty.Providers})
	}
	for _, route := range d.Routing.Ground {
		cfg.Routes = append(cfg.Routes, route.Route)
	}
	if d.Services.CDM != nil && d.Services.CDM.Config != nil {
		cfg.Cdm = *d.Services.CDM.Config
	}
	if d.Services.PDC != nil && d.Services.PDC.Validation != nil {
		cfg.PDCValidation = *d.Services.PDC.Validation
	}
	if d.Validation.Clearance != nil {
		cfg.ClxValidation = *d.Validation.Clearance
	}
	if d.Validation.TaxiwayTypes != nil {
		cfg.TaxiwayTypeValidation = *d.Validation.TaxiwayTypes
	}
	return cfg
}
