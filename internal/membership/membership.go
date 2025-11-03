package membership

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

type Status int

const (
	StatusUnknown Status = iota
	StatusAlive
	StatusSuspect
	StatusDead
	StatusLeft
)

type Membership interface {
	Start(ctx context.Context) error
	Stop() error
	Self() Peer
	Live() []Peer
	GetPeer(id PeerID) (Peer, Status, bool)
	Status() map[PeerID]Status
	RegisterEventHandler(EventHandler)
	Broadcast([]byte) error
}

type swimPeer struct {
	Peer        Peer
	Status      Status
	Incarnation uint64
	LastSeen    time.Time
	SuspectAge  time.Time
}

type swimChange struct {
	ID          PeerID
	Status      Status
	Incarnation uint64
}

type swimMembership struct {
	self           Peer
	cfg            *Config
	mu             sync.RWMutex
	peers          map[PeerID]*swimPeer
	incarnation    uint64
	changes        []swimChange
	eventHandlers  []EventHandler
	listener       net.Listener
	ctx            context.Context
	cancel         context.CancelFunc
	running        bool
	wg             sync.WaitGroup
	stopCh         chan struct{}
	broadcastCh    chan []byte
}

// Network messages
type swimMsg struct {
	Type        string
	Peer        Peer
	Incarnation uint64
	Changes     []swimChange
	RelayTo     PeerID // for ping-req
}

func NewMembership(cfg *Config) (Membership, error) {
	if cfg == nil {
		return nil, errors.New("config required")
	}
	inc := uint64(time.Now().UnixNano())
	m := &swimMembership{
		self: Peer{
			ID:   PeerID(cfg.NodeID),
			Addr: cfg.BindAddr,
			Meta: cfg.Meta,
		},
		cfg:           cfg,
		peers:         make(map[PeerID]*swimPeer),
		incarnation:   inc,
		stopCh:        make(chan struct{}),
		broadcastCh:   make(chan []byte, 128),
	}
	return m, nil
}

func (m *swimMembership) Start(ctx context.Context) error {
	ctx2, cancel := context.WithCancel(ctx)
	m.ctx = ctx2
	m.cancel = cancel

	m.peers[m.self.ID] = &swimPeer{
		Peer:        m.self,
		Status:      StatusAlive,
		Incarnation: m.incarnation,
		LastSeen:    time.Now(),
	}

	ln, err := net.Listen("tcp", m.self.Addr)
	if err != nil {
		return err
	}
	m.listener = ln
	m.running = true

	m.wg.Add(1)
	go m.acceptLoop()

	for _, addr := range m.cfg.Seeds {
		if addr == m.self.Addr || addr == "" {
			continue
		}
		go m.sendPing(addr, nil)
	}

	m.wg.Add(1)
	go m.tickLoop()

	return nil
}

func (m *swimMembership) Stop() error {
	m.running = false
	close(m.stopCh)
	if m.cancel != nil {
		m.cancel()
	}
	if m.listener != nil {
		m.listener.Close()
	}
	m.wg.Wait()
	return nil
}

func (m *swimMembership) tickLoop() {
	defer m.wg.Done()
	t := time.NewTicker(m.cfg.GossipInterval)
	defer t.Stop()
	for m.running {
		select {
		case <-t.C:
			m.swimProtocolRound()
		case <-m.ctx.Done():
			return
		case <-m.stopCh:
			return
		}
	}
}

func (m *swimMembership) acceptLoop() {
	defer m.wg.Done()
	for m.running {
		conn, err := m.listener.Accept()
		if err != nil {
			select {
			case <-m.stopCh:
				return // shut down
			default:
			}
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				time.Sleep(time.Millisecond * 50)
				continue
			}
			// Permanent error (listener closed etc): exit
			return
		}
		go m.handleConn(conn)
	}
}


// === THE KEY: Any message from a peer always updates LastSeen, Status, etc. ===

