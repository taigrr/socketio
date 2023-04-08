package socketio

import (
	"bytes"
	"testing"

	"github.com/pschlump/socketio/engineio"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPacketType(t *testing.T) {
	Convey("Type string", t, func() {
		So(Connect, ShouldEqual, 0)
		So(Connect.String(), ShouldEqual, "connect")
		So(Disconnect, ShouldEqual, 1)
		So(Disconnect.String(), ShouldEqual, "disconnect")
		So(Event, ShouldEqual, 2)
		So(Event.String(), ShouldEqual, "event")
		So(Ack, ShouldEqual, 3)
		So(Ack.String(), ShouldEqual, "ack")
		So(Error, ShouldEqual, 4)
		So(Error.String(), ShouldEqual, "error")
		So(BinaryEvent, ShouldEqual, 5)
		So(BinaryEvent.String(), ShouldEqual, "binary_event")
		So(BinaryAck, ShouldEqual, 6)
		So(BinaryAck.String(), ShouldEqual, "binary_ack")
	})
}

func TestParser(t *testing.T) {
	p := packet{}
	var decodeData interface{}
	output := ""
	message := ""

	test := func() {
		saver := &FrameSaver{}
		encoder := newEncoder(saver)
		err := encoder.Encode(p)
		So(err, ShouldBeNil)
		So(len(saver.data), ShouldBeGreaterThan, 0)
		So(saver.data[0].Buffer.String(), ShouldEqual, output)
		So(saver.data[0].Type, ShouldEqual, engineio.MessageText)
		if len(saver.data) > 1 {
			So(saver.data[1].Buffer.String(), ShouldEqual, "data")
			So(saver.data[1].Type, ShouldEqual, engineio.MessageBinary)
		}

		d := packet{Data: decodeData}
		decoder := newDecoder(saver)
		err = decoder.Decode(&d)
		So(err, ShouldBeNil)
		So(d.ID, ShouldEqual, p.ID)
		So(d.NSP, ShouldEqual, p.NSP)
		if decodeData == nil {
			So(d.Data, ShouldBeNil)
		}
		So(decoder.Message(), ShouldEqual, message)
		err = decoder.DecodeData(&d)
		So(err, ShouldBeNil)
		So(d.Type, ShouldEqual, p.Type)
		So(decoder.current, ShouldBeNil)
	}

	Convey("Only type", t, func() {
		p = packet{
			Type: Connect,
			ID:   -1,
		}
		decodeData = nil
		output = "0"
		message = ""

		test()
	})

	Convey("Type and id", t, func() {
		p = packet{
			Type: Event,
			ID:   1,
		}
		decodeData = nil
		output = "21"
		message = ""

		test()
	})

	Convey("Type and namespace", t, func() {
		p = packet{
			Type: Event,
			ID:   -1,
			NSP:  "/abc",
		}
		decodeData = nil
		output = "2/abc"
		message = ""

		test()
	})

	Convey("Type, id and namespace", t, func() {
		p = packet{
			Type: Event,
			ID:   1,
			NSP:  "/abc",
		}
		decodeData = nil
		output = "2/abc,1"
		message = ""

		test()
	})

	Convey("Type, namespace and data", t, func() {
		p = packet{
			Type: Event,
			ID:   -1,
			NSP:  "/abc",
			Data: []interface{}{"numbers", 1, 2, 3},
		}
		var i1, i2, i3 int
		decodeData = &[]interface{}{&i1, &i2, &i3}
		output = "2/abc,[\"numbers\",1,2,3]"
		message = "numbers"

		test()

		So(i1, ShouldEqual, 1)
		So(i2, ShouldEqual, 2)
		So(i3, ShouldEqual, 3)
	})

	Convey("Type, namespace, id and data", t, func() {
		p = packet{
			Type: Event,
			ID:   1,
			NSP:  "/abc",
			Data: []interface{}{"numbers", 1, 2, 3},
		}
		var i1, i2, i3 int
		decodeData = &[]interface{}{&i1, &i2, &i3}
		output = "2/abc,1[\"numbers\",1,2,3]"
		message = "numbers"

		test()

		So(i1, ShouldEqual, 1)
		So(i2, ShouldEqual, 2)
		So(i3, ShouldEqual, 3)
	})

	Convey("Type, namespace, id and data(ack)", t, func() {
		p = packet{
			Type: Ack,
			ID:   1,
			NSP:  "/abc",
			Data: []interface{}{1, 2, 3},
		}
		var i1, i2, i3 int
		decodeData = &[]interface{}{&i1, &i2, &i3}
		output = "3/abc,1[1,2,3]"
		message = ""

		test()

		So(i1, ShouldEqual, 1)
		So(i2, ShouldEqual, 2)
		So(i3, ShouldEqual, 3)
	})

	Convey("Binary type with attachment", t, func() {
		p = packet{
			Type: Event,
			ID:   1,
			NSP:  "/abc",
			Data: []interface{}{"binary", &Attachment{Data: bytes.NewBufferString("data")}},
		}
		buf := bytes.NewBuffer(nil)
		decodeData = &[]interface{}{&Attachment{Data: buf}}
		output = `51-/abc,1["binary",{"_placeholder":true,"num":0}]`
		message = "binary"

		test()

		So(buf.String(), ShouldEqual, "data")
	})
}
