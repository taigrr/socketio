package socketio

import (
	"bytes"
	"io"
	"testing"
)

func newTestSocketHandler() *socketHandler {
	return newSocketHandler(&socket{}, newBaseHandler("", newBroadcastDefault()))
}

func TestNewSocketHandlerCopiesEventHandlers(t *testing.T) {
	base := newBaseHandler("/chat", newBroadcastDefault())
	if err := base.On("ping", func() {}); err != nil {
		t.Fatalf("On() error = %v", err)
	}
	if err := base.OnAny(func() {}); err != nil {
		t.Fatalf("OnAny() error = %v", err)
	}

	handler := newSocketHandler(&socket{}, base)

	if _, ok := handler.events["ping"]; !ok {
		t.Fatal("expected socket handler to copy registered events")
	}
	if len(handler.allEvents) != 1 {
		t.Fatalf("expected 1 allEvents handler, got %d", len(handler.allEvents))
	}
	if handler.name != "/chat" {
		t.Fatalf("expected namespace name to be copied, got %q", handler.name)
	}

	// Mutating the copy must not affect the base handler.
	if err := handler.On("pong", func() {}); err != nil {
		t.Fatalf("On() error = %v", err)
	}
	if _, ok := base.events["pong"]; ok {
		t.Fatal("socket handler events should be independent of base handler")
	}
}

func TestCommitAckEvictsOldestOverCap(t *testing.T) {
	h := newTestSocketHandler()
	c := &caller{}

	// Fill to capacity, committing after each registration.
	for id := 0; id < maxOutstandingAcks; id++ {
		h.registerAck(id, c)
		h.commitAck()
	}
	if got := len(h.acks); got != maxOutstandingAcks {
		t.Fatalf("acks at cap = %d, want %d", got, maxOutstandingAcks)
	}

	// One more registration+commit must evict the oldest (id 0) and stay at cap.
	h.registerAck(maxOutstandingAcks, c)
	h.commitAck()
	if got := len(h.acks); got != maxOutstandingAcks {
		t.Fatalf("acks after eviction = %d, want %d", got, maxOutstandingAcks)
	}
	if _, ok := h.acks[0]; ok {
		t.Fatal("oldest ack (id 0) should have been evicted")
	}
	if _, ok := h.acks[maxOutstandingAcks]; !ok {
		t.Fatal("newest ack should be present")
	}
}

func TestAckOrderDoesNotGrowUnbounded(t *testing.T) {
	h := newTestSocketHandler()
	c := &caller{}

	// Sequential register+commit+ack drain: stale ids must be compacted, not
	// retained one-per-historical-id.
	for id := 0; id < maxOutstandingAcks*4; id++ {
		h.registerAck(id, c)
		h.commitAck()
		if _, ok := h.takeAck(id); !ok {
			t.Fatalf("takeAck(%d) missing", id)
		}
	}
	if len(h.acks) != 0 {
		t.Fatalf("acks after draining = %d, want 0", len(h.acks))
	}
	if len(h.ackOrder) > 64 {
		t.Fatalf("ackOrder length = %d, expected it to stay compacted", len(h.ackOrder))
	}
}

func TestAckOrderCompactsWithPinnedOldestAck(t *testing.T) {
	h := newTestSocketHandler()
	c := &caller{}

	// The oldest ack (id 0) is never answered, so it sits at the front of the
	// order forever. Out-of-order acking of everything else must still be
	// reaped from the middle of ackOrder rather than pinning growth.
	h.registerAck(0, c)
	h.commitAck()
	for id := 1; id < maxOutstandingAcks*4; id++ {
		h.registerAck(id, c)
		h.commitAck()
		if _, ok := h.takeAck(id); !ok {
			t.Fatalf("takeAck(%d) missing", id)
		}
	}
	if _, ok := h.acks[0]; !ok {
		t.Fatal("pinned ack id 0 should still be outstanding")
	}
	if len(h.acks) != 1 {
		t.Fatalf("acks = %d, want 1 (only the pinned id)", len(h.acks))
	}
	if len(h.ackOrder) > 64 {
		t.Fatalf("ackOrder length = %d, expected compaction despite pinned head", len(h.ackOrder))
	}
}

