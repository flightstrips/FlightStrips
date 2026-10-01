package coordinationrequest

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrRevisionConflict = errors.New("coordination request revision conflict")
	ErrCommandConflict  = errors.New("coordination request command identity conflict")
	ErrRequestNotFound  = errors.New("coordination request not found")
	ErrInvalidState     = errors.New("coordination request is not pending")
)

type CommitResult struct {
	Request           Request
	SupersededRequest *Request
	Revision          uint64
	Duplicate         bool
}

type TransferResult struct {
	Requests  []Request
	Revision  uint64
	Duplicate bool
}

type OwnershipFact struct {
	Airport    string
	Callsign   Callsign
	FactID     string
	Revision   uint64
	Owner      ControllerID
	ObservedAt time.Time
}

type ClearanceFact struct {
	Airport, FactID, Issuer, Value string
	Callsign                       Callsign
	Kind                           Kind
	ObservedAt                     time.Time
}

func coordinationRevision(requests []Request) uint64 {
	revision := uint64(len(requests))
	transfers := make(map[string]struct{})
	expiries := make(map[string]struct{})
	clearances := make(map[string]struct{})
	for _, request := range requests {
		if request.Decision != nil {
			revision++
		}
		for _, transfer := range request.RecipientTransfers {
			key := string(request.Callsign) + "\x00" + transfer.OwnershipFact + "\x00" + fmt.Sprint(transfer.OwnershipRevision)
			transfers[key] = struct{}{}
		}
		if request.Expiry != nil {
			expiries[request.Expiry.FactID+"\x00"+fmt.Sprint(request.Expiry.FactRevision)] = struct{}{}
		}
		if request.Clearance != nil {
			clearances[request.Clearance.FactID] = struct{}{}
		}
	}
	return revision + uint64(len(transfers)) + uint64(len(expiries)) + uint64(len(clearances))
}
