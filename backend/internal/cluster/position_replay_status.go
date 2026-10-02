package cluster

import "fmt"

// PositionReplayStatus reports only local synchronization metadata. It performs
// no broker requests and exposes no aircraft values or credentials, so failed
// recovery diagnostics cannot change replay or block on unavailable quorum.
func (p *Projection) PositionReplayStatus() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	c := p.positionCursor
	return fmt.Sprintf("position_ready=%t presence_ready=%t consumer=%s applied_consumer=%d applied_stream=%d proof_consumer_name=%s proof_consumer=%d proof_stream=%d last_proved=%s retained=%d metadata_at=%s stream_created=%s stream_first=%d stream_head=%d stream_messages=%d metadata_consumer=%s delivered_consumer=%d delivered_stream=%d pending=%d replay_problem=%q", p.positionReady, p.presenceReady, c.consumer, c.appliedConsumer, c.appliedStream, c.provedName, c.provedConsumer, c.provedStream, c.lastProved, len(c.retained), c.metadataAt, c.metadataStreamCreated, c.metadataStreamFirst, c.metadataStreamHead, c.metadataStreamMessages, c.metadataConsumer, c.metadataDeliveredConsumer, c.metadataDeliveredStream, c.metadataPending, p.positionReplayProblem)
}
