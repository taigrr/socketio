package socketio

import "testing"

func TestBaseHandlerHandleInitializesExtendedHandlers(t *testing.T) {
	handler := newBaseHandler("/", newBroadcastDefault())

	called := false
	if err := handler.Handle("ping", func(_ *Socket, message string, _ [][]byte) error {
		called = message == "ping"
		return nil
	}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	if _, ok := handler.xEvents["ping"]; !ok {
		t.Fatal("expected extended handler to be registered")
	}
	if called {
		t.Fatal("handler should not be invoked during registration")
	}
}

func TestNewSocketHandlerCopiesExtendedHandlers(t *testing.T) {
	base := newBaseHandler("/", newBroadcastDefault())
	base.xAllEvents = append(base.xAllEvents, func(_ *Socket, _ string, _ [][]byte) error { return nil })

	if err := base.Handle("ping", func(_ *Socket, message string, _ [][]byte) error {
		if message != "ping" {
			t.Fatalf("unexpected message %q", message)
		}
		return nil
	}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	handler := newSocketHandler(&socket{}, base)

	if _, ok := handler.xEvents["ping"]; !ok {
		t.Fatal("expected socket handler to copy extended handlers")
	}
	if len(handler.xAllEvents) != 1 {
		t.Fatalf("expected 1 xAllEvents handler, got %d", len(handler.xAllEvents))
	}
}
