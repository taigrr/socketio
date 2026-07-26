package websocket

import (
	"io"
	"net/http"

	"github.com/taigrr/socketio/engineio/message"
	"github.com/taigrr/socketio/engineio/parser"
	"github.com/taigrr/socketio/engineio/transport"

	"github.com/gorilla/websocket"
)

type Server struct {
	callback transport.Callback
	conn     *websocket.Conn
}

// Upgrader is the gorilla/websocket upgrader used to promote incoming HTTP
// requests to WebSocket connections. It is exported so applications can tune
// buffer sizes, subprotocols, compression, and—most importantly—the
// CheckOrigin policy.
//
// By default CheckOrigin is nil, which makes gorilla/websocket enforce a
// same-origin policy (the request Origin host must match the Host header).
// This is the safe default and prevents Cross-Site WebSocket Hijacking. To
// intentionally accept cross-origin connections, set a custom CheckOrigin
// before serving, e.g.:
//
//	websocket.Upgrader.CheckOrigin = func(r *http.Request) bool { return true }
var Upgrader = websocket.Upgrader{
	ReadBufferSize:  10240,
	WriteBufferSize: 10240,
}

func NewServer(w http.ResponseWriter, r *http.Request, callback transport.Callback) (transport.Server, error) {
	conn, err := Upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}

	ret := &Server{
		callback: callback,
		conn:     conn,
	}

	go ret.serveHTTP(w, r)

	return ret, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusBadRequest)
}

func (s *Server) NextWriter(msgType message.MessageType, packetType parser.PacketType) (io.WriteCloser, error) {
	wsType, newEncoder := websocket.TextMessage, parser.NewStringEncoder
	if msgType == message.MessageBinary {
		wsType, newEncoder = websocket.BinaryMessage, parser.NewBinaryEncoder
	}

	w, err := s.conn.NextWriter(wsType)
	if err != nil {
		return nil, err
	}
	ret, err := newEncoder(w, packetType)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (s *Server) Close() error {
	return s.conn.Close()
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	defer s.callback.OnClose(s)

	for {
		t, r, err := s.conn.NextReader()
		if err != nil {
			s.conn.Close()
			return
		}

		switch t {
		case websocket.TextMessage:
			fallthrough
		case websocket.BinaryMessage:
			decoder, err := parser.NewDecoder(r)
			if err != nil {
				return
			}
			s.callback.OnPacket(decoder)
			decoder.Close()
		}
	}
}