func TestClearAcksDropsPending(t *testing.T) {
	h := newTestSocketHandler()
	c := &caller{}
	for id := 0; id < 5; id++ {
		h.registerAck(id, c)
		h.commitAck()
	}

	h.clearAcks()

	if len(h.acks) != 0 {
		t.Fatalf("acks after clear = %d, want 0", len(h.acks))
	}
	if len(h.ackOrder) != 0 {
		t.Fatalf("ack order not reset: len=%d", len(h.ackOrder))
	}
}

func TestUnregisterAckDoesNotGrowOrderOnFailedSends(t *testing.T) {
	h := newTestSocketHandler()
	c := &caller{}

	// Mirror sendAck's failure path: register then roll back, repeatedly.
	// commitAck never runs, so unregisterAck itself must reap the stale id.
	for id := 0; id < maxOutstandingAcks*4; id++ {
		h.registerAck(id, c)
		h.unregisterAck(id)
	}
	if len(h.acks) != 0 {
		t.Fatalf("acks after failed sends = %d, want 0", len(h.acks))
	}
	if len(h.ackOrder) != 0 {
		t.Fatalf("ackOrder after failed sends = %d, want 0", len(h.ackOrder))
	}
}

// eventDecoder encodes an Event packet and returns a decoder positioned to
// dispatch it, along with the decoded header packet.
func eventDecoder(t *testing.T, data ...any) (*decoder, *packet) {
	t.Helper()
	saver := &FrameSaver{}
	if err := newEncoder(saver).Encode(packet{Type: Event, ID: -1, Data: data}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	d := newDecoder(saver)
	var p packet
	if err := d.Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return d, &p
}

func TestOnPacketDispatchesSpecificAndOnAny(t *testing.T) {
	base := newBaseHandler("", newBroadcastDefault())

	var specificMsg string
	var specificNum int
	if err := base.On("chat", func(_ Socket, msg string, n int) string {
		specificMsg, specificNum = msg, n
		return "ok"
	}); err != nil {
		t.Fatalf("On: %v", err)
	}

	anyCalls := 0
	var anyMsg string
	var anyNum int
	if err := base.OnAny(func(_ Socket, msg string, n int) {
		anyCalls++
		anyMsg, anyNum = msg, n
	}); err != nil {
		t.Fatalf("OnAny: %v", err)
	}

	h := newSocketHandler(&socket{}, base)
	d, p := eventDecoder(t, "chat", "hello", 42)
	ret, err := h.onPacket(d, p)
	if err != nil {
		t.Fatalf("onPacket: %v", err)
	}

	if specificMsg != "hello" || specificNum != 42 {
		t.Fatalf("specific handler got (%q, %d), want (hello, 42)", specificMsg, specificNum)
	}
	if anyCalls != 1 || anyMsg != "hello" || anyNum != 42 {
		t.Fatalf("OnAny got calls=%d (%q, %d), want 1 (hello, 42)", anyCalls, anyMsg, anyNum)
	}
	if len(ret) != 1 || ret[0] != "ok" {
		t.Fatalf("specific handler ret = %v, want [ok]", ret)
	}
	if d.current != nil {
		t.Fatal("decoder stream should be closed after dispatch")
	}
}

func TestOnPacketOnAnyWithoutSpecificHandler(t *testing.T) {
	base := newBaseHandler("", newBroadcastDefault())

	got := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		if err := base.OnAny(func(_ Socket, msg string) {
			got = append(got, msg)
		}); err != nil {
			t.Fatalf("OnAny: %v", err)
		}
	}

	h := newSocketHandler(&socket{}, base)
	d, p := eventDecoder(t, "no-specific-handler", "payload")
	if _, err := h.onPacket(d, p); err != nil {
		t.Fatalf("onPacket: %v", err)
	}

	// Both OnAny handlers fire, each decoding the payload independently.
	if len(got) != 2 || got[0] != "payload" || got[1] != "payload" {
		t.Fatalf("OnAny handlers got %v, want [payload payload]", got)
	}
}

func TestOnPacketOnAnySkipsLifecycleMessages(t *testing.T) {
	base := newBaseHandler("", newBroadcastDefault())

	anyCalls := 0
	if err := base.OnAny(func() { anyCalls++ }); err != nil {
		t.Fatalf("OnAny: %v", err)
	}
	if err := base.On("connection", func(Socket) {}); err != nil {
		t.Fatalf("On: %v", err)
	}

	h := newSocketHandler(&socket{}, base)
	if _, err := h.onPacket(nil, &packet{Type: Connect, ID: -1}); err != nil {
		t.Fatalf("onPacket: %v", err)
	}

	if anyCalls != 0 {
		t.Fatalf("OnAny fired %d times for a lifecycle message, want 0", anyCalls)
	}
}

