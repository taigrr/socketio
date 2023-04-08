package socketio

import (
	"fmt"
	"log"
	"reflect"
	"sync"
	// "github.com/sirupsen/logrus"
)

// PJS - could have it return more than just an error, if "rmsg" and "rbody" - then emit response?
// that makes it more like a RPC - call a func get back a response
type EventHandlerFunc func(so *Socket, message string, args [][]byte) error

type baseHandler struct {
	events     map[string]*caller
	allEvents  []*caller
	xEvents    map[string]EventHandlerFunc
	xAllEvents []EventHandlerFunc
	name       string
	broadcast  BroadcastAdaptor
	lock       sync.RWMutex
}

func newBaseHandler(name string, broadcast BroadcastAdaptor) *baseHandler {
	return &baseHandler{
		events:    make(map[string]*caller),
		allEvents: make([]*caller, 0, 5),
		name:      name,
		broadcast: broadcast,
	}
}

// On registers the function f to handle message.
func (h *baseHandler) On(message string, f interface{}) error {
	c, err := newCaller(f)
	if err != nil {
		return err
	}
	h.lock.Lock()
	h.events[message] = c
	h.lock.Unlock()
	return nil
}

func (h *baseHandler) Handle(message string, f EventHandlerFunc) error {
	h.lock.Lock()
	h.xEvents[message] = f
	h.lock.Unlock()
	return nil
}

func (h *baseHandler) HandleAny(f EventHandlerFunc) error {
	h.lock.Lock()
	h.xAllEvents = append(h.xAllEvents, f)
	h.lock.Unlock()
	return nil
}

// On registers the function f to handle ANY message.
func (h *baseHandler) OnAny(f interface{}) error {
	c, err := newCaller(f)
	if err != nil {
		return err
	}
	h.lock.Lock()
	h.allEvents = append(h.allEvents, c)
	h.lock.Unlock()
	return nil
}

func (h *baseHandler) PrintEventsRespondedTo() {
	fmt.Printf("\tEvents:[")
	com := ""
	for i := range h.events {
		fmt.Printf("%s%s", com, i)
		com = ", "
	}
	fmt.Printf(" ] AllEvents = %d", len(h.allEvents))
	fmt.Printf("\n")
}

type socketHandler struct {
	*baseHandler
	acks   map[int]*caller
	socket *socket
	rooms  map[string]struct{}
}

func newSocketHandler(s *socket, base *baseHandler) *socketHandler {
	events := make(map[string]*caller)
	allEvents := make([]*caller, 0, 5)
	xEvents := make(map[string]EventHandlerFunc)
	xAllEvents := make([]EventHandlerFunc, 0, 5)
	base.lock.Lock()
	for k, v := range base.events {
		events[k] = v
	}
	base.lock.Unlock()
	return &socketHandler{
		baseHandler: &baseHandler{
			events:     events,
			allEvents:  allEvents,
			xEvents:    xEvents,
			xAllEvents: xAllEvents,
			broadcast:  base.broadcast,
		},
		acks:   make(map[int]*caller),
		socket: s,
		rooms:  make(map[string]struct{}),
	}
}

func (h *socketHandler) Emit(message string, args ...interface{}) error {
	var c *caller
	if l := len(args); l > 0 {
		fv := reflect.ValueOf(args[l-1])
		if fv.Kind() == reflect.Func {
			var err error
			c, err = newCaller(args[l-1])
			if err != nil {
				return err
			}
			args = args[:l-1]
		}
	}
	args = append([]interface{}{message}, args...)
	h.lock.Lock()
	defer h.lock.Unlock()
	if c != nil {
		id, err := h.socket.sendID(args)
		if err != nil {
			return err
		}
		h.acks[id] = c
		return nil
	}
	return h.socket.send(args)
}

func (h *socketHandler) Rooms() []string {
	h.lock.RLock()
	defer h.lock.RUnlock()
	ret := make([]string, len(h.rooms))
	i := 0
	for room := range h.rooms {
		ret[i] = room
		i++
	}
	return ret
}

func (h *socketHandler) Join(room string) error {
	if err := h.broadcast.Join(h.broadcastName(room), h.socket); err != nil {
		return err
	}
	h.lock.Lock()
	h.rooms[room] = struct{}{}
	h.lock.Unlock()
	return nil
}

func (h *socketHandler) Leave(room string) error {
	if err := h.broadcast.Leave(h.broadcastName(room), h.socket); err != nil {
		return err
	}
	h.lock.Lock()
	delete(h.rooms, room)
	h.lock.Unlock()
	return nil
}

func (h *socketHandler) LeaveAll() error {
	h.lock.RLock()
	tmp := h.rooms
	h.lock.RUnlock()
	for room := range tmp {
		if err := h.broadcast.Leave(h.broadcastName(room), h.socket); err != nil {
			return err
		}
	}
	return nil
}

func (h *baseHandler) BroadcastTo(room, message string, args ...interface{}) error {
	return h.broadcast.Send(nil, h.broadcastName(room), message, args...)
}

func (h *socketHandler) BroadcastTo(room, message string, args ...interface{}) error {
	return h.broadcast.Send(h.socket, h.broadcastName(room), message, args...)
}

func (h *baseHandler) broadcastName(room string) string {
	return fmt.Sprintf("%s:%s", h.name, room)
}

func (h *socketHandler) onPacket(decoder *decoder, packet *packet) ([]interface{}, error) {
	var message string
	switch packet.Type {
	case Connect:
		message = "connection"
	case Disconnect:
		message = "disconnect"
	case Error:
		message = "error"
	case Ack:
	case BinaryAck:
		return nil, h.onAck(packet.ID, decoder, packet)
	default:
		message = decoder.Message()
	}
	// h.PrintEventsRespondedTo()
	h.lock.RLock()
	c, ok := h.events[message]
	xc, ok1 := h.xEvents[message]
	h.lock.RUnlock()

	if !ok && !ok1 {
		// If the message is not recognized by the server, the decoder.currentCloser
		// needs to be closed otherwise the server will be stuck until the e xyzzy
		log.Printf("Error: %s was not found in h.events\n", message)
		decoder.Close()
		return nil, nil
	}

	_ = xc
	args := c.GetArgs() // returns Array of interface{}
	olen := len(args)
	if olen > 0 {
		packet.Data = &args
		if err := decoder.DecodeData(packet); err != nil {
			return nil, err
		}
	}

	// Padd out args to olen
	for i := len(args); i < olen; i++ {
		args = append(args, nil)
	}

	// ------------------------------------------------------ call ---------------------------------------------------------------------------------------
	retV := c.Call(h.socket, args)
	if len(retV) == 0 {
		return nil, nil
	}

	var err error
	if last, ok := retV[len(retV)-1].Interface().(error); ok {
		err = last
		retV = retV[0 : len(retV)-1]
	}
	ret := make([]interface{}, len(retV))
	for i, v := range retV {
		ret[i] = v.Interface()
	}

	return ret, err
}

func (h *socketHandler) onAck(id int, decoder *decoder, packet *packet) error {
	h.lock.Lock()
	defer h.lock.Unlock()
	c, ok := h.acks[id]
	if !ok {
		return nil
	}
	delete(h.acks, id)

	args := c.GetArgs()
	packet.Data = &args
	if err := decoder.DecodeData(packet); err != nil {
		return err
	}
	c.Call(h.socket, args)
	return nil
}
