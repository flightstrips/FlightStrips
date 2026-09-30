package cluster

import (
	"errors"
	"fmt"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

const effectDispatchWindow = 30 * time.Second
const effectResultWindow = 30 * time.Second
const effectRetention = 24 * time.Hour

var errEffectDeadlineRace = errors.New("effect deadline changed before commit")

func validateEffectRequest(effect *pb.EffectRecord, commandID string, previous *pb.EffectRecord) error {
	if effect == nil || previous != nil || !canonicalUUID(effect.CommandId) || effect.CommandId != commandID ||
		effect.TargetCid == "" || effect.GetPayload() == nil || effect.Status != pb.EffectRecord_WAITING ||
		effect.DispatchDeadline == nil || effect.ResultDeadline != nil || effect.DispatchConnectionId != nil || effect.ReasonCode != "" {
		return fmt.Errorf("invalid effect request")
	}
	if secret := effect.GetPrivateMessage(); secret != nil &&
		(secret.ObjectName != "effect/"+commandID || len(secret.Sha256) != 64 || secret.Recipient == "") {
		return fmt.Errorf("invalid private message object reference")
	}
	if squawk := effect.GetGenerateSquawk(); squawk != nil && !canonicalAircraft(squawk.Callsign) {
		return fmt.Errorf("invalid squawk effect")
	}
	return nil
}

func validateEffectTransition(old, next *pb.EffectRecord, outcome *pb.CommandOutcome, serverTime time.Time) error {
	if old == nil || next == nil || outcome == nil || outcome.Status != pb.CommandOutcome_ACCEPTED ||
		old.CommandId != next.CommandId || old.TargetCid != next.TargetCid ||
		!proto.Equal(effectPayload(old), effectPayload(next)) ||
		!proto.Equal(old.DispatchDeadline, next.DispatchDeadline) ||
		!optionalStringEqual(old.TargetConnectionId, next.TargetConnectionId) {
		return fmt.Errorf("effect identity or payload changed")
	}
	switch old.Status {
	case pb.EffectRecord_WAITING:
		if old.DispatchDeadline == nil || next.ResultDeadline != nil {
			return fmt.Errorf("invalid effect dispatch deadline")
		}
		if serverTime.After(old.DispatchDeadline.AsTime()) && next.Status != pb.EffectRecord_EXPIRED && !(old.GetGenerateSquawk() != nil && next.Status == pb.EffectRecord_FAILED) {
			return errEffectDeadlineRace
		}
		switch next.Status {
		case pb.EffectRecord_DISPATCH_CLAIMED:
			if next.DispatchConnectionId == nil || *next.DispatchConnectionId == "" || next.OwnerEpoch == 0 || next.ReasonCode != "" {
				return fmt.Errorf("invalid effect dispatch claim")
			}
		case pb.EffectRecord_EXPIRED:
			if serverTime.Before(old.DispatchDeadline.AsTime()) {
				return errEffectDeadlineRace
			}
			if next.DispatchConnectionId != nil || next.OwnerEpoch != old.OwnerEpoch || next.MasterEpoch != old.MasterEpoch {
				return fmt.Errorf("invalid effect expiry")
			}
		case pb.EffectRecord_FAILED:
			if old.GetGenerateSquawk() == nil || next.DispatchConnectionId != nil ||
				(next.ReasonCode != "SQUAWK_ASSIGNED" && next.ReasonCode != "STRIP_REMOVED") ||
				next.OwnerEpoch != old.OwnerEpoch || next.MasterEpoch != old.MasterEpoch {
				return fmt.Errorf("invalid waiting squawk cancellation")
			}
		default:
			return fmt.Errorf("invalid waiting effect transition")
		}
	case pb.EffectRecord_DISPATCH_CLAIMED:
		if next.Status != pb.EffectRecord_EXECUTED && next.Status != pb.EffectRecord_FAILED && next.Status != pb.EffectRecord_UNKNOWN {
			return fmt.Errorf("claimed effect cannot be dispatched again")
		}
		if !optionalStringEqual(old.DispatchConnectionId, next.DispatchConnectionId) ||
			old.OwnerEpoch != next.OwnerEpoch || old.MasterEpoch != next.MasterEpoch ||
			!proto.Equal(old.ResultDeadline, next.ResultDeadline) || old.ResultDeadline == nil {
			return fmt.Errorf("effect claim changed")
		}
		if next.Status == pb.EffectRecord_UNKNOWN {
			if serverTime.Before(old.ResultDeadline.AsTime()) {
				return errEffectDeadlineRace
			}
		} else if serverTime.After(old.ResultDeadline.AsTime()) {
			return errEffectDeadlineRace
		} else if next.Status == pb.EffectRecord_FAILED && next.ReasonCode == "" {
			return fmt.Errorf("invalid or late effect result")
		}
	default:
		return fmt.Errorf("terminal effect cannot change")
	}
	return nil
}

func optionalStringEqual(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func effectPayload(e *pb.EffectRecord) proto.Message {
	switch p := e.GetPayload().(type) {
	case *pb.EffectRecord_SetFlightPlan:
		return p.SetFlightPlan
	case *pb.EffectRecord_Pdc:
		return p.Pdc
	case *pb.EffectRecord_PrivateMessage:
		return p.PrivateMessage
	case *pb.EffectRecord_Coordination:
		return p.Coordination
	case *pb.EffectRecord_Cdm:
		return p.Cdm
	case *pb.EffectRecord_AmanHoldingEat:
		return p.AmanHoldingEat
	case *pb.EffectRecord_GenerateSquawk:
		return p.GenerateSquawk
	default:
		return nil
	}
}
