package websocket

import (
	"github.com/taigrr/socketio/engineio/transport"
)

var Creater = transport.Creater{
	Name:      "websocket",
	Upgrading: true,
	Server:    NewServer,
	Client:    NewClient,
}
