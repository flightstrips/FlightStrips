package app

import (
	appconfig "FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/standstatus"
	"context"
	"fmt"
)

type satInputs struct {
	registry  *sat.AirportInputRegistry
	readiness map[string]appconfig.StandAssignmentReadiness
	aggregate appconfig.StandAssignmentReadiness
}

type standSessions struct {
	sessions interface {
		List(context.Context) ([]*models.Session, error)
	}
	inputs *sat.AirportInputRegistry
}

func scopedStandSessions(sessions interface {
	List(context.Context) ([]*models.Session, error)
}, inputs *sat.AirportInputRegistry) standSessions {
	return standSessions{sessions, inputs}
}

func (s standSessions) List(ctx context.Context) ([]*models.Session, error) {
	sessions, err := s.sessions.List(ctx)
	if err != nil || s.inputs == nil {
		return sessions, err
	}
	result := make([]*models.Session, 0, len(sessions))
	for _, session := range sessions {
		if session != nil {
			if _, ok := s.inputs.Lookup(session.Airport); ok {
				result = append(result, session)
			}
		}
	}
	return result, nil
}

func configureSATInputs(cfg Config) satInputs {
	if cfg.Airports == nil {
		ready := configureStandAssignmentDefinition(cfg)
		return satInputs{aggregate: ready} // explicit legacy assembly fixtures
	}
	graph := satInputs{readiness: make(map[string]appconfig.StandAssignmentReadiness)}
	inputs := make(map[string]sat.AirportInputs)
	aircraft := make(map[string]*sat.AircraftRegistry)
	engines := make(map[string]*sat.AircraftEngineRegistry)
	borders := sat.NewAirportCountryRegistry()
	for _, airport := range cfg.Airports.Airports() {
		definition, _ := cfg.Airports.Lookup(airport)
		ready := appconfig.StandAssignmentReadiness{Enabled: definition.ServiceEnabled("stand_assignment")}
		if !ready.Enabled {
			graph.readiness[airport] = ready
			continue
		}
		graph.aggregate.Enabled = true
		resource := func(name string) string {
			path, _ := definition.Resource("services.stand_assignment." + name)
			return path
		}
		load := func() (sat.AirportInputs, error) {
			refPath, enginePath := resource("aircraft_reference"), resource("aircraft_icao")
			ref := aircraft[refPath]
			if ref == nil {
				var err error
				ref, err = sat.LoadAircraftReferenceFile(refPath)
				if err != nil {
					return sat.AirportInputs{}, err
				}
				aircraft[refPath] = ref
			}
			engineKey := refPath + "\x00" + enginePath
			engine := engines[engineKey]
			if engine == nil {
				var err error
				engine, err = sat.LoadAircraftEngineReferenceFile(enginePath, ref)
				if err != nil {
					return sat.AirportInputs{}, err
				}
				engines[engineKey] = engine
			}
			stands, err := sat.LoadStandCapabilityFile(resource("stands"))
			if err != nil {
				return sat.AirportInputs{}, err
			}
			if len(stands.Stands(airport)) == 0 {
				return sat.AirportInputs{}, fmt.Errorf("stand input has no stands for %s", airport)
			}
			policy, err := sat.LoadAirlineAssignmentFile(resource("airline_assignment"), stands.ForAirport(airport))
			if err != nil {
				return sat.AirportInputs{}, err
			}
			return sat.AirportInputs{Aircraft: ref, Engines: engine, Borders: borders, Stands: stands, Policy: policy}, nil
		}
		input, err := load()
		if err != nil {
			ready.Reason = fmt.Sprintf("load %s stand inputs: %v", airport, err)
		} else {
			ready.Ready = true
			graph.aggregate.Ready = true
			inputs[airport] = input
		}
		graph.readiness[airport] = ready
		if !ready.Ready && graph.aggregate.Reason == "" {
			graph.aggregate.Reason = ready.Reason
		}
	}
	graph.registry = sat.NewAirportInputRegistry(inputs)
	return graph
}

func (g satInputs) diagnostics() standstatus.WebAPIDiagnostics {
	if g.registry == nil {
		return standAssignmentDiagnostics()
	}
	var result standstatus.WebAPIDiagnostics
	for airport := range g.readiness {
		input, ok := g.registry.Lookup(airport)
		if !ok {
			continue
		}
		result.AircraftTypes += len(input.Aircraft.Types())
		stands := input.Stands.Stands(airport)
		result.Stands += len(stands)
		for _, stand := range stands {
			result.StandVariants += len(stand.Variants)
		}
		result.AirlineRules += len(input.Policy.Rules)
		result.StandGroups += len(input.Policy.StandGroups)
		result.FallbackRules += len(input.Policy.FallbackRules)
	}
	return result
}
