package socketio

import (
	"errors"
	"net/http"
	"slices"
	"testing"
)

type broadcastTestSocket struct {
	id      string
	emitted []string
	emitErr error
}

func (s *broadcastTestSocket) Id() string                     { return s.id }
func (s *broadcastTestSocket) Rooms() []string                { return nil }
func (s *broadcastTestSocket) Request() *http.Request         { return nil }
func (s *broadcastTestSocket) On(message string, f any) error { return nil }
func (s *broadcastTestSocket) OnAny(f any) error              { return nil }
func (s *broadcastTestSocket) Emit(message string, args ...any) error {
	s.emitted = append(s.emitted, message)
	return s.emitErr
}
func (s *broadcastTestSocket) Join(room string) error  { return nil }
func (s *broadcastTestSocket) Leave(room string) error { return nil }
func (s *broadcastTestSocket) BroadcastTo(room, message string, args ...any) error {
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

func TestBroadcastRoomAccounting(t *testing.T) {
	adapter := newBroadcastDefault().(*broadcast)
	alphaOne := &broadcastTestSocket{id: "alpha-1"}
	alphaTwo := &broadcastTestSocket{id: "alpha-2"}
	betaOne := &broadcastTestSocket{id: "beta-1"}

	for room, socket := range map[string]*broadcastTestSocket{
		"alpha": alphaOne,
		"beta":  betaOne,
	} {
		if err := adapter.Join(room, socket); err != nil {
			t.Fatalf("Join %s: %v", room, err)
		}
	}
	if err := adapter.Join("alpha", alphaTwo); err != nil {
		t.Fatalf("Join alpha second socket: %v", err)
	}

	count, err := adapter.NumberInRoom("alpha")
	if err != nil {
		t.Fatalf("NumberInRoom alpha: %v", err)
	}
	if count != 2 {
		t.Fatalf("NumberInRoom alpha=%d, want 2", count)
	}

	rooms, err := adapter.ListOfRooms("")
	if err != nil {
		t.Fatalf("ListOfRooms: %v", err)
	}
	slices.Sort(rooms)
	if !slices.Equal(rooms, []string{"alpha", "beta"}) {
		t.Fatalf("ListOfRooms=%v, want [alpha beta]", rooms)
	}

	roomCount, err := adapter.NumberOfRooms("")
	if err != nil {
		t.Fatalf("NumberOfRooms: %v", err)
	}
	if roomCount != 2 {
		t.Fatalf("NumberOfRooms=%d, want 2", roomCount)
	}

	if err := adapter.Leave("alpha", alphaOne); err != nil {
		t.Fatalf("Leave alpha first socket: %v", err)
	}
	count, err = adapter.NumberInRoom("alpha")
	if err != nil {
		t.Fatalf("NumberInRoom alpha after first leave: %v", err)
	}
	if count != 1 {
		t.Fatalf("NumberInRoom alpha after first leave=%d, want 1", count)
	}

	if err := adapter.Leave("alpha", alphaTwo); err != nil {
		t.Fatalf("Leave alpha second socket: %v", err)
	}
	count, err = adapter.NumberInRoom("alpha")
	if err != nil {
		t.Fatalf("NumberInRoom alpha after emptying room: %v", err)
	}
	if count != 0 {
		t.Fatalf("NumberInRoom alpha after emptying room=%d, want 0", count)
	}

	rooms, err = adapter.ListOfRooms("")
	if err != nil {
		t.Fatalf("ListOfRooms after emptying alpha: %v", err)
	}
	if len(rooms) != 1 || rooms[0] != "beta" {
		t.Fatalf("ListOfRooms after emptying alpha=%v, want [beta]", rooms)
	}
}
