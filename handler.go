package socketio

import (
	"errors"
	"fmt"
	"log"
	"maps"
	"reflect"
	"sync"
)

type baseHandler struct {
	events    map[string]*caller
	allEvents []*caller
	name      string
	broadcast BroadcastAdaptor
	lock      sync.RWMutex
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
func (h *baseHandler) On(message string, f any) error {
	c, err := newCaller(f)
	if err != nil {
		return err
	}
	h.lock.Lock()
	h.events[message] = c
	h.lock.Unlock()
	return nil
}

// On registers the function f to handle ANY message.
func (h *baseHandler) OnAny(f any) error {
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
	allEvents := make([]*caller, 0, len(base.allEvents))
	base.lock.Lock()
	maps.Copy(events, base.events)
	allEvents = append(allEvents, base.allEvents...)
	name := base.name
	base.lock.Unlock()
	return &socketHandler{
		baseHandler: &baseHandler{
			events:    events,
			allEvents: allEvents,
			name:      name,
			broadcast: base.broadcast,
		},
		acks:   make(map[int]*caller),
		socket: s,
		rooms:  make(map[string]struct{}),
	}
}

func (h *socketHandler) Emit(message string, args ...any) error {
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
	args = append([]any{message}, args...)
	if c != nil {
		id, err := h.socket.sendID(args)
		if err != nil {
			return err
		}
		h.lock.Lock()
		h.acks[id] = c
		h.lock.Unlock()
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
	h.lock.Lock()
	rooms := make([]string, 0, len(h.rooms))
	for room := range h.rooms {
		rooms = append(rooms, room)
	}
	h.rooms = make(map[string]struct{})
	h.lock.Unlock()

	var errs []error
	for _, room := range rooms {
		if err := h.broadcast.Leave(h.broadcastName(room), h.socket); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (h *baseHandler) BroadcastTo(room, message string, args ...any) error {
	return h.broadcast.Send(nil, h.broadcastName(room), message, args...)
}

func (h *socketHandler) BroadcastTo(room, message string, args ...any) error {
	return h.broadcast.Send(h.socket, h.broadcastName(room), message, args...)
}

func (h *baseHandler) broadcastName(room string) string {
	return fmt.Sprintf("%s:%s", h.name, room)
}

func (h *socketHandler) onPacket(decoder *decoder, packet *packet) ([]any, error) {
	var message string
	switch packet.Type {
	case Connect:
		message = "connection"
	case Disconnect:
		message = "disconnect"
	case Error:
		message = "error"
	case Ack, BinaryAck:
		return nil, h.onAck(packet.ID, decoder, packet)
	default:
		message = decoder.Message()
	}
	h.lock.RLock()
	c, ok := h.events[message]
	h.lock.RUnlock()

	if !ok {
		// The message has no registered handler. Close the decoder so its
		// underlying frame is released; otherwise the read loop can stall
		// waiting on an open reader.
		log.Printf("socketio: no handler registered for message %q", message)
		decoder.Close()
		return nil, nil
	}

	args := c.GetArgs()
	olen := len(args)
	if olen > 0 {
		packet.Data = &args
		if err := decoder.DecodeData(packet); err != nil {
			return nil, err
		}
	}

	// Pad out args to olen
	for i := len(args); i < olen; i++ {
		args = append(args, nil)
	}

	retV := c.Call(h.socket, args)
	if len(retV) == 0 {
		return nil, nil
	}

	var err error
	if last, ok := retV[len(retV)-1].Interface().(error); ok {
		err = last
		retV = retV[0 : len(retV)-1]
	}
	ret := make([]any, len(retV))
	for i, v := range retV {
		ret[i] = v.Interface()
	}

	return ret, err
}

func (h *socketHandler) onAck(id int, decoder *decoder, packet *packet) error {
	h.lock.Lock()
	c, ok := h.acks[id]
	if ok {
		delete(h.acks, id)
	}
	h.lock.Unlock()
	if !ok {
		// No handler is waiting on this ack id; close the decoder so the
		// read loop does not stall on an open frame.
		decoder.Close()
		return nil
	}

	args := c.GetArgs()
	packet.Data = &args
	if err := decoder.DecodeData(packet); err != nil {
		return err
	}
	c.Call(h.socket, args)
	return nil
}
