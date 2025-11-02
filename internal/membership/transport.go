package membership

import (
	"net"
	"time"
)

// For MVP, just use Go's net.Conn and net.Listener directly. Stub interface for future.
type Transport interface {
	Listen(addr string) (net.Listener, error)
	Dial(addr string, timeout time.Duration) (net.Conn, error)
	Close() error
}

// TCPSimpleTransport is an MVP transport; extend for TLS/custom later.
type TCPSimpleTransport struct {
	listener net.Listener
}

func NewTCPSimpleTransport() *TCPSimpleTransport {
	return &TCPSimpleTransport{}
}

func (t *TCPSimpleTransport) Listen(addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	t.listener = ln
	return ln, err
}

func (t *TCPSimpleTransport) Dial(addr string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	return d.Dial("tcp", addr)
}

func (t *TCPSimpleTransport) Close() error {
	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}

