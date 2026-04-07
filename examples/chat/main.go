package main

//
// Command line arguments can be used to set the IP address that is listened to and the port.
//
// $ ./chat --port=8080 --host=127.0.0.1 --dir=./asset
//
// Bring up a pair of browsers and chat between them.
//

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/taigrr/socketio"
)

var Port = flag.String("port", "9000", "Port to listen to")
var HostIP = flag.String("host", "localhost", "Host name or IP address to listen on")
var Dir = flag.String("dir", "./asset", "Directory where files are served from")

func init() {
	flag.StringVar(Port, "P", "9000", "Port to listen to")
	flag.StringVar(HostIP, "H", "localhost", "Host name or IP address to listen on")
	flag.StringVar(Dir, "d", "./asset", "Directory where files are served from")
}

func main() {
	flag.Parse()
	fns := flag.Args()

	if len(fns) != 0 {
		fmt.Printf("Usage: Invalid arguments supplied, %s\n", fns)
		flag.Usage()
		os.Exit(1)
	}

	var hostIP string
	if *HostIP != "localhost" {
		hostIP = *HostIP
	}

	server, err := socketio.NewServer(nil)
	if err != nil {
		log.Fatalf("creating socket.io server: %v", err)
	}

	server.On("connection", func(so socketio.Socket) {
		log.Println("user connected")
		so.Join("chat")

		so.On("new message", func(msg string) {
			log.Printf("chat message: %s", msg)
			so.BroadcastTo("chat", "chat message", msg)
		})
		so.On("disconnect", func() {
			log.Println("user disconnected")
		})
		so.On("add user", func() {
			log.Println("add user")
			so.Emit("login", "Hello")
		})
		so.On("typing", func() {
			log.Println("typing")
		})
		so.On("stop typing", func() {
			log.Println("stop typing")
		})
	})

	server.On("error", func(so socketio.Socket, err error) {
		log.Printf("error: %v", err)
	})

	http.Handle("/socket.io/", server)
	http.Handle("/", http.FileServer(http.Dir(*Dir)))
	listen := fmt.Sprintf("%s:%s", hostIP, *Port)
	fmt.Printf("Serving on port %s, browse to http://localhost:%s/\n", *Port, *Port)
	log.Fatal(http.ListenAndServe(listen, nil))
}
