//go:build task22fault

package app

import (
	"net/url"
	"os"
)

// Compiled local fault tests can replace external HTTP origins with loopback
// provider fixtures while exercising the actual adapters and business policy.
// Ordinary builds exclude these environment controls entirely.
func faultDependencies(deps Dependencies) Dependencies {
	for name, target := range map[string]*string{"TASK22_AIRAC_URL": &deps.NATS.AIRACBaseURL, "TASK22_WIND_URL": &deps.NATS.OpenMeteoBaseURL} {
		if value := os.Getenv(name); value != "" {
			parsed, err := url.Parse(value)
			if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" {
				panic("fault provider must be a loopback HTTP fixture")
			}
			*target = value
		}
	}
	return deps
}
