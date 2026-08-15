package transfer

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
)

const (
	frameJSON  byte = 'J'
	frameChunk byte = 'B'
	maxPlain   = 63 << 10 // leave room for type byte under maxFrameBytes ciphertext
)

// SecureConn encrypts application frames with AES-256-GCM using the handshake session key.
type SecureConn struct {
	conn   net.Conn
	aead   cipher.AEAD
	sendN  uint64
	recvN  uint64
	sendDir byte
	recvDir byte
}

// NewSecureConn wraps conn after a successful Handshake.
func NewSecureConn(conn net.Conn, sess *Session) (*SecureConn, error) {
	if sess == nil || len(sess.SessionKey) != 32 {
		return nil, fmt.Errorf("transfer: invalid session key")
	}
	block, err := aes.NewCipher(sess.SessionKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	sendDir := byte(0) // dialer → listener
	recvDir := byte(1)
	if !sess.Dialer {
		sendDir, recvDir = 1, 0
	}
	return &SecureConn{
		conn:    conn,
		aead:    aead,
		sendDir: sendDir,
		recvDir: recvDir,
	}, nil
}

func (s *SecureConn) nonce(dir byte, n uint64) []byte {
	var out [12]byte
	out[0] = dir
	binary.BigEndian.PutUint64(out[4:], n)
	return out[:]
}

func (s *SecureConn) writeSealed(plain []byte) error {
	if len(plain) > maxPlain+1 {
		return fmt.Errorf("plaintext too large")
	}
	nonce := s.nonce(s.sendDir, s.sendN)
	s.sendN++
	sealed := s.aead.Seal(nil, nonce, plain, nil)
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(sealed)))
	if _, err := s.conn.Write(hdr[:]); err != nil {
		return err
	}
	_, err := s.conn.Write(sealed)
	return err
}

func (s *SecureConn) readSealed() ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(s.conn, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	maxSealed := uint32(maxPlain + 1 + s.aead.Overhead())
	if n == 0 || n > maxSealed {
		return nil, fmt.Errorf("invalid sealed length %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(s.conn, buf); err != nil {
		return nil, err
	}
	nonce := s.nonce(s.recvDir, s.recvN)
	s.recvN++
	plain, err := s.aead.Open(nil, nonce, buf, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plain, nil
}

// WriteJSON sends a sealed JSON control frame.
func (s *SecureConn) WriteJSON(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	plain := make([]byte, 1+len(raw))
	plain[0] = frameJSON
	copy(plain[1:], raw)
	return s.writeSealed(plain)
}

// ReadJSON reads a sealed JSON control frame.
func (s *SecureConn) ReadJSON(v any) error {
	plain, err := s.readSealed()
	if err != nil {
		return err
	}
	if len(plain) < 1 || plain[0] != frameJSON {
		return fmt.Errorf("expected JSON frame")
	}
	return json.Unmarshal(plain[1:], v)
}

// WriteChunk sends a sealed binary data chunk.
func (s *SecureConn) WriteChunk(p []byte) error {
	if len(p) == 0 {
		return nil
	}
	if len(p) > maxPlain {
		return fmt.Errorf("chunk too large")
	}
	plain := make([]byte, 1+len(p))
	plain[0] = frameChunk
	copy(plain[1:], p)
	return s.writeSealed(plain)
}

// ReadChunk reads a sealed binary data chunk.
func (s *SecureConn) ReadChunk() ([]byte, error) {
	plain, err := s.readSealed()
	if err != nil {
		return nil, err
	}
	if len(plain) < 1 || plain[0] != frameChunk {
		return nil, fmt.Errorf("expected chunk frame")
	}
	out := make([]byte, len(plain)-1)
	copy(out, plain[1:])
	return out, nil
}
