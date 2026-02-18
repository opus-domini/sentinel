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

// ---------------------------------------------------------------------------
// Helper: in-memory test connection pair via net.Pipe
// ---------------------------------------------------------------------------

// newTestServerConn creates a *Conn (server side) backed by a net.Pipe.
// The returned net.Conn is the raw "client" side for injecting/reading frames.
func newTestServerConn(t *testing.T) (*Conn, net.Conn) {
	t.Helper()
	sConn, cConn := net.Pipe()
	t.Cleanup(func() {
		_ = sConn.Close()
		_ = cConn.Close()
	})
	return &Conn{
		conn:   sConn,
		reader: bufio.NewReader(sConn),
		writer: bufio.NewWriter(sConn),
	}, cConn
}

// writeMaskedFrame writes a complete masked WebSocket frame with configurable
// FIN bit, RSV bits, opcode, and payload. Supports all payload length encodings.
func writeMaskedFrame(w io.Writer, fin bool, rsv byte, opcode byte, payload []byte) error {
	var b0 byte
	if fin {
		b0 = 0x80
	}
	b0 |= (rsv << 4) & 0x70
	b0 |= opcode & 0x0F

	length := len(payload)
	var header []byte
	switch {
	case length < 126:
		header = []byte{b0, 0x80 | byte(length)}
	case length <= 0xFFFF:
		header = make([]byte, 4)
		header[0] = b0
		header[1] = 0x80 | 126
		binary.BigEndian.PutUint16(header[2:4], uint16(length))
	default:
		header = make([]byte, 10)
		header[0] = b0
		header[1] = 0x80 | 127
		binary.BigEndian.PutUint64(header[2:10], uint64(length))
	}
	if _, err := w.Write(header); err != nil {
		return err
	}

	mask := [4]byte{0x12, 0x34, 0x56, 0x78}
	if _, err := w.Write(mask[:]); err != nil {
		return err
	}

	masked := make([]byte, length)
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}
	_, err := w.Write(masked)
	return err
}

// ---------------------------------------------------------------------------
// Pure function tests (continued)
// ---------------------------------------------------------------------------

func TestFrameError(t *testing.T) {
	t.Parallel()

	fe := &frameError{closeCode: CloseProtocol, msg: "test error message"}
	if fe.Error() != "test error message" {
		t.Errorf("Error() = %q, want %q", fe.Error(), "test error message")
	}
}

func TestSelectSubprotocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		header    string
		preferred []string
		want      string
	}{
		{"nil preferred", "foo, bar", nil, ""},
		{"empty preferred", "foo, bar", []string{}, ""},
		{"empty header", "", []string{"foo"}, ""},
		{"exact match", "foo, bar", []string{"bar"}, "bar"},
		{"first preferred wins", "foo, bar, baz", []string{"baz", "foo"}, "baz"},
		{"case insensitive", "Foo, BAR", []string{"foo"}, "foo"},
		{"no match", "foo, bar", []string{"qux"}, ""},
		{"whitespace in header", "  foo , bar  ", []string{"bar"}, "bar"},
		{"empty preferred entry skipped", "foo", []string{"", "foo"}, "foo"},
		{"whitespace-only preferred entry skipped", "foo", []string{"  ", "foo"}, "foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := selectSubprotocol(tt.header, tt.preferred)
			if got != tt.want {
				t.Errorf("selectSubprotocol(%q, %v) = %q, want %q", tt.header, tt.preferred, got, tt.want)
			}
		})
	}
}

