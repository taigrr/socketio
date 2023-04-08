package socketio

import (
	"net/http"

	"github.com/pschlump/socketio/engineio"
)

// Socket is the socket object of socket.io.
type Socket interface {
	Id() string                                                  // Id returns the session id of socket.
	Rooms() []string                                             // Rooms returns the rooms name joined now.
	Request() *http.Request                                      // Request returns the first http request when established connection.
	On(message string, f interface{}) error                      // On registers the function f to handle message.
	OnAny(f interface{}) error                                   // Register a function that will get called on any message
	Emit(message string, args ...interface{}) error              // Emit emits the message with given args.
	Join(room string) error                                      // Join joins the room.
	Leave(room string) error                                     // Leave leaves the room.
	BroadcastTo(room, message string, args ...interface{}) error // BroadcastTo broadcasts the message to the room with given args.
}

type socket struct {
	*socketHandler
	conn      engineio.Conn
	namespace string
	id        int
}

func newSocket(conn engineio.Conn, base *baseHandler) *socket {
	ret := &socket{
		conn: conn,
	}
	ret.socketHandler = newSocketHandler(ret, base)
	return ret
}

func (s *socket) Id() string {
	return s.conn.Id()
}

func (s *socket) Request() *http.Request {
	return s.conn.Request()
}

func (s *socket) Emit(message string, args ...interface{}) error {
	if err := s.socketHandler.Emit(message, args...); err != nil {
		return err
	}
	if message == "disconnect" {
		s.conn.Close()
	}
	return nil
}

func (s *socket) send(args []interface{}) error {
	p := packet{
		Type: Event,
		ID:   -1,
		NSP:  s.namespace,
		Data: args,
	}
	encoder := newEncoder(s.conn)
	return encoder.Encode(p)
}

func (s *socket) sendConnect() error {
	p := packet{
		Type: Connect,
		ID:   -1,
		NSP:  s.namespace,
	}
	encoder := newEncoder(s.conn)
	return encoder.Encode(p)
}

func (s *socket) sendID(args []interface{}) (int, error) {
	p := packet{
		Type: Event,
		ID:   s.id,
		NSP:  s.namespace,
		Data: args,
	}
	s.id++
	if s.id < 0 {
		s.id = 0
	}
	encoder := newEncoder(s.conn)
	err := encoder.Encode(p)
	if err != nil {
		return -1, nil
	}
	return p.ID, nil
}

func (s *socket) loop() error {
	defer func() {
		s.LeaveAll()
		p := packet{
			Type: Disconnect,
			ID:   -1,
		}
		s.onPacket(nil, &p)
	}()

	p := packet{
		Type: Connect,
		ID:   -1,
	}
	encoder := newEncoder(s.conn)
	if err := encoder.Encode(p); err != nil {
		return err
	}
	s.onPacket(nil, &p)
	for {
		decoder := newDecoder(s.conn)
		var p packet
		if err := decoder.Decode(&p); err != nil {
			return err
		}
		ret, err := s.onPacket(decoder, &p)
		if err != nil {
			return err
		}
		switch p.Type {
		case Connect:
			s.namespace = p.NSP
			s.sendConnect()
		case BinaryEvent:
			fallthrough
		case Event:
			if p.ID >= 0 {
				p = packet{
					Type: Ack,
					ID:   p.ID,
					NSP:  s.namespace,
					Data: ret,
				}
				encoder := newEncoder(s.conn)
				if err := encoder.Encode(p); err != nil {
					return err
				}
			}
		case Disconnect:
			return nil
		}
	}
}
