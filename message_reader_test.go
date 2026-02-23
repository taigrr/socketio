package socketio

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMessageReader(t *testing.T) {

	Convey("Read with args", t, func() {
		buf := bufio.NewReader(bytes.NewBufferString(`["message",1]`))
		reader, err := newMessageReader(buf)
		So(err, ShouldBeNil)
		So(reader.Message(), ShouldEqual, "message")
		b, err := io.ReadAll(reader)
		So(err, ShouldBeNil)
		So(string(b), ShouldEqual, "[1]")
	})

	Convey("Read with args, space", t, func() {
		buf := bufio.NewReader(bytes.NewBufferString(`["message"   ,   1]`))
		reader, err := newMessageReader(buf)
		So(err, ShouldBeNil)
		So(reader.Message(), ShouldEqual, "message")
		b, err := io.ReadAll(reader)
		So(err, ShouldBeNil)
		So(string(b), ShouldEqual, "[   1]")
	})

	Convey("Read only message", t, func() {
		buf := bufio.NewReader(bytes.NewBufferString(`["message"]`))
		reader, err := newMessageReader(buf)
		So(err, ShouldBeNil)
		So(reader.Message(), ShouldEqual, "message")
		b, err := io.ReadAll(reader)
		So(err, ShouldBeNil)
		So(string(b), ShouldEqual, "[]")
	})

	Convey("Read only message", t, func() {
		buf := bufio.NewReader(bytes.NewBufferString(`["message"   ]`))
		reader, err := newMessageReader(buf)
		So(err, ShouldBeNil)
		So(reader.Message(), ShouldEqual, "message")
		b, err := io.ReadAll(reader)
		So(err, ShouldBeNil)
		So(string(b), ShouldEqual, "[]")
	})

}
