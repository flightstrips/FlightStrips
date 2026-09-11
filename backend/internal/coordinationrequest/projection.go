package coordinationrequest

import (
	"fmt"
	"sort"
)

// Audience contains server-derived viewer identity. Controller is the
// controller callsign, while Role identifies an authorized FMP position.
type Audience struct {
	Controller ControllerID
	Role       string
}

// Project returns the request read model visible to one audience. Controllers
// receive only pending requests assigned to them. The originating FMP position
// retains every state so supersession and later decisions remain observable.
func Project(requests []Request, audience Audience, fmpRoles []string) ([]Request, error) {
	authorizedFMP := false
	for _, role := range fmpRoles {
		if role == audience.Role {
			authorizedFMP = true
			break
		}
	}

	projected := make([]Request, 0, len(requests))
	for _, request := range requests {
		if err := request.Validate(); err != nil {
			return nil, fmt.Errorf("project invalid coordination request: %w", err)
		}
		request.RecipientStatus = request.effectiveRecipientStatus()
		forOriginatingFMP := authorizedFMP && request.SubmittedRole == audience.Role
		forRecipient := request.State == StatePending && request.RecipientStatus != RecipientUnassigned &&
			request.RecipientController == audience.Controller
		if forOriginatingFMP || forRecipient {
			projected = append(projected, request)
		}
	}
	sort.Slice(projected, func(i, j int) bool {
		if projected[i].CreatedAt.Equal(projected[j].CreatedAt) {
			return projected[i].ID < projected[j].ID
		}
		return projected[i].CreatedAt.Before(projected[j].CreatedAt)
	})
	return projected, nil
}
