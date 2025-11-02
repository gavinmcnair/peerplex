package membership

import "net"

// PeerID is a unique identifier for a cluster member.
type PeerID string

// Peer represents a single node in the cluster.
type Peer struct {
	ID   PeerID            // Unique, stable identity (can be UUID, hostname, etc.)
	Addr string            // Network address (host:port)
	Meta map[string]string // Arbitrary metadata labels (role, region, az, etc.)
}

// IsLocal returns true if this peer is the current node.
func (p Peer) IsLocal(localID PeerID) bool {
	return p.ID == localID
}

// AddrIP returns the parsed IP of the peer address, if valid.
func (p Peer) AddrIP() net.IP {
	host, _, err := net.SplitHostPort(p.Addr)
	if err != nil {
		return nil
	}
	return net.ParseIP(host)
}