func TestOnPacketOnAnyDecodeErrorIsNonFatal(t *testing.T) {
	base := newBaseHandler("", newBroadcastDefault())

	specificCalled := false
	if err := base.On("chat", func(_ Socket, msg string) string {
		specificCalled = msg == "hi"
		return "ok"
	}); err != nil {
		t.Fatalf("On: %v", err)
	}
	// This OnAny handler's typed args are incompatible with the payload
	// (int where the event sends a string), so applyArgs fails for it.
	if err := base.OnAny(func(_ Socket, n int) {
		t.Fatal("incompatible OnAny handler should not be invoked")
	}); err != nil {
		t.Fatalf("OnAny: %v", err)
	}

	h := newSocketHandler(&socket{}, base)
	d, p := eventDecoder(t, "chat", "hi")
	ret, err := h.onPacket(d, p)
	if err != nil {
		t.Fatalf("onPacket returned fatal error for a bad OnAny handler: %v", err)
	}
	if !specificCalled {
		t.Fatal("specific handler must still run despite a failing OnAny observer")
	}
	if len(ret) != 1 || ret[0] != "ok" {
		t.Fatalf("ack response = %v, want [ok]", ret)
	}
}

func TestOnPacketOnAnyPanicIsNonFatal(t *testing.T) {
	base := newBaseHandler("", newBroadcastDefault())

	specificCalled := false
	if err := base.On("chat", func(_ Socket, msg string) string {
		specificCalled = msg == "hi"
		return "ok"
	}); err != nil {
		t.Fatalf("On: %v", err)
	}
	if err := base.OnAny(func(_ Socket, _ string) {
		panic("boom")
	}); err != nil {
		t.Fatalf("OnAny: %v", err)
	}

	h := newSocketHandler(&socket{}, base)
	d, p := eventDecoder(t, "chat", "hi")
	ret, err := h.onPacket(d, p)
	if err != nil {
		t.Fatalf("onPacket returned fatal error for a panicking OnAny handler: %v", err)
	}
	if !specificCalled {
		t.Fatal("specific handler must still run despite a panicking OnAny observer")
	}
	if len(ret) != 1 || ret[0] != "ok" {
		t.Fatalf("ack response = %v, want [ok]", ret)
	}
}

type fileArg struct {
	Name string      `json:"name"`
	File *Attachment `json:"file"`
}

func TestOnPacketBinaryEventDispatchesToSpecificAndOnAny(t *testing.T) {
	saver := &FrameSaver{}
	if err := newEncoder(saver).Encode(packet{
		Type: Event,
		ID:   -1,
		Data: []any{"file", fileArg{Name: "x", File: &Attachment{Data: bytes.NewBufferString("blob")}}},
	}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	d := newDecoder(saver)
	var p packet
	if err := d.Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}

	base := newBaseHandler("", newBroadcastDefault())

	read := func(a *Attachment) string {
		if a == nil || a.Data == nil {
			return ""
		}
		b, _ := io.ReadAll(a.Data)
		return string(b)
	}

	var specificData string
	if err := base.On("file", func(_ Socket, arg fileArg) {
		specificData = read(arg.File)
	}); err != nil {
		t.Fatalf("On: %v", err)
	}
	var anyName, anyData string
	if err := base.OnAny(func(_ Socket, arg fileArg) {
		anyName = arg.Name
		anyData = read(arg.File)
	}); err != nil {
		t.Fatalf("OnAny: %v", err)
	}

	h := newSocketHandler(&socket{}, base)
	if _, err := h.onPacket(d, &p); err != nil {
		t.Fatalf("onPacket: %v", err)
	}

	// Both callers must independently receive the same attachment bytes.
	if specificData != "blob" {
		t.Fatalf("specific handler attachment = %q, want blob", specificData)
	}
	if anyName != "x" || anyData != "blob" {
		t.Fatalf("OnAny got (%q, %q), want (x, blob)", anyName, anyData)
	}
}
