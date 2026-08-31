package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseControlLineDecodesPaneOutput(t *testing.T) {
	event, ok := parseControlLine(`%output %12 hello\040world\015\012`)
	if !ok {
		t.Fatal("parseControlLine() rejected output event")
	}
	if event.Type != "output" || event.PaneID != "%12" || event.Data != "hello world\r\n" {
		t.Fatalf("parseControlLine() = %#v", event)
	}
}

func TestParseControlLineKeepsControlEvents(t *testing.T) {
	event, ok := parseControlLine("%window-add @3")
	if !ok {
		t.Fatal("parseControlLine() rejected control event")
	}
	if event.Type != "window-add" || event.Data != "@3" {
		t.Fatalf("parseControlLine() = %#v", event)
	}
}

func TestControlStreamReadUsesCursorAndPaneFilter(t *testing.T) {
	stream := newTestControlStream()
	stream.mu.Lock()
	stream.appendLocked(ControlEvent{Type: "output", PaneID: "%1", Data: "one"})
	stream.appendLocked(ControlEvent{Type: "output", PaneID: "%2", Data: "two"})
	stream.appendLocked(ControlEvent{Type: "window-add", Data: "@2"})
	stream.mu.Unlock()

	batch := stream.read(context.Background(), 0, "%1", 0)
	if batch.Cursor != 3 {
		t.Fatalf("cursor = %d, want 3", batch.Cursor)
	}
	if len(batch.Events) != 2 || batch.Events[0].Data != "one" || batch.Events[1].Type != "window-add" {
		t.Fatalf("events = %#v", batch.Events)
	}

	next := stream.read(context.Background(), batch.Cursor, "%1", 2*time.Millisecond)
	if !next.TimedOut || len(next.Events) != 0 || next.Cursor != batch.Cursor {
		t.Fatalf("next batch = %#v", next)
	}
}

func TestControlStreamReportsDroppedEvents(t *testing.T) {
	stream := newTestControlStream()
	stream.mu.Lock()
	for range maxControlEvents + 1 {
		stream.appendLocked(ControlEvent{Type: "output", PaneID: "%1"})
	}
	stream.mu.Unlock()

	batch := stream.read(context.Background(), 0, "%1", 0)
	if !batch.Dropped {
		t.Fatal("read did not report overwritten events")
	}
	if len(batch.Events) != maxControlEvents {
		t.Fatalf("events = %d, want %d", len(batch.Events), maxControlEvents)
	}
}

func TestRemovingOldLeaseDoesNotRemoveReplacementStream(t *testing.T) {
	manager := &AttachmentManager{
		attachments: make(map[string]*attachmentLease),
		streams:     make(map[string]*controlStream),
	}
	oldStream := newTestControlStream()
	oldStream.key = "\x00dev"
	oldStream.refs = 1
	newStream := newTestControlStream()
	newStream.key = oldStream.key
	manager.streams[newStream.key] = newStream
	lease := &attachmentLease{id: "old", stream: oldStream}
	manager.attachments[lease.id] = lease

	closed := manager.removeLeaseLocked(lease)
	if closed != oldStream {
		t.Fatalf("removeLeaseLocked() = %p, want old stream %p", closed, oldStream)
	}
	if manager.streams[newStream.key] != newStream {
		t.Fatal("removing an old lease deleted its replacement stream")
	}
}

func TestDetachSessionRemovesOnlyMatchingAttachments(t *testing.T) {
	manager := &AttachmentManager{
		attachments: make(map[string]*attachmentLease),
		streams:     make(map[string]*controlStream),
	}
	target := newTestControlStream()
	target.key = "deploy\x00agent"
	target.user = "deploy"
	target.session = "agent"
	target.refs = 2
	target.done = make(chan struct{})
	close(target.done)
	target.cancel = func() {}
	target.stdin = testWriteCloser{}
	other := newTestControlStream()
	other.key = "deploy\x00other"
	other.user = "deploy"
	other.session = "other"
	other.refs = 1
	manager.streams[target.key] = target
	manager.streams[other.key] = other
	manager.attachments["one"] = &attachmentLease{id: "one", stream: target}
	manager.attachments["two"] = &attachmentLease{id: "two", stream: target}
	manager.attachments["other"] = &attachmentLease{id: "other", stream: other}

	manager.DetachSession("deploy", "agent")

	if _, ok := manager.attachments["one"]; ok {
		t.Fatal("first target attachment remains")
	}
	if _, ok := manager.attachments["two"]; ok {
		t.Fatal("second target attachment remains")
	}
	if manager.attachments["other"] == nil || manager.streams[other.key] != other {
		t.Fatal("unrelated attachment was removed")
	}
	if _, ok := manager.streams[target.key]; ok {
		t.Fatal("target control stream remains")
	}
}

