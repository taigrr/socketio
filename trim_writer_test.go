package socketio

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTrimWriter(t *testing.T) {
	var inputs []string
	var target string
	var trim string

	test := func() {
		buf := bytes.NewBuffer(nil)
		w := newTrimWriter(buf, trim)
		for _, str := range inputs {
			_, err := w.Write([]byte(str))
			So(err, ShouldBeNil)
		}
		So(buf.String(), ShouldEqual, target)
	}

	Convey("Trim nothing", t, func() {
		inputs = []string{"1234", "2234"}
		target = "12342234"
		trim = ""

		test()
	})

	Convey("Trim something", t, func() {
		trim = "34"

		Convey("Something at the end of final packet", func() {
			inputs = []string{"1234", "2234"}
			target = "123422"

			test()
		})

		Convey("Something at the multiple packets", func() {
			inputs = []string{"1234", "3434"}
			target = "12"

			test()
		})

		Convey("Something in the middle of packets", func() {
			inputs = []string{"1234", "3434", "5678"}
			target = "123434345678"

			test()
		})

		Convey("Something in the middle and end of packets", func() {
			inputs = []string{"1234", "3434", "5678", "9034"}
			target = "12343434567890"

			test()
		})
	})
}

func TestTrimWriterRetainsOwnedSuffix(t *testing.T) {
	var output bytes.Buffer
	writer := newTrimWriter(&output, " \n")
	input := []byte("hello \n")
	if _, err := writer.Write(input); err != nil {
		t.Fatal(err)
	}
	// Writers must not retain the caller's slice: callers may reuse it as soon
	// as Write returns, even when trailing bytes are buffered internally.
	copy(input, "XXXXXXX")
	if _, err := writer.Write([]byte("world\n")); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "hello \nworld"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
