# socketio

[![Go Reference](https://pkg.go.dev/badge/github.com/taigrr/socketio.svg)](https://pkg.go.dev/github.com/taigrr/socketio)

A Go (golang) implementation of [socket.io](https://socket.io), compatible with socket.io version 2.3.0.
Supports rooms, namespaces, and real-time bidirectional browser-server communication.

Originally forked from [googollee/go-socket.io](https://github.com/googollee/go-socket.io) with
defect fixes, import cleanup, and dependency modernization.

## Install

```bash
go get github.com/taigrr/socketio
```

## Example

See the [examples/chat](./examples/chat) directory for a full working chat server.

```go
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/taigrr/socketio"
)

func main() {
	server, err := socketio.NewServer(nil)
	if err != nil {
		log.Fatal(err)
	}

	server.On("connection", func(so socketio.Socket) {
		fmt.Println("a user connected")
		so.Join("chat")
		so.On("chat message", func(msg string) {
			fmt.Println("chat message:", msg)
			so.BroadcastTo("chat", "chat message", msg)
		})
		so.On("disconnect", func() {
			fmt.Println("user disconnected")
		})
	})

	server.On("error", func(so socketio.Socket, err error) {
		fmt.Printf("error: %s\n", err)
	})

	http.Handle("/socket.io/", server)
	http.Handle("/", http.FileServer(http.Dir("./asset")))
	fmt.Println("serving on :9000")
	log.Fatal(http.ListenAndServe(":9000", nil))
}
```

## License

3-clause BSD — see [LICENSE](./LICENSE) for details.
