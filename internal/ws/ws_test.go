package ws

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Pure function tests
// ---------------------------------------------------------------------------

func TestComputeAcceptKey(t *testing.T) {
	t.Parallel()

	// RFC 6455 §4.2.2 example.
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	got := computeAcceptKey(key)
	if got != want {
		t.Errorf("computeAcceptKey(%q) = %q, want %q", key, got, want)
	}
}

func TestHeaderContainsToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header http.Header
		key    string
		token  string
		want   bool
	}{
		{
			name:   "single value match",
			header: http.Header{"Connection": {"Upgrade"}},
			key:    "Connection",
			token:  "upgrade",
			want:   true,
		},
		{
			name:   "comma-separated case-insensitive",
			header: http.Header{"Connection": {"keep-alive, Upgrade"}},
			key:    "Connection",
			token:  "upgrade",
			want:   true,
		},
		{
			name:   "no match",
			header: http.Header{"Connection": {"keep-alive"}},
			key:    "Connection",
			token:  "upgrade",
			want:   false,
		},
		{
			name:   "empty header",
			header: http.Header{},
			key:    "Connection",
			token:  "upgrade",
			want:   false,
		},
		{
			name: "multiple header values",
			header: func() http.Header {
				h := http.Header{}
				h.Add("Connection", "keep-alive")
				h.Add("Connection", "Upgrade")
				return h
			}(),
			key:   "Connection",
			token: "upgrade",
			want:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := headerContainsToken(tt.header, tt.key, tt.token)
			if got != tt.want {
				t.Errorf("headerContainsToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsControlOpcode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		op   byte
		want bool
	}{
		{"opClose", 0x8, true},
		{"opPing", 0x9, true},
		{"opPong", 0xA, true},
		{"OpText", OpText, false},
		{"OpBinary", OpBinary, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isControlOpcode(tt.op); got != tt.want {
				t.Errorf("isControlOpcode(0x%x) = %v, want %v", tt.op, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration tests
// ---------------------------------------------------------------------------

// writeClientFrame builds a masked WebSocket client frame per RFC 6455.
func writeClientFrame(w io.Writer, opcode byte, payload []byte) error {
	// FIN + opcode
	if _, err := w.Write([]byte{0x80 | opcode}); err != nil {
		return err
	}
	// Mask bit set + payload length (only handles < 126 for test simplicity)
	length := len(payload)
	if length >= 126 {
		return fmt.Errorf("writeClientFrame: payload too large (%d >= 126)", length)
	}
	if _, err := w.Write([]byte{0x80 | byte(length)}); err != nil {
		return err
	}
	// 4 random mask bytes
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}
	if _, err := w.Write(mask); err != nil {
		return err
	}
	// Masked payload
	masked := make([]byte, length)
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}
	_, err := w.Write(masked)
	return err
}

// readServerFrame reads an unmasked server frame.
func readServerFrame(r io.Reader) (byte, []byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	opcode := header[0] & 0x0F
	lengthByte := header[1] & 0x7F

	var payloadLen uint64
	switch {
	case lengthByte < 126:
		payloadLen = uint64(lengthByte)
	case lengthByte == 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = uint64(binary.BigEndian.Uint16(ext[:]))
	case lengthByte == 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = binary.BigEndian.Uint64(ext[:])
	}

	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
	}
	return opcode, payload, nil
}

// setupWSServer starts an httptest.Server that upgrades to WebSocket and
// calls handler with the resulting Conn.
func setupWSServer(t *testing.T, handler func(*Conn)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r, nil)
		if err != nil {
			return
		}
		handler(conn)
	}))
}

// dialWebSocket performs the HTTP upgrade handshake over a raw TCP connection.
func dialWebSocket(t *testing.T, serverURL string) net.Conn {
	t.Helper()

	// Parse host:port from http://host:port
	addr := strings.TrimPrefix(serverURL, "http://")
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("net.Dial(%s) error = %v", addr, err)
	}

	key := base64.StdEncoding.EncodeToString([]byte("test-websocket-key!"))
	req := "GET / HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Connection: Upgrade\r\n" +
		"Upgrade: websocket\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		_ = conn.Close()
		t.Fatalf("write upgrade request error = %v", err)
	}

	// Read response line.
	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		t.Fatalf("read status line error = %v", err)
	}
	if !strings.Contains(statusLine, "101") {
		_ = conn.Close()
		t.Fatalf("expected 101, got: %s", statusLine)
	}
	// Consume remaining headers until blank line.
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			t.Fatalf("read headers error = %v", err)
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}

	// We need to return a conn that includes the buffered reader's data.
	// Wrap conn so reads come from the buffered reader first.
	return &bufferedConn{Conn: conn, reader: reader}
}

