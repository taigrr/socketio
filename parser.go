package socketio

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"

	"github.com/pschlump/json" //	"encoding/json"
	"github.com/pschlump/socketio/engineio"
)

const Protocol = 4

type packetType int

const (
	Connect packetType = iota
	Disconnect
	Event
	Ack
	Error
	BinaryEvent
	BinaryAck
)

func (t packetType) String() string {
	switch t {
	case Connect:
		return "connect"
	case Disconnect:
		return "disconnect"
	case Event:
		return "event"
	case Ack:
		return "ack"
	case Error:
		return "error"
	case BinaryEvent:
		return "binary_event"
	case BinaryAck:
		return "binary_ack"
	}
	return fmt.Sprintf("unknown(%d)", t)
}

type frameReader interface {
	NextReader() (engineio.MessageType, io.ReadCloser, error)
}

type frameWriter interface {
	NextWriter(engineio.MessageType) (io.WriteCloser, error)
}

type packet struct {
	Type         packetType
	NSP          string
	ID           int
	Data         interface{}
	attachNumber int
}

type encoder struct {
	w   frameWriter
	err error
}

func newEncoder(w frameWriter) *encoder {
	return &encoder{
		w: w,
	}
}

func (e *encoder) Encode(v packet) error {
	attachments := encodeAttachments(v.Data)
	v.attachNumber = len(attachments)
	if v.attachNumber > 0 {
		v.Type += BinaryEvent - Event
	}
	if err := e.encodePacket(v); err != nil {
		return err
	}
	for _, a := range attachments {
		if err := e.writeBinary(a); err != nil {
			return err
		}
	}
	return nil
}

func (e *encoder) encodePacket(v packet) error {
	writer, err := e.w.NextWriter(engineio.MessageText)
	if err != nil {
		return err
	}
	defer writer.Close()

	w := newTrimWriter(writer, "\n")
	wh := newWriterHelper(w)
	wh.Write([]byte{byte(v.Type) + '0'})
	if v.Type == BinaryEvent || v.Type == BinaryAck {
		wh.Write([]byte(fmt.Sprintf("%d-", v.attachNumber)))
	}
	needEnd := false
	if v.NSP != "" {
		wh.Write([]byte(v.NSP))
		needEnd = true
	}
	if v.ID >= 0 {
		f := "%d"
		if needEnd {
			f = ",%d"
			needEnd = false
		}
		wh.Write([]byte(fmt.Sprintf(f, v.ID)))
	}
	if v.Data != nil {
		if needEnd {
			wh.Write([]byte{','})
			needEnd = false
		}
		if wh.Error() != nil {
			return wh.Error()
		}
		encoder := json.NewEncoder(w)
		return encoder.Encode(v.Data)
	}
	return wh.Error()
}

func (e *encoder) writeBinary(r io.Reader) error {
	writer, err := e.w.NextWriter(engineio.MessageBinary)
	if err != nil {
		return err
	}
	defer writer.Close()

	if _, err := io.Copy(writer, r); err != nil {
		return err
	}
	return nil
}

type decoder struct {
	reader        frameReader
	message       string
	current       io.Reader
	currentCloser io.Closer
}

func newDecoder(r frameReader) *decoder {
	return &decoder{
		reader: r,
	}
}

func (d *decoder) Close() {
	if d != nil && d.currentCloser != nil {
		d.currentCloser.Close()
		d.current = nil
		d.currentCloser = nil
	}
}

func (d *decoder) Decode(v *packet) error {
	ty, r, err := d.reader.NextReader()
	if err != nil {
		return err
	}
	if d.current != nil {
		d.Close()
	}
	defer func() {
		if d.current == nil {
			r.Close()
		}
	}()

	if ty != engineio.MessageText {
		return fmt.Errorf("need text package")
	}
	reader := bufio.NewReader(r)

	v.ID = -1

	t, err := reader.ReadByte()
	if err != nil {
		return err
	}
	v.Type = packetType(t - '0')

	if v.Type == BinaryEvent || v.Type == BinaryAck {
		var num []byte
		num, err = reader.ReadBytes('-')
		if err != nil {
			return err
		}
		numLen := len(num)
		if numLen == 0 {
			return fmt.Errorf("invalid packet")
		}
		var n int64
		n, err = strconv.ParseInt(string(num[:numLen-1]), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid packet")
		}
		v.attachNumber = int(n)
	}

	next, err := reader.Peek(1)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	if len(next) == 0 {
		return fmt.Errorf("invalid packet")
	}

	if next[0] == '/' {
		path, err := reader.ReadBytes(',')
		if err != nil && err != io.EOF {
			return err
		}
		pathLen := len(path)
		if pathLen == 0 {
			return fmt.Errorf("invalid packet")
		}
		if err == nil {
			path = path[:pathLen-1]
		}
		v.NSP = string(path)
		if err == io.EOF {
			return nil
		}
	}

	id := bytes.NewBuffer(nil)
	finish := false
	for {
		next, err := reader.Peek(1)
		if err == io.EOF {
			finish = true
			break
		}
		if err != nil {
			return err
		}
		if '0' <= next[0] && next[0] <= '9' {
			if err := id.WriteByte(next[0]); err != nil {
				return err
			}
		} else {
			break
		}
		reader.ReadByte()
	}
	if id.Len() > 0 {
		id, err := strconv.ParseInt(id.String(), 10, 64)
		if err != nil {
			return err
		}
		v.ID = int(id)
	}
	if finish {
		return nil
	}

	switch v.Type {
	case Event:
		fallthrough
	case BinaryEvent:
		msgReader, err := newMessageReader(reader)
		if err != nil {
			return err
		}
		d.message = msgReader.Message()
		d.current = msgReader
		d.currentCloser = r
	case Ack:
		fallthrough
	case BinaryAck:
		d.current = reader
		d.currentCloser = r
	}
	return nil
}

func (d *decoder) Message() string {
	// fmt.Printf("parser.c: Message() >%s<\n", d.message)
	return d.message
}

func (d *decoder) DecodeData(v *packet) error {
	if d.current == nil {
		return nil
	}
	defer func() {
		d.Close()
	}()
	decoder := json.NewDecoder(d.current)
	if err := decoder.Decode(v.Data); err != nil {
		return err
	}
	if v.Type == BinaryEvent || v.Type == BinaryAck {
		binary, err := d.decodeBinary(v.attachNumber)
		if err != nil {
			return err
		}
		if err := decodeAttachments(v.Data, binary); err != nil {
			return err
		}
		v.Type -= BinaryEvent - Event
	}
	return nil
}

func (d *decoder) decodeBinary(num int) ([][]byte, error) {
	ret := make([][]byte, num)
	for i := 0; i < num; i++ {
		d.currentCloser.Close()
		t, r, err := d.reader.NextReader()
		if err != nil {
			return nil, err
		}
		d.currentCloser = r
		if t == engineio.MessageText {
			return nil, fmt.Errorf("need binary")
		}
		b, err := ioutil.ReadAll(r)
		if err != nil {
			return nil, err
		}
		ret[i] = b
	}
	return ret, nil
}
