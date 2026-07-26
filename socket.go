package socketio

import (
	"net/http"
	"sync"

	"github.com/taigrr/socketio/engineio"
)

// Socket is the socket object of socket.io.
type Socket interface {
	Id() string                                          // Id returns the session id of socket.
	Rooms() []string                                     // Rooms returns the rooms name joined now.
	Request() *http.Request                              // Request returns the first http request when established connection.
	On(message string, f any) error                      // On registers the function f to handle message.
	OnAny(f any) error                                   // Register a function that will get called on any message
	Emit(message string, args ...any) error              // Emit emits the message with given args.
	Join(room string) error                              // Join joins the room.
	Leave(room string) error                             // Leave leaves the room.
	BroadcastTo(room, message string, args ...any) error // BroadcastTo broadcasts the message to the room with given args.
}

type socket struct {
	*socketHandler
	conn      engineio.Conn
	namespace string
	id        int
	writeLock sync.Mutex
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

func (s *socket) Emit(message string, args ...any) error {
	if err := s.socketHandler.Emit(message, args...); err != nil {
		return err
	}
	if message == "disconnect" {
		s.conn.Close()
	}
	return nil
}

// sendPacket serializes writes to the underlying connection so concurrent
// emits (and the read loop's acks) cannot interleave frames. It does not touch
// the socket handler lock, so a slow client write never blocks event dispatch.
func (s *socket) sendPacket(p packet) error {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()
	encoder := newEncoder(s.conn)
	return encoder.Encode(p)
}

func (s *socket) send(args []any) error {
	p := packet{
		Type: Event,
		ID:   -1,
		NSP:  s.namespace,
		Data: args,
	}
	return s.sendPacket(p)
}

func (s *socket) sendConnect() error {
	p := packet{
		Type: Connect,
		ID:   -1,
		NSP:  s.namespace,
	}
	return s.sendPacket(p)
}

// sendAck writes an event packet that expects an acknowledgement. The ack id
// is allocated, the callback registered via register, and the frame written
// while holding writeLock, so the callback is always registered before the
// frame can be observed (and acked) by the peer. On write failure the
// registration is rolled back via unregister. On success commit runs so the
// caller can enforce any outstanding-ack bookkeeping (cap/compaction).
func (s *socket) sendAck(args []any, register func(id int), unregister func(id int), commit func()) error {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()
	id := s.id
	s.id++
	if s.id < 0 {
		s.id = 0
	}
	register(id)
	p := packet{
		Type: Event,
		ID:   id,
		NSP:  s.namespace,
		Data: args,
	}
	encoder := newEncoder(s.conn)
	if err := encoder.Encode(p); err != nil {
		unregister(id)
		return err
	}
	commit()
	return nil
}

func (s *socket) loop() error {
	defer func() {
		s.LeaveAll()
		s.clearAcks()
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
	if err := s.sendPacket(p); err != nil {
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
				if err := s.sendPacket(p); err != nil {
					return err
				}
			}
		case Disconnect:
			return nil
		}
	}
}
