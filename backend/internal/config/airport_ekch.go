package config

import (
	"fmt"
	"github.com/golang/geo/s2"
	"slices"
	"sync"
)

var ekchStartup struct {
	sync.Mutex
	revision string
}

// InitializeEKCHCompatibility binds the existing EKCH-only domain consumers
// once at startup. It cannot select/switch another airport or hot reload data.
// New assembly consumers use AirportDefinition directly; runtime handler and
// service-state migrations remain MA-02/MA-05 work.
func InitializeEKCHCompatibility(definition *AirportDefinition) error {
	if definition == nil || definition.Airport() != "EKCH" {
		return fmt.Errorf("EKCH compatibility requires an explicit EKCH definition")
	}
	ekchStartup.Lock()
	defer ekchStartup.Unlock()
	if ekchStartup.revision != "" {
		if ekchStartup.revision != definition.Revision() {
			return fmt.Errorf("EKCH definition already bound; configuration changes require a restart")
		}
		return nil
	}
	if err := installEKCHDefinition(definition); err != nil {
		return err
	}
	ekchStartup.revision = definition.Revision()
	return nil
}
func installEKCHDefinition(definition *AirportDefinition) error {
	if definition == nil || definition.Airport() != "EKCH" {
		return fmt.Errorf("expected EKCH definition")
	}
	_, ok := definition.Resource("geography.regions")
	if !ok {
		return fmt.Errorf("EKCH geography.regions is unresolved")
	}
	if err := installEKCHDomainConfig(definition.DomainConfig()); err != nil {
		return err
	}
	cloneRegions := func(group []Region) []Region {
		result := slices.Clone(group)
		for i := range result {
			result[i].Runways = slices.Clone(result[i].Runways)
			result[i].Region = s2.LoopFromPoints(slices.Clone(result[i].Region.Vertices()))
		}
		return result
	}
	regions, runwayRegions, finalApproachRegions = cloneRegions(definition.regionGroups[0]), cloneRegions(definition.regionGroups[1]), cloneRegions(definition.regionGroups[2])
	return nil
}