func (m *swimMembership) handleConn(conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	var msg swimMsg
	if err := dec.Decode(&msg); err != nil {
		return
	}

	m.mu.Lock()
	if msg.Peer.ID != m.self.ID {
		ps, exists := m.peers[msg.Peer.ID]
		now := time.Now()
		if !exists {
			m.peers[msg.Peer.ID] = &swimPeer{
				Peer:        msg.Peer,
				Status:      StatusAlive,
				Incarnation: msg.Incarnation,
				LastSeen:    now,
			}
			log.Printf("[membership] discovered peer: %s", msg.Peer.ID)
		} else {
			if msg.Incarnation > ps.Incarnation || ps.Status != StatusAlive {
				ps.Status = StatusAlive
				ps.Incarnation = msg.Incarnation
				ps.SuspectAge = time.Time{}
			}
			ps.LastSeen = now
		}
	}
	m.mu.Unlock()

	switch msg.Type {
	case "ping":
		resp := swimMsg{Type: "ack", Peer: m.self, Incarnation: m.incarnation, Changes: m.consumeChanges()}
		_ = json.NewEncoder(conn).Encode(&resp)
		m.mergeChanges(msg.Changes)
	case "ack":
		m.mergeChanges(msg.Changes)
	case "ping-req":
		_ = m.handlePingReq(msg, conn)
	case "suspect", "alive", "dead":
		m.mergeChanges(append(msg.Changes, swimChange{
			ID:          msg.Peer.ID,
			Status:      msgTypeToStatus(msg.Type),
			Incarnation: msg.Incarnation,
		}))
	}
}

// -- Protocol --

func msgTypeToStatus(t string) Status {
	switch t {
	case "suspect":
		return StatusSuspect
	case "alive":
		return StatusAlive
	case "dead":
		return StatusDead
	default:
		return StatusUnknown
	}
}

func (m *swimMembership) swimProtocolRound() {
	m.cleanup()

	peers := m.getPeersExceptSelf()
	probeN := len(peers)
	if probeN > 3 {
		probeN = 3
	}
	perm := rand.Perm(len(peers))
	for i := 0; i < probeN; i++ {
		target := peers[perm[i]]
		success := m.directPing(target.Peer.Addr, target.Peer.ID)
		if !success {
			m.indirectProbe(target.Peer.ID, probeN)
		}
	}
}

func (m *swimMembership) getPeersExceptSelf() []*swimPeer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*swimPeer
	for id, ps := range m.peers {
		if id != m.self.ID {
			out = append(out, ps)
		}
	}
	return out
}

func (m *swimMembership) directPing(addr string, peerID PeerID) bool {
	ch := make(chan bool, 1)
	go func() {
		ch <- m.sendPing(addr, nil)
	}()
	select {
	case ok := <-ch:
		return ok
	case <-time.After(m.cfg.ProbeTimeout):
		return false
	}
}

func (m *swimMembership) sendPing(addr string, relay *PeerID) bool {
	conn, err := net.DialTimeout("tcp", addr, m.cfg.ProbeTimeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	req := swimMsg{Type: "ping", Peer: m.self, Incarnation: m.incarnation, Changes: m.consumeChanges()}
	if relay != nil {
		req.Type = "ping-req"
		req.RelayTo = *relay
	}
	enc := json.NewEncoder(conn)
	if err := enc.Encode(&req); err != nil {
		return false
	}
	conn.SetReadDeadline(time.Now().Add(m.cfg.ProbeTimeout))
	var resp swimMsg
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return false
	}
	m.mergeChanges(resp.Changes)
	return resp.Type == "ack"
}

func (m *swimMembership) indirectProbe(target PeerID, fanout int) {
	if fanout < 2 {
		fanout = 2
	}
	relays := m.pickRandomRelays(target, fanout)
	for _, relay := range relays {
		go func(addr string) {
			m.sendPing(addr, &target)
		}(relay.Peer.Addr)
	}
	// Suspect if no proof
	go func() {
		time.Sleep(m.cfg.ProbeTimeout * 2)
		if m.suspectIfNoProof(target) {
			log.Printf("[membership] suspect %v", target)
			m.enqueueChange(target, StatusSuspect)
		}
	}()
}

