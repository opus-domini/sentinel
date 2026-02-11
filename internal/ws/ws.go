package ws

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	OpText   byte = 0x1
	OpBinary byte = 0x2
	opClose  byte = 0x8
	opPing   byte = 0x9
	opPong   byte = 0xA
)

const (
	CloseNormal    = 1000
	CloseGoingAway = 1001
	CloseProtocol  = 1002
	CloseTooLarge  = 1009
	CloseInternal  = 1011
)

const (
	maxControlFramePayload = 125
	defaultMaxFramePayload = 64 * 1024
)

var (
	ErrClosed = errors.New("websocket closed")
)

type Conn struct {
	conn      net.Conn
	reader    *bufio.Reader
	writer    *bufio.Writer
	writeMu   sync.Mutex
	closeOnce sync.Once
	closed    atomic.Bool
}

func Upgrade(w http.ResponseWriter, r *http.Request, originCheck func(*http.Request) error) (*Conn, error) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return nil, fmt.Errorf("method not allowed")
	}
	if originCheck != nil {
		if err := originCheck(r); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return nil, err
		}
	}
	if !headerContainsToken(r.Header, "Connection", "upgrade") {
		http.Error(w, "bad websocket request", http.StatusBadRequest)
		return nil, fmt.Errorf("missing Connection: Upgrade")
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		http.Error(w, "bad websocket request", http.StatusBadRequest)
		return nil, fmt.Errorf("missing Upgrade: websocket")
	}
	if strings.TrimSpace(r.Header.Get("Sec-WebSocket-Version")) != "13" {
		http.Error(w, "unsupported websocket version", http.StatusBadRequest)
		return nil, fmt.Errorf("unsupported websocket version")
	}
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		http.Error(w, "missing websocket key", http.StatusBadRequest)
		return nil, fmt.Errorf("missing Sec-WebSocket-Key")
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket upgrade unsupported", http.StatusInternalServerError)
		return nil, fmt.Errorf("response writer is not a hijacker")
	}

	rawConn, rw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	accept := computeAcceptKey(key)
	if _, err := rw.WriteString(
		"HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + accept + "\r\n" +
			"\r\n",
	); err != nil {
		_ = rawConn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = rawConn.Close()
		return nil, err
	}

	return &Conn{
		conn:   rawConn,
		reader: rw.Reader,
		writer: rw.Writer,
	}, nil
}

func (c *Conn) ReadMessage() (byte, []byte, error) {
	for {
		opcode, payload, err := c.readFrame(defaultMaxFramePayload)
		if err != nil {
			var ferr *frameError
			if errors.As(err, &ferr) {
				_ = c.WriteClose(ferr.closeCode, ferr.Error())
				return 0, nil, err
			}
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return 0, nil, ErrClosed
			}
			return 0, nil, err
		}

		switch opcode {
		case opPing:
			if err := c.WritePong(payload); err != nil {
				return 0, nil, err
			}
		case opPong:
			continue
		case opClose:
			_ = c.WriteClose(CloseNormal, "")
			return 0, nil, ErrClosed
		case OpText, OpBinary:
			return opcode, payload, nil
		default:
			_ = c.WriteClose(CloseProtocol, "unsupported opcode")
			return 0, nil, &frameError{
				closeCode: CloseProtocol,
				msg:       "unsupported opcode",
			}
		}
	}
}

func (c *Conn) WriteText(payload []byte) error {
	return c.writeFrame(OpText, payload)
}

func (c *Conn) WriteBinary(payload []byte) error {
	return c.writeFrame(OpBinary, payload)
}

func (c *Conn) WritePing(payload []byte) error {
	if len(payload) > maxControlFramePayload {
		payload = payload[:maxControlFramePayload]
	}
	return c.writeFrame(opPing, payload)
}

func (c *Conn) WritePong(payload []byte) error {
	if len(payload) > maxControlFramePayload {
		payload = payload[:maxControlFramePayload]
	}
	return c.writeFrame(opPong, payload)
}

