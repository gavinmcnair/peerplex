package membership

import (
	"sync"
	"time"
)

// NodeState represents the local node's cluster state.
type NodeState struct {
	mux     sync.RWMutex
	Self    Peer
	Status  Status
	Version uint64 // Incarnation/version number (bumps on restart or meta update)
	Since   time.Time
	Meta    map[string]string
}

// NewNodeState creates a new node state with the given peer and metadata.
func NewNodeState(self Peer, meta map[string]string) *NodeState {
	return &NodeState{
		Self:   self,
		Status: StatusAlive,
		Since:  time.Now(),
		Meta:   meta,
	}
}

func (n *NodeState) UpdateStatus(s Status) {
	n.mux.Lock()
	defer n.mux.Unlock()
	if n.Status != s {
		n.Status = s
		n.Since = time.Now()
		n.Version++
	}
}

func (n *NodeState) UpdateMeta(meta map[string]string) {
	n.mux.Lock()
	defer n.mux.Unlock()
	n.Meta = meta
	n.Version++
}

func (n *NodeState) Snapshot() (Peer, Status, uint64) {
	n.mux.RLock()
	defer n.mux.RUnlock()
	return n.Self, n.Status, n.Version
}

