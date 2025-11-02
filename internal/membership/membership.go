package membership

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"time"
)

// Status represents peer liveness/health.
type Status int

const (
	StatusUnknown Status = iota
	StatusAlive
	StatusSuspect
	StatusDead
	StatusLeft
)

// MembershipEvent could be used for more complex status/event lifecycles.
type MembershipEvent struct {
	Type  Status
	Peer  Peer
	Extra map[string]string
}

// Membership is the primary membership API.
type Membership interface {
	Start(ctx context.Context) error
	Stop() error
	Self() Peer
	Live() []Peer
	All() map[PeerID]Peer
	Status() map[PeerID]Status
	GetPeer(id PeerID) (Peer, Status, bool)
	RegisterEventHandler(EventHandler)
	Broadcast([]byte) error
}

type Option func(cfg *Config) error

// -- MVP implementation below --

type mvpMembership struct {
	self    Peer
	cfg     *Config
	mu      sync.RWMutex
	peers   map[PeerID]Peer
	running bool

	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewMembership(cfg *Config, opts ...Option) (Membership, error) {
	if cfg == nil {
		return nil, errors.New("config required")
	}
	m := &mvpMembership{
		self: Peer{
			ID:   PeerID(cfg.NodeID),
			Addr: cfg.BindAddr,
			Meta: cfg.Meta,
		},
		cfg:   cfg,
		peers: make(map[PeerID]Peer),
	}
	return m, nil
}

func (m *mvpMembership) Start(ctx context.Context) error {
	ctx2, cancel := context.WithCancel(ctx)
	m.ctx = ctx2
	m.cancel = cancel

	m.mu.Lock()
	m.peers[m.self.ID] = m.self
	m.mu.Unlock()

	ln, err := net.Listen("tcp", m.self.Addr)
	if err != nil {
		return err
	}
	m.listener = ln
	m.running = true

	go m.acceptLoop() // Accept network joins/gossip

	// Join seeds
	for _, addr := range m.cfg.Seeds {
		if addr == m.self.Addr || addr == "" {
			continue
		}
		go m.sendJoin(addr)
	}

	go m.gossipLoop()

	return nil
}

func (m *mvpMembership) Stop() error {
	m.running = false
	if m.cancel != nil {
		m.cancel()
	}
	if m.listener != nil {
		m.listener.Close()
	}
	return nil
}

// -- Network/gossip logic --

type wireMsg struct {
	Type  string // "join" or "peers"
	Peer  Peer
	Peers []Peer // for multi-peer exchange
}

func (m *mvpMembership) acceptLoop() {
	for m.running {
		conn, err := m.listener.Accept()
		if err != nil {
			continue
		}
		go m.handleConn(conn)
	}
}

func (m *mvpMembership) handleConn(conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	var msg wireMsg
	if err := dec.Decode(&msg); err != nil {
		return
	}
	switch msg.Type {
	case "join":
		m.mu.Lock()
		m.peers[msg.Peer.ID] = msg.Peer
		peers := make([]Peer, 0, len(m.peers))
		for _, p := range m.peers {
			peers = append(peers, p)
		}
		m.mu.Unlock()
		out := wireMsg{Type: "peers", Peer: m.self, Peers: peers}
		enc := json.NewEncoder(conn)
		_ = enc.Encode(&out)
	case "peers":
		m.mu.Lock()
		for _, p := range msg.Peers {
			if p.ID != m.self.ID {
				m.peers[p.ID] = p
			}
		}
		m.mu.Unlock()
	}
}

func (m *mvpMembership) sendJoin(addr string) {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return
	}
	defer conn.Close()
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)
	req := wireMsg{Type: "join", Peer: m.self}
	if err := enc.Encode(&req); err != nil {
		return
	}
	var resp wireMsg
	if err := dec.Decode(&resp); err != nil {
		return
	}
	m.mu.Lock()
	for _, p := range resp.Peers {
		if p.ID != m.self.ID {
			m.peers[p.ID] = p
		}
	}
	m.mu.Unlock()
}

func (m *mvpMembership) gossipLoop() {
	t := time.NewTicker(m.cfg.GossipInterval)
	defer t.Stop()
	for m.running {
		select {
		case <-m.ctx.Done():
			return
		case <-t.C:
			m.pushToAll()
		}
	}
}

func (m *mvpMembership) pushToAll() {
	m.mu.RLock()
	peerList := make([]Peer, 0, len(m.peers))
	for _, p := range m.peers {
		if p.ID != m.self.ID {
			peerList = append(peerList, p)
		}
	}
	m.mu.RUnlock()

	for _, p := range peerList {
		go func(addr string) {
			conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
			if err != nil {
				return
			}
			defer conn.Close()
			allPeers := m.Live()
			msg := wireMsg{Type: "peers", Peer: m.self, Peers: allPeers}
			enc := json.NewEncoder(conn)
			_ = enc.Encode(&msg)
		}(p.Addr)
	}
}

// -- API methods --

func (m *mvpMembership) Self() Peer {
	return m.self
}

func (m *mvpMembership) Live() []Peer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Peer, 0, len(m.peers))
	for _, p := range m.peers {
		out = append(out, p)
	}
	return out
}

func (m *mvpMembership) All() map[PeerID]Peer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make(map[PeerID]Peer, len(m.peers))
	for k, v := range m.peers {
		cp[k] = v
	}
	return cp
}

func (m *mvpMembership) Status() map[PeerID]Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := make(map[PeerID]Status, len(m.peers))
	for k := range m.peers {
		s[k] = StatusAlive // MVP: always alive
	}
	return s
}

func (m *mvpMembership) GetPeer(id PeerID) (Peer, Status, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.peers[id]
	return p, StatusAlive, ok
}

func (m *mvpMembership) RegisterEventHandler(EventHandler)   {}
func (m *mvpMembership) Broadcast([]byte) error             { return nil }

