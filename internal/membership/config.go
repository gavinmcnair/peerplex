package membership

import (
	"time"
)

// Config holds configuration for the Membership system.
type Config struct {
	NodeID         string            // Unique stable ID.
	BindAddr       string            // Host:port to bind.
	Seeds          []string          // Bootstrap peers.
	Meta           map[string]string // Custom node metadata.
	GossipInterval time.Duration     // Interval for gossip.
	ProbeTimeout   time.Duration     // Probe timeout.
	ProbeRetries   int               // Retries before suspect.
	Replicas       int               // Replication factor.
	TLSEnabled     bool              // TLS toggle.
	TLSCertFile    string            // Cert path.
	TLSKeyFile     string            // Key path.
	LogLevel       string            // Log level.
}

// DefaultConfig returns standard config values.
func DefaultConfig() *Config {
	return &Config{
		NodeID:         "",
		BindAddr:       ":9700",
		Seeds:          nil,
		Meta:           make(map[string]string),
		GossipInterval: 1 * time.Second,
		ProbeTimeout:   500 * time.Millisecond,
		ProbeRetries:   3,
		Replicas:       3,
		TLSEnabled:     false,
	}
}

