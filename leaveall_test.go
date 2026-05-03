package socketio

import (
	"io"
	"net/http"
	"testing"

	"github.com/taigrr/socketio/engineio"
)

type stubConn struct {
	id string
}

func (c *stubConn) Id() string {
	return c.id
}

func (c *stubConn) Request() *http.Request {
	return nil
}

func (c *stubConn) Close() error {
	return nil
}

func (c *stubConn) NextReader() (engineio.MessageType, io.ReadCloser, error) {
	return engineio.MessageText, nil, io.EOF
}

func (c *stubConn) NextWriter(engineio.MessageType) (io.WriteCloser, error) {
	return nil, io.EOF
}

func TestSocketHandlerLeaveAllClearsRooms(t *testing.T) {
	broadcastAdaptor := newBroadcastDefault()
	base := newBaseHandler("", broadcastAdaptor)
	socket := &socket{conn: &stubConn{id: "socket-1"}}
	handler := newSocketHandler(socket, base)
	socket.socketHandler = handler

	if err := handler.Join("alpha"); err != nil {
		t.Fatalf("join alpha: %v", err)
	}
	if err := handler.Join("beta"); err != nil {
		t.Fatalf("join beta: %v", err)
	}

	if got := len(handler.Rooms()); got != 2 {
		t.Fatalf("expected 2 joined rooms before leave all, got %d", got)
	}

	if err := handler.LeaveAll(); err != nil {
		t.Fatalf("leave all: %v", err)
	}

	if got := len(handler.Rooms()); got != 0 {
		t.Fatalf("expected leave all to clear tracked rooms, got %d", got)
	}

	defaultBroadcast, ok := broadcastAdaptor.(*broadcast)
	if !ok {
		t.Fatal("expected default broadcast implementation")
	}
	if rooms, err := defaultBroadcast.NumberOfRooms(""); err != nil {
		t.Fatalf("count rooms: %v", err)
	} else if rooms != 0 {
		t.Fatalf("expected broadcast rooms to be empty after leave all, got %d", rooms)
	}
}
