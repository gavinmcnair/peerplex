package membership

import (
	"context"
	"errors"
)

// Status represents the health/liveness of a peer.
type Status int

const (
	StatusUnknown Status = iota
	StatusAlive
	StatusSuspect
	StatusDead
	StatusLeft
)

// MembershipEvent represents a change in cluster membership or state.
type MembershipEvent struct {
	Type  Status
	Peer  Peer
	Extra map[string]string
}

// Membership is the primary interface for cluster membership control and query.
type Membership interface {
	Start(ctx context.Context) error
	Stop() error
	Self() Peer
	Live() []Peer
	All() map[PeerID]Peer
	Status() map[PeerID]Status
	GetPeer(id PeerID) (Peer, Status, bool)
	RegisterEventHandler(EventHandler)
	Broadcast(payload []byte) error
}

// Option pattern for customizations.
type Option func(cfg *Config) error

// NewMembership constructs a Membership manager.
func NewMembership(cfg *Config, opts ...Option) (Membership, error) {
	if cfg == nil {
		return nil, errors.New("config required")
	}
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}
	// Placeholder: actual implementation needed!
	return &dummyMembership{self: Peer{ID: PeerID(cfg.NodeID), Addr: cfg.BindAddr, Meta: cfg.Meta}}, nil
}

// dummyMembership is a placeholder - replace with real implementation!
type dummyMembership struct {
	self Peer
}

func (d *dummyMembership) Start(ctx context.Context) error            { return nil }
func (d *dummyMembership) Stop() error                                { return nil }
func (d *dummyMembership) Self() Peer                                 { return d.self }
func (d *dummyMembership) Live() []Peer                               { return []Peer{d.self} }
func (d *dummyMembership) All() map[PeerID]Peer                       { return map[PeerID]Peer{d.self.ID: d.self} }
func (d *dummyMembership) Status() map[PeerID]Status                  { return map[PeerID]Status{d.self.ID: StatusAlive} }
func (d *dummyMembership) GetPeer(id PeerID) (Peer, Status, bool)     { return d.self, StatusAlive, id == d.self.ID }
func (d *dummyMembership) RegisterEventHandler(EventHandler)          {}
func (d *dummyMembership) Broadcast(payload []byte) error             { return nil }

