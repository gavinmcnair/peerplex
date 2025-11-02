package membership

// EventType describes the kind of cluster event.
type EventType int

const (
	EventJoin EventType = iota
	EventLeave
	EventSuspect
	EventRecover
	EventUpdate
	EventBroadcast
)

// Event represents a generic cluster event.
type Event struct {
	Type    EventType
	Peer    Peer
	Payload []byte
	Meta    map[string]string
}

// EventHandler allows users to receive notifications.
type EventHandler interface {
	OnEvent(e Event)
}