func TestControlStreamDrainsOutputBeforeReapingTheProcess(t *testing.T) {
	stream := newTestControlStream()
	stream.done = make(chan struct{})

	drained := -1
	stream.pump(strings.NewReader("%output %1 one\n%output %1 two\n"), func() string {
		stream.mu.Lock()
		defer stream.mu.Unlock()
		drained = len(stream.events)
		return ""
	})

	<-stream.done
	if drained != 2 {
		t.Fatalf("events recorded before the process was reaped = %d, want 2", drained)
	}
	if stream.isAlive() {
		t.Fatal("stream is still alive after the process was reaped")
	}
	batch := stream.read(context.Background(), 0, "%1", 0)
	if len(batch.Events) != 2 || !batch.Closed {
		t.Fatalf("batch = %#v", batch)
	}
}

func TestLockPaneTakesTheCursorAfterTheLockIsHeld(t *testing.T) {
	stream := newTestControlStream()
	stream.key = "\x00dev"
	stream.session = "dev"
	manager := &AttachmentManager{
		attachments: make(map[string]*attachmentLease),
		streams:     map[string]*controlStream{stream.key: stream},
		ttl:         time.Hour,
	}
	lease := &attachmentLease{id: "att", stream: stream, lastUsed: time.Now()}
	manager.attachments[lease.id] = lease

	_, unlock, err := manager.LockPane(lease.id, "%1")
	if err != nil {
		t.Fatalf("LockPane() error = %v", err)
	}

	manager.mu.Lock()
	lease.lastUsed = time.Now().Add(-time.Minute)
	before := lease.lastUsed
	manager.mu.Unlock()

	type grant struct {
		attachment Attachment
		release    func()
		err        error
	}
	granted := make(chan grant, 1)
	go func() {
		attachment, release, err := manager.LockPane(lease.id, "%1")
		granted <- grant{attachment: attachment, release: release, err: err}
	}()

	// The contending call refreshes the lease before it can reach the pane
	// lock, so once lastUsed moves it is blocked on the lock and nothing but
	// the lock can still change the cursor it reports.
	deadline := time.Now().Add(2 * time.Second)
	for {
		manager.mu.Lock()
		lookedUp := lease.lastUsed.After(before)
		manager.mu.Unlock()
		if lookedUp {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("contending LockPane() never looked the lease up")
		}
		time.Sleep(time.Millisecond)
	}

	stream.mu.Lock()
	stream.appendLocked(ControlEvent{Type: "output", PaneID: "%1", Data: "first client output"})
	stream.mu.Unlock()
	unlock()

	result := <-granted
	if result.err != nil {
		t.Fatalf("contending LockPane() error = %v", result.err)
	}
	defer result.release()
	if result.attachment.Cursor != 1 {
		t.Fatalf("cursor = %d, want 1 (the cursor observed once the lock was granted)", result.attachment.Cursor)
	}
}

func TestAttachmentManagerCloseHonorsTheShutdownBudget(t *testing.T) {
	manager := &AttachmentManager{
		attachments: make(map[string]*attachmentLease),
		streams:     make(map[string]*controlStream),
		ttl:         time.Hour,
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}
	go manager.sweep()
	for index := range 3 {
		stream := newTestControlStream()
		stream.key = fmt.Sprintf("\x00session-%d", index)
		// The control client never exits, so every close waits its full cap.
		stream.done = make(chan struct{})
		stream.cancel = func() {}
		stream.stdin = testWriteCloser{}
		manager.streams[stream.key] = stream
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	manager.Close(ctx)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Close() took %s, want it bounded by the caller's shutdown budget", elapsed)
	}
	if len(manager.streams) != 0 {
		t.Fatal("Close() left control streams registered")
	}
}

type testWriteCloser struct{}

func (testWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (testWriteCloser) Close() error                { return nil }

func newTestControlStream() *controlStream {
	return &controlStream{
		alive:   true,
		changed: make(chan struct{}),
	}
}
