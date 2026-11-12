package socketio

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/taigrr/socketio/engineio"
)

type closeErrorFrame struct {
	bytes.Buffer
	writeErr error
	closeErr error
	closes   int
}

func (frame *closeErrorFrame) Write(data []byte) (int, error) {
	if frame.writeErr != nil {
		return 0, frame.writeErr
	}
	return frame.Buffer.Write(data)
}

func (frame *closeErrorFrame) Close() error {
	frame.closes++
	return frame.closeErr
}

type closeErrorFrames struct {
	frames []*closeErrorFrame
	calls  int
}

func (frames *closeErrorFrames) NextWriter(engineio.MessageType) (io.WriteCloser, error) {
	if frames.calls >= len(frames.frames) {
		return nil, errors.New("unexpected frame")
	}
	frame := frames.frames[frames.calls]
	frames.calls++
	return frame, nil
}

func TestEncoderFrameErrors(t *testing.T) {
	writeErr := errors.New("write failed")
	closeErr := errors.New("close failed")
	for _, test := range []struct {
		name     string
		data     any
		binary   bool
		writeErr error
		closeErr error
		want     error
	}{
		{name: "text close", closeErr: closeErr, want: closeErr},
		{name: "JSON close", data: []string{"event"}, closeErr: closeErr, want: closeErr},
		{name: "text write and close", writeErr: writeErr, closeErr: closeErr, want: writeErr},
		{name: "JSON write and close", data: []string{"event"}, writeErr: writeErr, closeErr: closeErr, want: writeErr},
		{name: "binary close", binary: true, closeErr: closeErr, want: closeErr},
		{name: "binary write and close", binary: true, writeErr: writeErr, closeErr: closeErr, want: writeErr},
		{name: "text success"},
		{name: "binary success", binary: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			frame := &closeErrorFrame{writeErr: test.writeErr, closeErr: test.closeErr}
			frames := &closeErrorFrames{frames: []*closeErrorFrame{frame}}
			encoder := newEncoder(frames)
			var err error
			if test.binary {
				err = encoder.writeBinary(bytes.NewBufferString("attachment"))
			} else {
				err = encoder.encodePacket(packet{Type: Event, ID: -1, Data: test.data})
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if frame.closes != 1 {
				t.Fatalf("Close calls = %d, want 1", frame.closes)
			}
		})
	}
}

func TestEncoderStopsAfterFrameCloseError(t *testing.T) {
	closeErr := errors.New("close failed")
	for _, failingFrame := range []int{0, 1} {
		t.Run([]string{"text", "binary"}[failingFrame], func(t *testing.T) {
			frames := &closeErrorFrames{frames: []*closeErrorFrame{{}, {}, {}}}
			frames.frames[failingFrame].closeErr = closeErr
			data := []*Attachment{{Data: bytes.NewBufferString("first")}, {Data: bytes.NewBufferString("second")}}
			err := newEncoder(frames).Encode(packet{Type: Event, ID: -1, Data: data})
			if !errors.Is(err, closeErr) {
				t.Fatalf("Encode error = %v, want %v", err, closeErr)
			}
			if frames.calls != failingFrame+1 {
				t.Fatalf("frames opened = %d, want %d", frames.calls, failingFrame+1)
			}
		})
	}
}
