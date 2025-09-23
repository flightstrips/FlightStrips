package sectors

import "github.com/golang/geo/s2"

type SectorRegion struct {
	Name   string
	Region *s2.Loop
}

type Sector struct {
	Name   string
	Region string
	Active []string
	Owner  []string
}