func (c *Conn) WriteClose(code int, reason string) error {
	if len(reason) > 123 {
		reason = reason[:123]
	}
	var payload []byte
	if code > 0 {
		payload = make([]byte, 2+len(reason))
		binary.BigEndian.PutUint16(payload[:2], uint16(code))
		copy(payload[2:], reason)
	}
	err := c.writeFrame(opClose, payload)
	_ = c.Close()
	return err
}

func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		err = c.conn.Close()
	})
	return err
}

func (c *Conn) writeFrame(opcode byte, payload []byte) error {
	if c.closed.Load() {
		return ErrClosed
	}
	header := make([]byte, 0, 14)
	header = append(header, 0x80|opcode)
	switch {
	case len(payload) < 126:
		header = append(header, byte(len(payload)))
	case len(payload) <= 0xFFFF:
		header = append(header, 126)
		tmp := make([]byte, 2)
		binary.BigEndian.PutUint16(tmp, uint16(len(payload)))
		header = append(header, tmp...)
	default:
		header = append(header, 127)
		tmp := make([]byte, 8)
		binary.BigEndian.PutUint64(tmp, uint64(len(payload)))
		header = append(header, tmp...)
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := c.writer.Write(header); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := c.writer.Write(payload); err != nil {
			return err
		}
	}
	return c.writer.Flush()
}

func (c *Conn) readFrame(maxPayload int64) (byte, []byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(c.reader, header[:]); err != nil {
		return 0, nil, err
	}

	fin := header[0]&0x80 != 0
	rsv := header[0] & 0x70
	opcode := header[0] & 0x0F
	masked := header[1]&0x80 != 0
	lengthToken := header[1] & 0x7F

	if !fin {
		return 0, nil, &frameError{
			closeCode: CloseProtocol,
			msg:       "fragmented frames are not supported",
		}
	}
	if rsv != 0 {
		return 0, nil, &frameError{
			closeCode: CloseProtocol,
			msg:       "rsv bits are not supported",
		}
	}
	if !masked {
		return 0, nil, &frameError{
			closeCode: CloseProtocol,
			msg:       "client frame is not masked",
		}
	}

	payloadLen, err := c.readPayloadLength(lengthToken)
	if err != nil {
		return 0, nil, err
	}
	if isControlOpcode(opcode) && payloadLen > maxControlFramePayload {
		return 0, nil, &frameError{
			closeCode: CloseProtocol,
			msg:       "control frame too large",
		}
	}
	if payloadLen > maxPayload {
		return 0, nil, &frameError{
			closeCode: CloseTooLarge,
			msg:       "frame payload too large",
		}
	}

	var mask [4]byte
	if _, err := io.ReadFull(c.reader, mask[:]); err != nil {
		return 0, nil, err
	}

	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(c.reader, payload); err != nil {
			return 0, nil, err
		}
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}

	return opcode, payload, nil
}

func (c *Conn) readPayloadLength(lengthToken byte) (int64, error) {
	switch lengthToken {
	case 126:
		var extended [2]byte
		if _, err := io.ReadFull(c.reader, extended[:]); err != nil {
			return 0, err
		}
		return int64(binary.BigEndian.Uint16(extended[:])), nil
	case 127:
		var extended [8]byte
		if _, err := io.ReadFull(c.reader, extended[:]); err != nil {
			return 0, err
		}
		length := binary.BigEndian.Uint64(extended[:])
		if length > (1<<63)-1 {
			return 0, &frameError{
				closeCode: CloseTooLarge,
				msg:       "frame payload too large",
			}
		}
		return int64(length), nil
	default:
		return int64(lengthToken), nil
	}
}

func isControlOpcode(op byte) bool {
	return op == opClose || op == opPing || op == opPong
}

func computeAcceptKey(key string) string {
	h := sha1.New()
	_, _ = h.Write([]byte(key))
	_, _ = h.Write([]byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func headerContainsToken(h http.Header, key, token string) bool {
	for _, value := range h.Values(key) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

type frameError struct {
	closeCode int
	msg       string
}

func (e *frameError) Error() string {
	return e.msg
}
