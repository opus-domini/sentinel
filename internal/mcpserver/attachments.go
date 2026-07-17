package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/tmux"
)

const (
	defaultAttachmentTTL = 30 * time.Minute
	maxControlEvents     = 4096
	maxControlLineBytes  = 1024 * 1024
)

var errAttachmentNotFound = errors.New("attachment not found or expired")

// ControlEvent is one ordered event received from tmux control mode.
type ControlEvent struct {
	Seq    int64  `json:"seq"`
	Type   string `json:"type"`
	PaneID string `json:"paneId,omitempty"`
	Data   string `json:"data,omitempty"`
}

// Attachment identifies one MCP lease on a shared tmux control client.
type Attachment struct {
	ID      string
	User    string
	Session string
	Cursor  int64
}

// EventBatch is an incremental control-mode read.
type EventBatch struct {
	Events   []ControlEvent
	Cursor   int64
	Dropped  bool
	Closed   bool
	TimedOut bool
}

type attachmentLease struct {
	id       string
	stream   *controlStream
	lastUsed time.Time
}

type controlStream struct {
	key     string
	user    string
	session string
	cancel  context.CancelFunc
	stdin   io.WriteCloser
	done    chan struct{}

	mu      sync.Mutex
	events  []ControlEvent
	nextSeq int64
	alive   bool
	changed chan struct{}
	refs    int
}

// AttachmentManager owns shared control-mode clients and their MCP leases.
type AttachmentManager struct {
	mu          sync.Mutex
	attachments map[string]*attachmentLease
	streams     map[string]*controlStream
	paneLocks   sync.Map
	ttl         time.Duration
	stop        chan struct{}
	done        chan struct{}
}

// NewAttachmentManager starts an attachment manager with bounded idle leases.
func NewAttachmentManager() *AttachmentManager {
	m := &AttachmentManager{
		attachments: make(map[string]*attachmentLease),
		streams:     make(map[string]*controlStream),
		ttl:         defaultAttachmentTTL,
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}
	go m.sweep()
	return m
}

// Open creates a lease, reusing the native control client for the same user/session.
func (m *AttachmentManager) Open(user, session string) (Attachment, error) {
	if m == nil {
		return Attachment{}, errors.New("attachment manager is unavailable")
	}
	user = strings.TrimSpace(user)
	session = strings.TrimSpace(session)
	if session == "" {
		return Attachment{}, errors.New("session is required")
	}
	key := user + "\x00" + session
	id, err := randomAttachmentID()
	if err != nil {
		return Attachment{}, err
	}

	m.mu.Lock()
	stream := m.streams[key]
	if stream == nil || !stream.isAlive() {
		if stream != nil {
			delete(m.streams, key)
		}
		var err error
		stream, err = startControlStream(key, user, session)
		if err != nil {
			m.mu.Unlock()
			return Attachment{}, err
		}
		m.streams[key] = stream
	}
	stream.refs++
	lease := &attachmentLease{id: id, stream: stream, lastUsed: time.Now()}
	m.attachments[id] = lease
	cursor := stream.cursor()
	m.mu.Unlock()

	return Attachment{ID: id, User: user, Session: session, Cursor: cursor}, nil
}

// Lookup returns and refreshes a live attachment lease.
func (m *AttachmentManager) Lookup(id string) (Attachment, error) {
	if m == nil {
		return Attachment{}, errAttachmentNotFound
	}
	id = strings.TrimSpace(id)
	m.mu.Lock()
	lease := m.attachments[id]
	if lease == nil || time.Since(lease.lastUsed) >= m.ttl {
		var closeStream *controlStream
		if lease != nil {
			closeStream = m.removeLeaseLocked(lease)
		}
		m.mu.Unlock()
		if closeStream != nil {
			closeStream.close()
		}
		return Attachment{}, errAttachmentNotFound
	}
	lease.lastUsed = time.Now()
	stream := lease.stream
	attachment := Attachment{
		ID:      lease.id,
		User:    stream.user,
		Session: stream.session,
		Cursor:  stream.cursor(),
	}
	m.mu.Unlock()
	return attachment, nil
}

