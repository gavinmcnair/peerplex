package membership

import (
	"sync"
	"time"
)

// FailureDetector interface for generic failure detection.
type FailureDetector interface {
	Tick(now time.Time)
	NotifyPeerSeen(peer Peer, now time.Time)
	PeerStatus(peer Peer) Status
	MarkSuspect(peer Peer, reason string)
	MarkAlive(peer Peer)
	MarkDead(peer Peer)
}

// BasicSWIMFailureDetector is an MVP example; extend for production.
type BasicSWIMFailureDetector struct {
	mux     sync.Mutex
	status  map[PeerID]Status
	seenAt  map[PeerID]time.Time
	timeout time.Duration
}

func NewBasicSWIMFailureDetector(timeout time.Duration) *BasicSWIMFailureDetector {
	return &BasicSWIMFailureDetector{
		status:  make(map[PeerID]Status),
		seenAt:  make(map[PeerID]time.Time),
		timeout: timeout,
	}
}

func (fd *BasicSWIMFailureDetector) Tick(now time.Time) {
	fd.mux.Lock()
	defer fd.mux.Unlock()
	for id, last := range fd.seenAt {
		if now.Sub(last) > fd.timeout {
			fd.status[id] = StatusSuspect
		}
	}
}

func (fd *BasicSWIMFailureDetector) NotifyPeerSeen(peer Peer, now time.Time) {
	fd.mux.Lock()
	defer fd.mux.Unlock()
	fd.seenAt[peer.ID] = now
	fd.status[peer.ID] = StatusAlive
}

func (fd *BasicSWIMFailureDetector) PeerStatus(peer Peer) Status {
	fd.mux.Lock()
	defer fd.mux.Unlock()
	st, ok := fd.status[peer.ID]
	if !ok {
		return StatusUnknown
	}
	return st
}

func (fd *BasicSWIMFailureDetector) MarkSuspect(peer Peer, reason string) {
	fd.mux.Lock()
	defer fd.mux.Unlock()
	fd.status[peer.ID] = StatusSuspect
}

func (fd *BasicSWIMFailureDetector) MarkAlive(peer Peer) {
	fd.mux.Lock()
	defer fd.mux.Unlock()
	fd.status[peer.ID] = StatusAlive
}

func (fd *BasicSWIMFailureDetector) MarkDead(peer Peer) {
	fd.mux.Lock()
	defer fd.mux.Unlock()
	fd.status[peer.ID] = StatusDead
}

