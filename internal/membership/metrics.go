package membership


// Metrics exposes health/liveness counters, timings, etc.
type Metrics interface {
	Register()
	IncEvent(event string)
	ObserveLatency(peerID PeerID, latencyMs float64)
}

// (Implementation may use global vars or plug into existing metric registry)