// Read waits for matching events after cursor, bounded by timeout.
func (m *AttachmentManager) Read(ctx context.Context, id string, cursor int64, paneID string, timeout time.Duration) (EventBatch, error) {
	if timeout < 0 {
		timeout = 0
	}
	attachment, err := m.Lookup(id)
	if err != nil {
		return EventBatch{}, err
	}
	m.mu.Lock()
	lease := m.attachments[attachment.ID]
	var stream *controlStream
	if lease != nil {
		stream = lease.stream
	}
	m.mu.Unlock()
	if stream == nil {
		return EventBatch{}, errAttachmentNotFound
	}
	return stream.read(ctx, cursor, strings.TrimSpace(paneID), timeout), nil
}

// LockPane serializes interactive writes for one attachment target.
func (m *AttachmentManager) LockPane(id, paneID string) (func(), error) {
	attachment, err := m.Lookup(id)
	if err != nil {
		return nil, err
	}
	key := attachment.User + "\x00" + attachment.Session + "\x00" + strings.TrimSpace(paneID)
	value, _ := m.paneLocks.LoadOrStore(key, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock, nil
}

// Detach releases a lease without touching the tmux session.
func (m *AttachmentManager) Detach(id string) error {
	if m == nil {
		return errAttachmentNotFound
	}
	m.mu.Lock()
	lease := m.attachments[strings.TrimSpace(id)]
	if lease == nil {
		m.mu.Unlock()
		return errAttachmentNotFound
	}
	closeStream := m.removeLeaseLocked(lease)
	m.mu.Unlock()
	if closeStream != nil {
		closeStream.close()
	}
	return nil
}

// Close stops every control client and the lease sweeper.
func (m *AttachmentManager) Close() {
	if m == nil {
		return
	}
	select {
	case <-m.stop:
		return
	default:
		close(m.stop)
	}
	<-m.done

	m.mu.Lock()
	streams := make([]*controlStream, 0, len(m.streams))
	for _, stream := range m.streams {
		streams = append(streams, stream)
	}
	m.attachments = make(map[string]*attachmentLease)
	m.streams = make(map[string]*controlStream)
	m.mu.Unlock()
	for _, stream := range streams {
		stream.close()
	}
}

func (m *AttachmentManager) removeLeaseLocked(lease *attachmentLease) *controlStream {
	delete(m.attachments, lease.id)
	stream := lease.stream
	stream.refs--
	if stream.refs > 0 {
		return nil
	}
	if m.streams[stream.key] == stream {
		delete(m.streams, stream.key)
	}
	return stream
}

func (m *AttachmentManager) sweep() {
	ticker := time.NewTicker(time.Minute)
	defer func() {
		ticker.Stop()
		close(m.done)
	}()
	for {
		select {
		case <-m.stop:
			return
		case now := <-ticker.C:
			var closeStreams []*controlStream
			m.mu.Lock()
			for _, lease := range m.attachments {
				if now.Sub(lease.lastUsed) >= m.ttl {
					if stream := m.removeLeaseLocked(lease); stream != nil {
						closeStreams = append(closeStreams, stream)
					}
				}
			}
			m.mu.Unlock()
			for _, stream := range closeStreams {
				stream.close()
			}
		}
	}
}

func startControlStream(key, user, session string) (*controlStream, error) {
	name, args, err := tmux.BuildControlCommand(user, session)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, name, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open tmux control input: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open tmux control output: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start tmux control client: %w", err)
	}

	stream := &controlStream{
		key:     key,
		user:    user,
		session: session,
		cancel:  cancel,
		stdin:   stdin,
		done:    make(chan struct{}),
		alive:   true,
		changed: make(chan struct{}),
	}
	go stream.scan(stdout)
	go func() {
		waitErr := cmd.Wait()
		stream.mu.Lock()
		stream.alive = false
		message := strings.TrimSpace(stderr.String())
		if message == "" && waitErr != nil && !errors.Is(ctx.Err(), context.Canceled) {
			message = waitErr.Error()
		}
		if message != "" {
			stream.appendLocked(ControlEvent{Type: "exit", Data: message})
		} else {
			stream.signalLocked()
		}
		stream.mu.Unlock()
		close(stream.done)
	}()
	return stream, nil
}

