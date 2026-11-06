package events

// BoardContext identifies one authorized subscription and exact own incarnation.
// Generations are decimal strings on JSON because they exceed JS safe integers.
// Master epoch is deliberately absent: human authority stays with its own plugin.
type BoardContext struct {
	SessionID           int32  `json:"session_id"`
	Airport             string `json:"airport"`
	Environment         string `json:"environment"`
	InstanceID          string `json:"instance_id"`
	MembershipID        string `json:"membership_id"`
	TransportGeneration uint64 `json:"transport_generation,string"`
	Generation          uint64 `json:"generation,string"`
	DefinitionRevision  string `json:"definition_revision"`
	SubscriptionID      string `json:"subscription_id"`
}

func (b BoardContext) Complete() bool {
	return b.SessionID > 0 && b.Airport != "" && b.Environment != "" && b.InstanceID != "" && b.MembershipID != "" && b.TransportGeneration != 0 && b.Generation != 0 && b.DefinitionRevision != "" && b.SubscriptionID != ""
}
