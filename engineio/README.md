# engineio

[![Go Reference](https://pkg.go.dev/badge/github.com/taigrr/socketio/engineio.svg)](https://pkg.go.dev/github.com/taigrr/socketio/engineio)

A Go implementation of [engine.io](https://github.com/socketio/engine.io), the transport layer
for [socket.io](https://socket.io). Supports long-polling and WebSocket transports.

Compatible with the Node.js engine.io implementation.

## Install

```bash
go get github.com/taigrr/socketio/engineio
```

## Example

See the [example](./example) directory for a working demo.

```go
package main

import (
	"encoding/hex"
	"io"
	"log"
	"net/http"

	"github.com/taigrr/socketio/engineio"
)

func main() {
	server, err := engineio.NewServer(nil)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			conn, _ := server.Accept()
			go func() {
				defer conn.Close()
				for i := 0; i < 10; i++ {
					t, r, _ := conn.NextReader()
					b, _ := io.ReadAll(r)
					r.Close()
					if t == engineio.MessageText {
						log.Println(t, string(b))
					} else {
						log.Println(t, hex.EncodeToString(b))
					}
					w, _ := conn.NextWriter(t)
					w.Write([]byte("pong"))
					w.Close()
				}
			}()
		}
	}()

	http.Handle("/engine.io/", server)
	http.Handle("/", http.FileServer(http.Dir("./asset")))
	log.Println("serving at localhost:5000...")
	log.Fatal(http.ListenAndServe(":5000", nil))
}
```

## License

3-clause BSD — see [LICENSE](../LICENSE) for details.
