package socketio

import (
	"errors"
	"io"
	"net/http"
	"testing"

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

func TestSendIDPropagatesEncoderError(t *testing.T) {
	Convey("sendID returns encoder errors and preserves the ack counter", t, func() {
		wantErr := errors.New("writer unavailable")
		socketConn := &failingConn{nextWriterErr: wantErr}
		serverSocket := newSocket(socketConn, newBaseHandler("", newBroadcastDefault()))

		id, err := serverSocket.sendID([]any{"event", "payload"})
		So(id, ShouldEqual, -1)
		So(err, ShouldEqual, wantErr)
		So(serverSocket.id, ShouldEqual, 1)
	})
}
