package socketio

import "testing"

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
