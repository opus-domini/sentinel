package mcpserver

import (
	"context"
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

type testWriteCloser struct{}

func (testWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (testWriteCloser) Close() error                { return nil }

func newTestControlStream() *controlStream {
	return &controlStream{
		alive:   true,
		changed: make(chan struct{}),
	}
}
