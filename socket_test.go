package socketio

import (
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/taigrr/socketio/engineio"

	. "github.com/smartystreets/goconvey/convey"
)

type failingConn struct {
	nextWriterErr error
}

func (c *failingConn) Id() string {
	return "test"
}

func (c *failingConn) Request() *http.Request {
	return nil
}

func (c *failingConn) Close() error {
	return nil
}

func (c *failingConn) NextReader() (engineio.MessageType, io.ReadCloser, error) {
	return engineio.MessageText, nil, io.EOF
}

func (c *failingConn) NextWriter(engineio.MessageType) (io.WriteCloser, error) {
	return nil, c.nextWriterErr
}

func TestSendAckRollsBackOnEncoderError(t *testing.T) {
	Convey("sendAck rolls back registration and advances the counter on write error", t, func() {
		wantErr := errors.New("writer unavailable")
		socketConn := &failingConn{nextWriterErr: wantErr}
		serverSocket := newSocket(socketConn, newBaseHandler("", newBroadcastDefault()))

		registered, unregistered := -1, -1
		committed := false
		err := serverSocket.sendAck([]any{"event", "payload"},
			func(id int) { registered = id },
			func(id int) { unregistered = id },
			func() { committed = true },
		)
		So(err, ShouldEqual, wantErr)
		So(serverSocket.id, ShouldEqual, 1)
		So(registered, ShouldEqual, 0)
		So(unregistered, ShouldEqual, 0)
		So(committed, ShouldBeFalse)
	})
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

type discardConn struct{}

func (discardConn) Id() string             { return "test" }
func (discardConn) Request() *http.Request { return nil }
func (discardConn) Close() error           { return nil }
func (discardConn) NextReader() (engineio.MessageType, io.ReadCloser, error) {
	return engineio.MessageText, nil, io.EOF
}

func (discardConn) NextWriter(engineio.MessageType) (io.WriteCloser, error) {
	return nopWriteCloser{io.Discard}, nil
}

func TestOnAckCallbackMayReenterSocket(t *testing.T) {
	// An ack callback that itself performs socket operations (Emit/Join)
	// must not deadlock: onAck must release the handler lock before invoking
	// the callback, and Emit must not hold the handler lock across the write.
	done := make(chan struct{})
	go func() {
		defer close(done)
		serverSocket := newSocket(discardConn{}, newBaseHandler("", newBroadcastDefault()))
		c, err := newCaller(func(so Socket) {
			_ = so.Emit("reentrant")
			_ = so.Join("room")
			_ = so.Rooms()
		})
		if err != nil {
			t.Errorf("newCaller: %v", err)
			return
		}
		serverSocket.acks[7] = c
		if err := serverSocket.onAck(7, newDecoder(nil), &packet{Type: Ack, ID: 7}); err != nil {
			t.Errorf("onAck: %v", err)
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("onAck deadlocked when its callback re-entered the socket")
	}
}