// bufferedConn wraps a net.Conn so Read() drains the bufio.Reader first.
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (bc *bufferedConn) Read(p []byte) (int, error) {
	return bc.reader.Read(p)
}

func TestUpgrade(t *testing.T) {
	t.Parallel()

	t.Run("valid handshake", func(t *testing.T) {
		t.Parallel()

		connected := make(chan struct{})
		srv := setupWSServer(t, func(c *Conn) {
			close(connected)
			_ = c.Close()
		})
		defer srv.Close()

		conn := dialWebSocket(t, srv.URL)
		defer func() { _ = conn.Close() }()

		// Wait for server handler to confirm connection.
		select {
		case <-connected:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for connection")
		}
	})

	t.Run("wrong method", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = Upgrade(w, r, nil)
		}))
		defer srv.Close()

		addr := strings.TrimPrefix(srv.URL, "http://")
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			t.Fatalf("dial error = %v", err)
		}
		defer func() { _ = conn.Close() }()

		req := "POST / HTTP/1.1\r\nHost: " + addr + "\r\n\r\n"
		_, _ = conn.Write([]byte(req))

		reader := bufio.NewReader(conn)
		status, _ := reader.ReadString('\n')
		if !strings.Contains(status, "405") {
			t.Errorf("expected 405, got: %s", status)
		}
	})

	t.Run("missing Connection header", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = Upgrade(w, r, nil)
		}))
		defer srv.Close()

		addr := strings.TrimPrefix(srv.URL, "http://")
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			t.Fatalf("dial error = %v", err)
		}
		defer func() { _ = conn.Close() }()

		req := "GET / HTTP/1.1\r\n" +
			"Host: " + addr + "\r\n" +
			"Upgrade: websocket\r\n" +
			"Sec-WebSocket-Version: 13\r\n" +
			"Sec-WebSocket-Key: dGVzdA==\r\n" +
			"\r\n"
		_, _ = conn.Write([]byte(req))

		reader := bufio.NewReader(conn)
		status, _ := reader.ReadString('\n')
		if !strings.Contains(status, "400") {
			t.Errorf("expected 400, got: %s", status)
		}
	})

	t.Run("missing Upgrade header", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = Upgrade(w, r, nil)
		}))
		defer srv.Close()

		addr := strings.TrimPrefix(srv.URL, "http://")
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			t.Fatalf("dial error = %v", err)
		}
		defer func() { _ = conn.Close() }()

		req := "GET / HTTP/1.1\r\n" +
			"Host: " + addr + "\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Version: 13\r\n" +
			"Sec-WebSocket-Key: dGVzdA==\r\n" +
			"\r\n"
		_, _ = conn.Write([]byte(req))

		reader := bufio.NewReader(conn)
		status, _ := reader.ReadString('\n')
		if !strings.Contains(status, "400") {
			t.Errorf("expected 400, got: %s", status)
		}
	})

	t.Run("missing Sec-WebSocket-Key", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = Upgrade(w, r, nil)
		}))
		defer srv.Close()

		addr := strings.TrimPrefix(srv.URL, "http://")
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			t.Fatalf("dial error = %v", err)
		}
		defer func() { _ = conn.Close() }()

		req := "GET / HTTP/1.1\r\n" +
			"Host: " + addr + "\r\n" +
			"Connection: Upgrade\r\n" +
			"Upgrade: websocket\r\n" +
			"Sec-WebSocket-Version: 13\r\n" +
			"\r\n"
		_, _ = conn.Write([]byte(req))

		reader := bufio.NewReader(conn)
		status, _ := reader.ReadString('\n')
		if !strings.Contains(status, "400") {
			t.Errorf("expected 400, got: %s", status)
		}
	})

	t.Run("origin check fails", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = Upgrade(w, r, func(r *http.Request) error {
				return fmt.Errorf("origin denied")
			})
		}))
		defer srv.Close()

		addr := strings.TrimPrefix(srv.URL, "http://")
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			t.Fatalf("dial error = %v", err)
		}
		defer func() { _ = conn.Close() }()

		key := base64.StdEncoding.EncodeToString([]byte("test-key"))
		req := "GET / HTTP/1.1\r\n" +
			"Host: " + addr + "\r\n" +
			"Connection: Upgrade\r\n" +
			"Upgrade: websocket\r\n" +
			"Sec-WebSocket-Version: 13\r\n" +
			"Sec-WebSocket-Key: " + key + "\r\n" +
			"\r\n"
		_, _ = conn.Write([]byte(req))

		reader := bufio.NewReader(conn)
		status, _ := reader.ReadString('\n')
		if !strings.Contains(status, "403") {
			t.Errorf("expected 403, got: %s", status)
		}
	})
}

