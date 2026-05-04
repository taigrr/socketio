package socketio

import (
	"errors"
	"net/http"
	"testing"
)

type broadcastTestSocket struct {
	id      string
	emitted []string
	emitErr error
}

func (s *broadcastTestSocket) Id() string                             { return s.id }
func (s *broadcastTestSocket) Rooms() []string                        { return nil }
func (s *broadcastTestSocket) Request() *http.Request                 { return nil }
func (s *broadcastTestSocket) On(message string, f interface{}) error { return nil }
func (s *broadcastTestSocket) OnAny(f interface{}) error              { return nil }
func (s *broadcastTestSocket) Emit(message string, args ...interface{}) error {
	s.emitted = append(s.emitted, message)
	return s.emitErr
}
func (s *broadcastTestSocket) Join(room string) error  { return nil }
func (s *broadcastTestSocket) Leave(room string) error { return nil }
func (s *broadcastTestSocket) BroadcastTo(room, message string, args ...interface{}) error {
	return nil
}

func TestBroadcastSendSkipsIgnoredSocket(t *testing.T) {
	adapter := newBroadcastDefault().(*broadcast)
	sender := &broadcastTestSocket{id: "sender"}
	receiver := &broadcastTestSocket{id: "receiver"}

	if err := adapter.Join("room", sender); err != nil {
		t.Fatalf("Join sender: %v", err)
	}
	if err := adapter.Join("room", receiver); err != nil {
		t.Fatalf("Join receiver: %v", err)
	}

	if err := adapter.Send(sender, "room", "update", 1, 2, 3); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if len(sender.emitted) != 0 {
		t.Fatalf("ignored socket received %d messages", len(sender.emitted))
	}
	if len(receiver.emitted) != 1 || receiver.emitted[0] != "update" {
		t.Fatalf("receiver emitted=%v, want [update]", receiver.emitted)
	}
}

func TestBroadcastSendReturnsEmitError(t *testing.T) {
	adapter := newBroadcastDefault().(*broadcast)
	wantErr := errors.New("boom")
	receiver := &broadcastTestSocket{id: "receiver", emitErr: wantErr}

	if err := adapter.Join("room", receiver); err != nil {
		t.Fatalf("Join: %v", err)
	}

	if err := adapter.Send(nil, "room", "update"); !errors.Is(err, wantErr) {
		t.Fatalf("Send error=%v, want %v", err, wantErr)
	}
}
