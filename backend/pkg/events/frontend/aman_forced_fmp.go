package frontend

const (
	// AMANForceFMPType asks the server to give the sending controller forced
	// FMP authority. It takes the grant from any current holder.
	AMANForceFMPType EventType = "aman.force_fmp"
	// AMANReleaseForcedFMPType gives back the sender's forced FMP authority.
	AMANReleaseForcedFMPType EventType = "aman.release_forced_fmp"
	// AMANFMPAuthorityType tells one client its FMP authority and who, if
	// anyone, currently holds forced FMP in its session.
	AMANFMPAuthorityType EventType = "aman.fmp_authority"
)

type AMANForcedFMPRequest struct {
	CommandID string `json:"command_id"`
}

type AMANForcedFMPMessage struct {
	Type    EventType            `json:"type"`
	Version int                  `json:"version"`
	Data    AMANForcedFMPRequest `json:"data"`
}

// AMANFMPAuthorityEvent is computed per client: HasFMPAuthority and Forced
// describe the receiving client, the other fields describe its session.
type AMANFMPAuthorityEvent struct {
	Type    EventType        `json:"type"`
	Version int              `json:"version"`
	Data    AMANFMPAuthority `json:"data"`
}

type AMANFMPAuthority struct {
	HasFMPAuthority bool `json:"has_fmp_authority"`
	// Forced is true when the client's authority comes from a forced grant.
	Forced bool `json:"forced"`
	// ForcedHolder is the callsign holding forced FMP, or null.
	ForcedHolder *string `json:"forced_holder"`
	// FMPOnline is true when an FMP position is logged on in the session.
	FMPOnline bool `json:"fmp_online"`
	// CanForce is true when the client may force FMP now.
	CanForce bool `json:"can_force"`
}

func (e AMANFMPAuthorityEvent) Marshal() ([]byte, error) { return marshall(e) }
func (AMANFMPAuthorityEvent) GetType() EventType         { return AMANFMPAuthorityType }
