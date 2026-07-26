package socketio

import (
	"errors"
	"fmt"
	"log"
	"maps"
	"reflect"
	"sync"
	"time"
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

// maxOutstandingAcks caps how many un-acknowledged emit callbacks a single
// socket may have registered at once. A peer that never sends ack responses
// (or a server that emits acks faster than they are answered) would otherwise
// grow the acks map without bound, exhausting memory. When the cap is
// exceeded the oldest outstanding callbacks are evicted (they will never fire).
const maxOutstandingAcks = 10000

type socketHandler struct {
	*baseHandler
	acks     map[int]*caller
	ackOrder []int // ids in insertion order; may hold ids already taken/evicted
	socket   *socket
	rooms    map[string]struct{}

	lastAckEvictLog time.Time
}

// registerAck records the ack callback for id. The cap is not enforced here so
// that a subsequent send failure (rolled back via unregisterAck) cannot cause
// an unrelated, still-valid callback to be evicted; commitAck enforces the cap
// once the frame has actually been written.
func (h *socketHandler) registerAck(id int, c *caller) {
	h.lock.Lock()
	h.acks[id] = c
	h.ackOrder = append(h.ackOrder, id)
	h.lock.Unlock()
}

// unregisterAck removes a pending ack callback by id (used to roll back a
// failed send). Because sendAck holds writeLock across register→encode→
// unregister, the rolled-back id is always the most recently appended entry,
// so it is popped from ackOrder here; otherwise a stream of failing sends
// (which never reach commitAck) would grow ackOrder without bound.
func (h *socketHandler) unregisterAck(id int) {
	h.lock.Lock()
	delete(h.acks, id)
	if n := len(h.ackOrder); n > 0 && h.ackOrder[n-1] == id {
		h.ackOrder = h.ackOrder[:n-1]
	}
	h.lock.Unlock()
}

// commitAck runs after a successful send: it enforces the outstanding-ack cap
// (evicting the oldest live callbacks) and compacts ackOrder so stale ids left
// by answered or evicted acks cannot accumulate without bound.
func (h *socketHandler) commitAck() {
	h.lock.Lock()
	defer h.lock.Unlock()

	evicted := 0
	if len(h.acks) > maxOutstandingAcks {
		// ackOrder holds every live id in insertion order, so scanning from
		// the front evicts the oldest outstanding callbacks first (FIFO).
		for _, id := range h.ackOrder {
			if len(h.acks) <= maxOutstandingAcks {
				break
			}
			if _, ok := h.acks[id]; ok {
				delete(h.acks, id)
				evicted++
			}
		}
	}

	// Compact when stale ids (already answered or evicted) dominate. This
	// reaps entries anywhere in the slice, not just a contiguous prefix, so an
	// early un-answered ack (out-of-order acking) cannot pin growth.
	if len(h.ackOrder) > 2*len(h.acks)+16 {
		kept := h.ackOrder[:0]
		for _, id := range h.ackOrder {
			if _, ok := h.acks[id]; ok {
				kept = append(kept, id)
			}
		}
		h.ackOrder = kept
	}

	if evicted > 0 {
		// Under a sustained flood this would fire on nearly every send; rate
		// limit so logging does not become its own amplification vector.
		if now := time.Now(); now.Sub(h.lastAckEvictLog) > time.Second {
			h.lastAckEvictLog = now
			log.Printf("socketio: evicting outstanding acks (more than %d unanswered)", maxOutstandingAcks)
		}
	}
}

// takeAck removes and returns the ack callback for id, if present.
func (h *socketHandler) takeAck(id int) (*caller, bool) {
	h.lock.Lock()
	defer h.lock.Unlock()
	c, ok := h.acks[id]
	if ok {
		delete(h.acks, id)
	}
	return c, ok
}

// clearAcks drops all pending ack callbacks. Called on socket teardown so the
// read loop's exit releases any callbacks (and their captured state) still
// waiting on responses that will never arrive.
func (h *socketHandler) clearAcks() {
	h.lock.Lock()
	h.acks = make(map[int]*caller)
	h.ackOrder = nil
	h.lock.Unlock()
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
		return h.socket.sendAck(args,
			func(id int) { h.registerAck(id, c) },
			h.unregisterAck,
			h.commitAck,
		)
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
	c, ok := h.takeAck(id)
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