func (s *controlStream) scan(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxControlLineBytes)
	for scanner.Scan() {
		event, ok := parseControlLine(scanner.Text())
		if !ok {
			continue
		}
		s.mu.Lock()
		s.appendLocked(event)
		s.mu.Unlock()
	}
	if err := scanner.Err(); err != nil {
		s.mu.Lock()
		s.appendLocked(ControlEvent{Type: "read-error", Data: err.Error()})
		s.mu.Unlock()
	}
}

func (s *controlStream) appendLocked(event ControlEvent) {
	s.nextSeq++
	event.Seq = s.nextSeq
	if len(s.events) == maxControlEvents {
		copy(s.events, s.events[1:])
		s.events[len(s.events)-1] = event
	} else {
		s.events = append(s.events, event)
	}
	s.signalLocked()
}

func (s *controlStream) signalLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

func (s *controlStream) cursor() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextSeq
}

func (s *controlStream) isAlive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.alive
}

func (s *controlStream) read(ctx context.Context, cursor int64, paneID string, timeout time.Duration) EventBatch {
	if cursor < 0 {
		cursor = 0
	}
	readCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		readCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	observed := cursor
	for {
		s.mu.Lock()
		batch := EventBatch{Cursor: s.nextSeq, Closed: !s.alive}
		if len(s.events) > 0 && cursor < s.events[0].Seq-1 {
			batch.Dropped = true
		}
		for _, event := range s.events {
			if event.Seq <= observed {
				continue
			}
			if paneID == "" || event.PaneID == "" || event.PaneID == paneID {
				batch.Events = append(batch.Events, event)
			}
		}
		observed = s.nextSeq
		changed := s.changed
		s.mu.Unlock()
		if len(batch.Events) > 0 || batch.Closed || timeout == 0 {
			return batch
		}
		select {
		case <-readCtx.Done():
			batch.Cursor = observed
			batch.TimedOut = errors.Is(readCtx.Err(), context.DeadlineExceeded)
			return batch
		case <-changed:
		}
	}
}

func (s *controlStream) close() {
	s.cancel()
	_ = s.stdin.Close()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}

func parseControlLine(line string) (ControlEvent, bool) {
	line = strings.TrimSuffix(line, "\r")
	if line == "" {
		return ControlEvent{}, false
	}
	if !strings.HasPrefix(line, "%") {
		return ControlEvent{Type: "command-output", Data: strings.ToValidUTF8(line, "�")}, true
	}
	parts := strings.SplitN(line, " ", 3)
	eventType := strings.TrimPrefix(parts[0], "%")
	if eventType == "" {
		return ControlEvent{}, false
	}
	event := ControlEvent{Type: eventType}
	if eventType == "output" && len(parts) >= 2 {
		event.PaneID = parts[1]
		if len(parts) == 3 {
			event.Data = decodeControlOutput(parts[2])
		}
		return event, true
	}
	if len(parts) >= 2 {
		event.Data = strings.Join(parts[1:], " ")
	}
	return event, true
}

func decodeControlOutput(value string) string {
	decoded := make([]byte, 0, len(value))
	for i := 0; i < len(value); {
		if value[i] == '\\' && i+3 < len(value) && isOctal(value[i+1]) && isOctal(value[i+2]) && isOctal(value[i+3]) {
			decoded = append(decoded, (value[i+1]-'0')*64+(value[i+2]-'0')*8+(value[i+3]-'0'))
			i += 4
			continue
		}
		decoded = append(decoded, value[i])
		i++
	}
	return strings.ToValidUTF8(string(decoded), "�")
}

func isOctal(value byte) bool {
	return value >= '0' && value <= '7'
}

func randomAttachmentID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate attachment ID: %w", err)
	}
	return "att_" + hex.EncodeToString(raw[:]), nil
}
