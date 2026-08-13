package transfer

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
)

// DefaultListenAddr is used when ListenConfig.Addr is empty.
const DefaultListenAddr = "127.0.0.1:7900"

// ConnHandler is invoked after a successful handshake. The handler owns closing conn.
type ConnHandler func(ctx context.Context, sess *Session, conn net.Conn)

// ListenConfig configures the TCP transfer acceptor (ADR 23).
type ListenConfig struct {
	Addr     string // host:port; default DefaultListenAddr
	Identity Identity
	Allow    PeerAllowFunc
	OnSession ConnHandler // required for useful accept; if nil, conn is closed after handshake
	Logger   *log.Logger
}

// Listener accepts P2P transfer connections and runs the handshake.
type Listener struct {
	cfg      ListenConfig
	ln       net.Listener
	logger   *log.Logger
	mu       sync.Mutex
	closed   bool
}

// Listen binds TCP and returns a Listener. Call Serve to accept.
func Listen(cfg ListenConfig) (*Listener, error) {
	if err := cfg.Identity.Validate(); err != nil {
		return nil, err
	}
	addr := cfg.Addr
	if addr == "" {
		addr = DefaultListenAddr
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("transfer listen: %w", err)
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	return &Listener{cfg: cfg, ln: ln, logger: logger}, nil
}

// Addr returns the bound address (useful when port was :0).
func (l *Listener) Addr() net.Addr {
	return l.ln.Addr()
}

// Endpoint returns host:port suitable for presence advertisement.
func (l *Listener) Endpoint() string {
	return l.ln.Addr().String()
}

// Close stops accepting new connections.
func (l *Listener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	return l.ln.Close()
}

// Serve accepts connections until ctx is cancelled or Close is called.
func (l *Listener) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()
	for {
		conn, err := l.ln.Accept()
		if err != nil {
			l.mu.Lock()
			closed := l.closed
			l.mu.Unlock()
			if closed || errors.Is(ctx.Err(), context.Canceled) {
				return ctx.Err()
			}
			return err
		}
		go l.handleConn(ctx, conn)
	}
}

func (l *Listener) handleConn(ctx context.Context, conn net.Conn) {
	sess, err := Handshake(conn, l.cfg.Identity, false, l.cfg.Allow)
	if err != nil {
		l.logger.Printf("transfer handshake (inbound): %v", err)
		_ = conn.Close()
		return
	}
	l.logger.Printf("transfer session inbound peer=%s", sess.PeerDeviceID)
	if l.cfg.OnSession == nil {
		_ = conn.Close()
		return
	}
	l.cfg.OnSession(ctx, sess, conn)
}

// Dial connects to addr, runs the handshake as the dialer, and returns the session.
// The caller owns conn and must Close it.
func Dial(ctx context.Context, addr string, id Identity, allow PeerAllowFunc) (*Session, net.Conn, error) {
	if err := id.Validate(); err != nil {
		return nil, nil, err
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("transfer dial: %w", err)
	}
	sess, err := Handshake(conn, id, true, allow)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return sess, conn, nil
}
