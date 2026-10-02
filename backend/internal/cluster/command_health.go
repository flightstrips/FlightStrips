package cluster

// commandHealth protects the routing and verified owner commit paths without
// claiming that the independent replay cursor has reached every aggregate.
func (p *Projection) commandHealth() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if err := p.healthLocked(); err != nil {
		return err
	}
	if p.history != nil {
		return p.history.check()
	}
	return nil
}
