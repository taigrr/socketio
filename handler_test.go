package socketio

import "testing"

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
