package app

import (
	"fmt"
	"strings"
)

func (p *amanTransport) RemoveSession(session int32) {
	prefix := fmt.Sprintf("%d/", session)
	p.mu.Lock()
	defer p.mu.Unlock()
	for key := range p.lastGainLossAuthority {
		if strings.HasPrefix(key, prefix) {
			delete(p.lastGainLossAuthority, key)
		}
	}
	for key := range p.lastHoldingEAT {
		if strings.HasPrefix(key, prefix) {
			delete(p.lastHoldingEAT, key)
		}
	}
}
