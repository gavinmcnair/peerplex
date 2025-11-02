package membership

import (
	"sync"
)

// Broadcast allows sending arbitrary messages to the cluster.
type Broadcast interface {
	Send(msg []byte) error
	Subscribe(chan<- []byte)
	Unsubscribe(chan<- []byte)
}

// SimpleBroadcast is a naive, in-memory broadcast channel for MVP/test.
type SimpleBroadcast struct {
	mux      sync.RWMutex
	channels []chan<- []byte
}

func NewSimpleBroadcast() *SimpleBroadcast {
	return &SimpleBroadcast{
		channels: make([]chan<- []byte, 0),
	}
}

func (b *SimpleBroadcast) Send(msg []byte) error {
	b.mux.RLock()
	defer b.mux.RUnlock()
	for _, ch := range b.channels {
		select {
		case ch <- msg:
		default:
			// Drop or log if buffer full.
		}
	}
	return nil
}

func (b *SimpleBroadcast) Subscribe(ch chan<- []byte) {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.channels = append(b.channels, ch)
}

func (b *SimpleBroadcast) Unsubscribe(ch chan<- []byte) {
	b.mux.Lock()
	defer b.mux.Unlock()
	for i, c := range b.channels {
		if c == ch {
			b.channels = append(b.channels[:i], b.channels[i+1:]...)
			break
		}
	}
}