func (m *swimMembership) pickRandomRelays(exclude PeerID, k int) []*swimPeer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var candidates []*swimPeer
	for _, ps := range m.peers {
		if ps.Peer.ID != m.self.ID && ps.Peer.ID != exclude && ps.Status == StatusAlive {
			candidates = append(candidates, ps)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	perm := rand.Perm(len(candidates))
	var out []*swimPeer
	for i := 0; i < k && i < len(candidates); i++ {
		out = append(out, candidates[perm[i]])
	}
	return out
}

func (m *swimMembership) handlePingReq(msg swimMsg, directConn net.Conn) error {
	targetID := msg.RelayTo
	if targetID == m.self.ID {
		resp := swimMsg{Type: "ack", Peer: m.self, Incarnation: m.incarnation, Changes: m.consumeChanges()}
		return json.NewEncoder(directConn).Encode(&resp)
	}
	m.mu.RLock()
	ps, exists := m.peers[targetID]
	m.mu.RUnlock()
	if !exists {
		return nil
	}
	ok := m.directPing(ps.Peer.Addr, targetID)
	_ = ok
	resp := swimMsg{Type: "ack", Peer: ps.Peer, Incarnation: ps.Incarnation, Changes: m.consumeChanges()}
	return json.NewEncoder(directConn).Encode(&resp)
}

func (m *swimMembership) enqueueChange(id PeerID, status Status) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.changes = append(m.changes, swimChange{ID: id, Status: status, Incarnation: m.nextIncarnation(id, status)})
}

func (m *swimMembership) consumeChanges() []swimChange {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := m.changes
	m.changes = nil
	return ch
}

func (m *swimMembership) mergeChanges(changes []swimChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range changes {
		ps, exists := m.peers[c.ID]
		now := time.Now()
		if !exists || c.Incarnation > ps.Incarnation {
			if ps == nil {
				ps = &swimPeer{Peer: Peer{ID: c.ID}}
				m.peers[c.ID] = ps
			}
			ps.Status = c.Status
			ps.Incarnation = c.Incarnation
			ps.LastSeen = now
			if c.Status == StatusSuspect {
				ps.SuspectAge = now
			} else {
				ps.SuspectAge = time.Time{}
			}
		}
	}
}

func (m *swimMembership) suspectIfNoProof(id PeerID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	ps, exists := m.peers[id]
	if !exists {
		return false
	}
	if ps.Status != StatusAlive {
		return false
	}
	if time.Since(ps.LastSeen) > m.cfg.ProbeTimeout*2 {
		ps.Status = StatusSuspect
		ps.SuspectAge = time.Now()
		return true
	}
	return false
}

func (m *swimMembership) nextIncarnation(id PeerID, status Status) uint64 {
	if id == m.self.ID && status == StatusAlive {
		m.incarnation++
		return m.incarnation
	}
	ps, exists := m.peers[id]
	if exists {
		return ps.Incarnation + 1
	}
	return 1
}

func (m *swimMembership) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	deadTimeout := m.cfg.ProbeTimeout * time.Duration(m.cfg.ProbeRetries*2)
	now := time.Now()
	for id, ps := range m.peers {
		if id == m.self.ID {
			continue
		}
		if ps.Status == StatusSuspect && now.Sub(ps.SuspectAge) > deadTimeout {
			ps.Status = StatusDead
			ps.Incarnation++
			ps.LastSeen = now
			ps.SuspectAge = time.Time{}
			log.Printf("[membership] marking DEAD after suspicion: %s", id)
			m.enqueueChange(id, StatusDead)
		}
	}
}

// =============== INTERFACE ================
func (m *swimMembership) Self() Peer {
	return m.self
}

func (m *swimMembership) Live() []Peer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Peer
	for _, ps := range m.peers {
		if ps.Status == StatusAlive {
			out = append(out, ps.Peer)
		}
	}
	return out
}

func (m *swimMembership) GetPeer(id PeerID) (Peer, Status, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ps, ok := m.peers[id]
	if !ok {
		return Peer{}, StatusUnknown, false
	}
	return ps.Peer, ps.Status, true
}

func (m *swimMembership) Status() map[PeerID]Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[PeerID]Status)
	for id, ps := range m.peers {
		out[id] = ps.Status
	}
	return out
}

func (m *swimMembership) RegisterEventHandler(handler EventHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventHandlers = append(m.eventHandlers, handler)
}

// For now—stub, you can build cluster broadcasts out later
func (m *swimMembership) Broadcast([]byte) error { return nil }

