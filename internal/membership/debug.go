package membership

import (
	"encoding/json"
	"net/http"
)

// DebugHandler provides cluster status for manual ops/test.
func DebugHandler(m Membership) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type info struct {
			Self    Peer
			Members []Peer
			Status  map[PeerID]Status
		}
		data := info{
			Self:    m.Self(),
			Members: m.Live(),
			Status:  m.Status(),
		}
		_ = json.NewEncoder(w).Encode(data)
	}
}

