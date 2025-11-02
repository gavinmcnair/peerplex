package membership

// Persistence abstracts saving/loading membership state (for restart, snapshot, etc.).
type Persistence interface {
	Save(snapshot []byte) error
	Load() ([]byte, error)
}