func TestReadWriteRoundTrip(t *testing.T) {
	t.Parallel()

	message := []byte("hello websocket")
	echoed := make(chan []byte, 1)

	srv := setupWSServer(t, func(c *Conn) {
		defer func() { _ = c.Close() }()
		op, payload, err := c.ReadMessage()
		if err != nil {
			return
		}
		if op != OpText {
			return
		}
		_ = c.WriteText(payload)
	})
	defer srv.Close()

	conn := dialWebSocket(t, srv.URL)
	defer func() { _ = conn.Close() }()

	// Client sends text frame.
	if err := writeClientFrame(conn, OpText, message); err != nil {
		t.Fatalf("writeClientFrame error = %v", err)
	}

	// Client reads server response.
	go func() {
		op, payload, err := readServerFrame(conn)
		if err != nil {
			return
		}
		if op != OpText {
			return
		}
		echoed <- payload
	}()

	select {
	case got := <-echoed:
		if string(got) != string(message) {
			t.Errorf("echoed = %q, want %q", got, message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for echo")
	}
}

func TestPingPong(t *testing.T) {
	t.Parallel()

	pingPayload := []byte("hello")
	pongReceived := make(chan []byte, 1)

	srv := setupWSServer(t, func(c *Conn) {
		defer func() { _ = c.Close() }()
		// ReadMessage auto-responds to pings with pongs.
		_, _, _ = c.ReadMessage()
	})
	defer srv.Close()

	conn := dialWebSocket(t, srv.URL)
	defer func() { _ = conn.Close() }()

	// Client sends ping.
	if err := writeClientFrame(conn, opPing, pingPayload); err != nil {
		t.Fatalf("writeClientFrame(ping) error = %v", err)
	}

	// Read pong response.
	go func() {
		op, payload, err := readServerFrame(conn)
		if err != nil {
			return
		}
		if op == opPong {
			pongReceived <- payload
		}
	}()

	select {
	case got := <-pongReceived:
		if string(got) != string(pingPayload) {
			t.Errorf("pong payload = %q, want %q", got, pingPayload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pong")
	}
}

func TestWriteClose(t *testing.T) {
	t.Parallel()

	closeReceived := make(chan struct{}, 1)
	var gotCode uint16
	var gotReason string

	srv := setupWSServer(t, func(c *Conn) {
		_ = c.WriteClose(1000, "goodbye")
	})
	defer srv.Close()

	conn := dialWebSocket(t, srv.URL)
	defer func() { _ = conn.Close() }()

	go func() {
		op, payload, err := readServerFrame(conn)
		if err != nil {
			return
		}
		if op == opClose && len(payload) >= 2 {
			gotCode = binary.BigEndian.Uint16(payload[:2])
			gotReason = string(payload[2:])
			closeReceived <- struct{}{}
		}
	}()

	select {
	case <-closeReceived:
		if gotCode != 1000 {
			t.Errorf("close code = %d, want 1000", gotCode)
		}
		if gotReason != "goodbye" {
			t.Errorf("close reason = %q, want %q", gotReason, "goodbye")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for close frame")
	}
}