func TestParseSubprotocolList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
		want   []string
	}{
		{"empty", "", nil},
		{"whitespace only", "   ", nil},
		{"single", "foo", []string{"foo"}},
		{"multiple", "foo, bar, baz", []string{"foo", "bar", "baz"}},
		{"trims spaces", "  foo , bar  , baz ", []string{"foo", "bar", "baz"}},
		{"ignores empty parts", "foo,,bar", []string{"foo", "bar"}},
		{"ignores space-only parts", "foo,  ,bar", []string{"foo", "bar"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseSubprotocolList(tt.header)
			if len(got) != len(tt.want) {
				t.Fatalf("parseSubprotocolList(%q) = %v (len %d), want %v (len %d)",
					tt.header, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// writeFrame tests
// ---------------------------------------------------------------------------

func TestWriteFrameWhenClosed(t *testing.T) {
	t.Parallel()

	wsConn, _ := newTestServerConn(t)
	wsConn.closed.Store(true)

	err := wsConn.writeFrame(OpText, []byte("hello"))
	if err != ErrClosed {
		t.Errorf("writeFrame on closed conn: error = %v, want ErrClosed", err)
	}
}

func TestWriteFrameEmptyPayload(t *testing.T) {
	t.Parallel()

	wsConn, rawConn := newTestServerConn(t)

	done := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 2)
		_, _ = io.ReadFull(rawConn, buf)
		done <- buf
	}()

	if err := wsConn.writeFrame(OpText, nil); err != nil {
		t.Fatalf("writeFrame(nil) error = %v", err)
	}

	raw := <-done
	if raw[0] != 0x81 {
		t.Errorf("header[0] = 0x%02x, want 0x81 (FIN+text)", raw[0])
	}
	if raw[1] != 0x00 {
		t.Errorf("header[1] = 0x%02x, want 0x00 (length 0)", raw[1])
	}
}

func TestWriteFramePayloadLengthEncoding(t *testing.T) {
	t.Parallel()

	t.Run("16-bit length", func(t *testing.T) {
		t.Parallel()
		wsConn, rawConn := newTestServerConn(t)

		payload := make([]byte, 200) // 126..65535 → 16-bit extended
		for i := range payload {
			payload[i] = byte(i)
		}

		done := make(chan []byte, 1)
		go func() {
			// 2 header + 2 extended + 200 payload
			buf := make([]byte, 204)
			_, _ = io.ReadFull(rawConn, buf)
			done <- buf
		}()

		if err := wsConn.writeFrame(OpText, payload); err != nil {
			t.Fatalf("writeFrame() error = %v", err)
		}

		raw := <-done
		if raw[0] != 0x81 {
			t.Errorf("header[0] = 0x%02x, want 0x81", raw[0])
		}
		if raw[1] != 126 {
			t.Errorf("header[1] = %d, want 126", raw[1])
		}
		extLen := binary.BigEndian.Uint16(raw[2:4])
		if extLen != 200 {
			t.Errorf("extended length = %d, want 200", extLen)
		}
		for i := range 200 {
			if raw[4+i] != byte(i) {
				t.Errorf("payload[%d] = 0x%02x, want 0x%02x", i, raw[4+i], byte(i))
				break
			}
		}
	})

	t.Run("64-bit length", func(t *testing.T) {
		t.Parallel()
		wsConn, rawConn := newTestServerConn(t)

		payloadLen := 70000 // >65535 → 64-bit extended
		payload := make([]byte, payloadLen)

		done := make(chan []byte, 1)
		go func() {
			// 2 header + 8 extended + payload
			buf := make([]byte, 10+payloadLen)
			_, _ = io.ReadFull(rawConn, buf)
			done <- buf
		}()

		if err := wsConn.writeFrame(OpBinary, payload); err != nil {
			t.Fatalf("writeFrame() error = %v", err)
		}

		raw := <-done
		if raw[0] != 0x82 {
			t.Errorf("header[0] = 0x%02x, want 0x82 (FIN+binary)", raw[0])
		}
		if raw[1] != 127 {
			t.Errorf("header[1] = %d, want 127", raw[1])
		}
		extLen := binary.BigEndian.Uint64(raw[2:10])
		if extLen != uint64(payloadLen) {
			t.Errorf("extended length = %d, want %d", extLen, payloadLen)
		}
	})
}

// ---------------------------------------------------------------------------
// WriteBinary / WritePing tests
// ---------------------------------------------------------------------------

func TestWriteBinary(t *testing.T) {
	t.Parallel()

	wsConn, rawConn := newTestServerConn(t)
	payload := []byte{0x00, 0xFF, 0x42, 0xDE, 0xAD}

	done := make(chan struct{})
	var gotOp byte
	var gotPayload []byte
	go func() {
		defer close(done)
		gotOp, gotPayload, _ = readServerFrame(rawConn)
	}()

	if err := wsConn.WriteBinary(payload); err != nil {
		t.Fatalf("WriteBinary() error = %v", err)
	}

	<-done
	if gotOp != OpBinary {
		t.Errorf("opcode = 0x%x, want 0x%x", gotOp, OpBinary)
	}
	if string(gotPayload) != string(payload) {
		t.Errorf("payload = %v, want %v", gotPayload, payload)
	}
}

func TestWritePing(t *testing.T) {
	t.Parallel()

	t.Run("normal payload", func(t *testing.T) {
		t.Parallel()
		wsConn, rawConn := newTestServerConn(t)

		payload := []byte("ping-data")
		done := make(chan struct{})
		var gotOp byte
		var gotPayload []byte
		go func() {
			defer close(done)
			gotOp, gotPayload, _ = readServerFrame(rawConn)
		}()

		if err := wsConn.WritePing(payload); err != nil {
			t.Fatalf("WritePing() error = %v", err)
		}

		<-done
		if gotOp != opPing {
			t.Errorf("opcode = 0x%x, want 0x%x (ping)", gotOp, opPing)
		}
		if string(gotPayload) != string(payload) {
			t.Errorf("payload = %q, want %q", gotPayload, payload)
		}
	})

	t.Run("truncates payload over 125 bytes", func(t *testing.T) {
		t.Parallel()
		wsConn, rawConn := newTestServerConn(t)

		payload := make([]byte, 200)
		for i := range payload {
			payload[i] = byte(i)
		}

		done := make(chan struct{})
		var gotPayload []byte
		go func() {
			defer close(done)
			_, gotPayload, _ = readServerFrame(rawConn)
		}()

		if err := wsConn.WritePing(payload); err != nil {
			t.Fatalf("WritePing() error = %v", err)
		}

		<-done
		if len(gotPayload) != maxControlFramePayload {
			t.Errorf("payload length = %d, want %d (maxControlFramePayload)", len(gotPayload), maxControlFramePayload)
		}
	})
}

// ---------------------------------------------------------------------------
// readPayloadLength tests
// ---------------------------------------------------------------------------

func TestReadPayloadLength(t *testing.T) {
	t.Parallel()

	t.Run("inline 0-125", func(t *testing.T) {
		t.Parallel()
		wsConn, _ := newTestServerConn(t)

		got, err := wsConn.readPayloadLength(42)
		if err != nil {
			t.Fatalf("readPayloadLength(42) error = %v", err)
		}
		if got != 42 {
			t.Errorf("readPayloadLength(42) = %d, want 42", got)
		}
	})

	t.Run("inline zero", func(t *testing.T) {
		t.Parallel()
		wsConn, _ := newTestServerConn(t)

		got, err := wsConn.readPayloadLength(0)
		if err != nil {
			t.Fatalf("readPayloadLength(0) error = %v", err)
		}
		if got != 0 {
			t.Errorf("readPayloadLength(0) = %d, want 0", got)
		}
	})

	t.Run("16-bit extended", func(t *testing.T) {
		t.Parallel()
		wsConn, rawConn := newTestServerConn(t)

		go func() {
			tmp := make([]byte, 2)
			binary.BigEndian.PutUint16(tmp, 300)
			_, _ = rawConn.Write(tmp)
		}()

		got, err := wsConn.readPayloadLength(126)
		if err != nil {
			t.Fatalf("readPayloadLength(126) error = %v", err)
		}
		if got != 300 {
			t.Errorf("readPayloadLength(126) = %d, want 300", got)
		}
	})

	t.Run("64-bit extended", func(t *testing.T) {
		t.Parallel()
		wsConn, rawConn := newTestServerConn(t)

		go func() {
			tmp := make([]byte, 8)
			binary.BigEndian.PutUint64(tmp, 70000)
			_, _ = rawConn.Write(tmp)
		}()

		got, err := wsConn.readPayloadLength(127)
		if err != nil {
			t.Fatalf("readPayloadLength(127) error = %v", err)
		}
		if got != 70000 {
			t.Errorf("readPayloadLength(127) = %d, want 70000", got)
		}
	})

	t.Run("64-bit overflow rejects high bit", func(t *testing.T) {
		t.Parallel()
		wsConn, rawConn := newTestServerConn(t)

		go func() {
			tmp := make([]byte, 8)
			binary.BigEndian.PutUint64(tmp, 1<<63) // exceeds int64 max
			_, _ = rawConn.Write(tmp)
		}()

		_, err := wsConn.readPayloadLength(127)
		if err == nil {
			t.Fatal("readPayloadLength(127) expected error for overflow, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// ReadMessage edge-case tests
// ---------------------------------------------------------------------------

func TestReadMessageEmptyPayload(t *testing.T) {
	t.Parallel()

	wsConn, rawConn := newTestServerConn(t)
	go func() {
		_ = writeMaskedFrame(rawConn, true, 0, OpText, nil)
	}()

	op, payload, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if op != OpText {
		t.Errorf("opcode = 0x%x, want 0x%x", op, OpText)
	}
	if len(payload) != 0 {
		t.Errorf("payload length = %d, want 0", len(payload))
	}
}

func TestReadMessageMediumPayload(t *testing.T) {
	t.Parallel()

	wsConn, rawConn := newTestServerConn(t)

	payload := make([]byte, 200)
	for i := range payload {
		payload[i] = byte(i)
	}

	go func() {
		_ = writeMaskedFrame(rawConn, true, 0, OpBinary, payload)
	}()

	op, got, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if op != OpBinary {
		t.Errorf("opcode = 0x%x, want 0x%x", op, OpBinary)
	}
	if len(got) != len(payload) {
		t.Fatalf("payload length = %d, want %d", len(got), len(payload))
	}
	for i := range got {
		if got[i] != payload[i] {
			t.Errorf("payload[%d] = 0x%02x, want 0x%02x", i, got[i], payload[i])
			break
		}
	}
}

func TestReadMessageCloseFrame(t *testing.T) {
	t.Parallel()

	wsConn, rawConn := newTestServerConn(t)
	go func() {
		closePayload := make([]byte, 2)
		binary.BigEndian.PutUint16(closePayload, CloseNormal)
		_ = writeMaskedFrame(rawConn, true, 0, opClose, closePayload)
		// Drain the close reply so WriteClose doesn't block.
		_, _, _ = readServerFrame(rawConn)
	}()

	_, _, err := wsConn.ReadMessage()
	if err != ErrClosed {
		t.Errorf("ReadMessage on close frame: error = %v, want ErrClosed", err)
	}
}

func TestReadMessagePongSkipped(t *testing.T) {
	t.Parallel()

	wsConn, rawConn := newTestServerConn(t)
	go func() {
		_ = writeMaskedFrame(rawConn, true, 0, opPong, []byte("pong-data"))
		_ = writeMaskedFrame(rawConn, true, 0, OpText, []byte("hello"))
	}()

	op, payload, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if op != OpText {
		t.Errorf("opcode = 0x%x, want 0x%x", op, OpText)
	}
	if string(payload) != "hello" {
		t.Errorf("payload = %q, want %q", payload, "hello")
	}
}

func TestReadMessagePingAutoReply(t *testing.T) {
	t.Parallel()

	wsConn, rawConn := newTestServerConn(t)

	pongReceived := make(chan []byte, 1)
	go func() {
		_ = writeMaskedFrame(rawConn, true, 0, opPing, []byte("ping"))
		// Read the auto-pong reply.
		op, payload, err := readServerFrame(rawConn)
		if err == nil && op == opPong {
			pongReceived <- payload
		}
		// Send a text frame so ReadMessage returns.
		_ = writeMaskedFrame(rawConn, true, 0, OpText, []byte("after-ping"))
	}()

	op, payload, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if op != OpText || string(payload) != "after-ping" {
		t.Errorf("got (0x%x, %q), want (0x%x, %q)", op, payload, OpText, "after-ping")
	}

	select {
	case pong := <-pongReceived:
		if string(pong) != "ping" {
			t.Errorf("pong payload = %q, want %q", pong, "ping")
		}
	case <-time.After(time.Second):
		t.Error("expected pong reply, timed out")
	}
}

func TestReadMessageUnsupportedOpcode(t *testing.T) {
	t.Parallel()

	wsConn, rawConn := newTestServerConn(t)
	go func() {
		_ = writeMaskedFrame(rawConn, true, 0, 0x3, []byte("data")) // 0x3 is reserved
		_, _, _ = readServerFrame(rawConn)                           // drain close reply
	}()

	_, _, err := wsConn.ReadMessage()
	if err == nil {
		t.Fatal("ReadMessage with unsupported opcode: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported opcode") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "unsupported opcode")
	}
}

func TestReadMessageUnmaskedFrame(t *testing.T) {
	t.Parallel()

	wsConn, rawConn := newTestServerConn(t)
	go func() {
		// Unmasked frame: mask bit not set. Write as single call to avoid pipe deadlock.
		frame := append([]byte{0x81, 0x05}, []byte("hello")...)
		_, _ = rawConn.Write(frame)
		_, _, _ = readServerFrame(rawConn) // drain close reply
	}()

	_, _, err := wsConn.ReadMessage()
	if err == nil {
		t.Fatal("ReadMessage with unmasked frame: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not masked") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not masked")
	}
}

func TestReadMessageRSVBits(t *testing.T) {
	t.Parallel()

	wsConn, rawConn := newTestServerConn(t)
	go func() {
		// RSV1 bit set (0x40).
		_, _ = rawConn.Write([]byte{
			0xC1,          // FIN + RSV1 + text
			0x80,          // masked + length 0
			0, 0, 0, 0,   // mask key
		})
		_, _, _ = readServerFrame(rawConn) // drain close reply
	}()

	_, _, err := wsConn.ReadMessage()
	if err == nil {
		t.Fatal("ReadMessage with RSV bits: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "rsv") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "rsv")
	}
}

func TestReadMessageFragmentedFrame(t *testing.T) {
	t.Parallel()

	wsConn, rawConn := newTestServerConn(t)
	go func() {
		// FIN=0 (fragmented).
		_, _ = rawConn.Write([]byte{
			0x01,          // NO FIN + text
			0x80,          // masked + length 0
			0, 0, 0, 0,   // mask key
		})
		_, _, _ = readServerFrame(rawConn) // drain close reply
	}()

	_, _, err := wsConn.ReadMessage()
	if err == nil {
		t.Fatal("ReadMessage with fragmented frame: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "fragmented") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "fragmented")
	}
}

func TestReadMessageEOF(t *testing.T) {
	t.Parallel()

	wsConn, rawConn := newTestServerConn(t)
	_ = rawConn.Close()

	_, _, err := wsConn.ReadMessage()
	if err != ErrClosed {
		t.Errorf("ReadMessage after EOF: error = %v, want ErrClosed", err)
	}
}

func TestReadMessageFrameTooLarge(t *testing.T) {
	t.Parallel()

	wsConn, rawConn := newTestServerConn(t)
	go func() {
		// Claim 100000-byte payload (> defaultMaxFramePayload=65536) via 64-bit length.
		header := []byte{0x81, 0x80 | 127} // FIN+text, masked+64-bit
		extLen := make([]byte, 8)
		binary.BigEndian.PutUint64(extLen, 100000)
		_, _ = rawConn.Write(append(header, extLen...))
		_, _, _ = readServerFrame(rawConn) // drain close reply
	}()

	_, _, err := wsConn.ReadMessage()
	if err == nil {
		t.Fatal("ReadMessage with too-large frame: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "too large")
	}
}

func TestReadMessageControlFrameTooLarge(t *testing.T) {
	t.Parallel()

	wsConn, rawConn := newTestServerConn(t)
	go func() {
		// Ping with 126-byte payload (> maxControlFramePayload=125) via 16-bit length.
		buf := make([]byte, 4)
		buf[0] = 0x80 | opPing // FIN + ping
		buf[1] = 0x80 | 126    // masked + 16-bit extended
		binary.BigEndian.PutUint16(buf[2:4], 126)
		_, _ = rawConn.Write(buf)
		_, _, _ = readServerFrame(rawConn) // drain close reply
	}()

	_, _, err := wsConn.ReadMessage()
	if err == nil {
		t.Fatal("ReadMessage with too-large control frame: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "control frame too large") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "control frame too large")
	}
}
