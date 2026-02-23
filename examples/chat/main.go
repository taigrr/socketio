package main

//
// Command line arguments can be used to set the IP address that is listened to and the port.
//
// $ ./chat --port=8080 --host=127.0.0.1 --dir=./asset
//
// Bring up a pair of browsers and chat between them.
//

//
// Notes
//
// 1. Updated to use current jQuery 3.1.5 -- Sun May 10 06:45:52 MDT 2020
//

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"runtime"

	"github.com/taigrr/socketio"
)

var Port = flag.String("port", "9000", "Port to listen to")                           // 0
var HostIP = flag.String("host", "localhost", "Host name or IP address to listen on") // 1
var Dir = flag.String("dir", "./asset", "Directory where files are served from")      // 2
var Debug = flag.String("debug", "", "Comma separated list of debug flags")           // 3
func init() {
	flag.StringVar(Port, "P", "9000", "Port to listen to")                           // 0
	flag.StringVar(HostIP, "H", "localhost", "Host name or IP address to listen on") // 1
	flag.StringVar(Dir, "d", "./asset", "Direcotry where files are served from")     // 2
}

func Usage() {
	fmt.Printf(`
Compile and run server with:

$ go run main.go [ -P | --port #### ] [ -H | --host IP-Host ] [ -d | --dir Path-To-Assets ]

-P | --port        Port number.  Default 9000
-H | --host        Host to listen on.  Default 'localhost' but can be an IP or 0.0.0.0 for
                   IP addresses on this system.
-d | --dir         Directory to serve with files.  Default ./asset.
--debug            Debug flags 

`)
}

var DebugFlag = make(map[string]bool)

func callerInfo() string {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		return "unknown"
	}
	return fmt.Sprintf("%s:%d", file, line)
}

func main() {

	flag.Parse()
	fns := flag.Args()

	if len(fns) != 0 {
		fmt.Printf("Usage: Invalid arguments supplied, %s\n", fns)
		Usage()
		os.Exit(1)
	}

	var host_ip string = ""
	if *HostIP != "localhost" {
		host_ip = *HostIP
	}

	if *Debug != "" {
		for _, s := range strings.Split(*Debug, ",") {
			DebugFlag[s] = true
		}
		// Debug flags removed (legacy pschlump debug system)
	}

	// Make certain that the command line parameters are handled correctly
	// fmt.Printf("host_ip >%s< HostIP >%s< Port >%s<\n", host_ip, *HostIP, *Port)

	server, err := socketio.NewServer(nil)
	if err != nil {
		log.Fatal(fmt.Errorf("creating socket.io server at %s: %w", callerInfo(), err))
	}

	// connection ->
	//	new messsage -> brodcast "chat message"
	//  disconnect
	//  add user -> login
	//  typing -> ???
	//  stop typing -> ???

	server.On("connection", func(so socketio.Socket) {
		fmt.Printf("%sa user connected%s, %s\n", "MiscLib.ColorGreen33[32m", "MiscLib.ColorReset33[0m", callerInfo())
		so.Join("chat")
		//so.On("chat message", func(msg string) {
		//	fmt.Printf("%schat message, %s%s, %s\n", "MiscLib.ColorGreen33[32m", msg, "MiscLib.ColorReset33[0m", callerInfo())
		//	so.BroadcastTo("chat", "chat message", msg)
		//})
		so.On("new message", func(msg string) {
			fmt.Printf("%schat message: -->>%s<<--%s, %s\n", "MiscLib.ColorGreen33[32m", msg, "MiscLib.ColorReset33[0m", callerInfo())
			so.BroadcastTo("chat", "chat message", msg)
		})
		so.On("disconnect", func() {
			fmt.Printf("%suser disconnect%s, %s\n", "MiscLib.ColorYellow33[33m", "MiscLib.ColorReset33[0m", callerInfo())
		})
		so.On("add user", func() {
			fmt.Printf("%sadd user%s, %s\n", "MiscLib.ColorRed33[31m", "MiscLib.ColorReset33[0m", callerInfo())
			so.Emit("login", fmt.Sprintf("Hello %s", "xyzzy"))
		})

		so.On("typing", func() {
			fmt.Printf("%styping%s, %s\n", "MiscLib.ColorRed33[31m", "MiscLib.ColorReset33[0m", callerInfo())
		})
		so.On("stop typing", func() {
			fmt.Printf("%sstop typing%s, %s\n", "MiscLib.ColorRed33[31m", "MiscLib.ColorReset33[0m", callerInfo())
		})
	})

	server.On("error", func(so socketio.Socket, err error) {
		fmt.Printf("Error: %s, %s\n", err, callerInfo())
	})

	http.Handle("/socket.io/", server)
	http.Handle("/", http.FileServer(http.Dir(*Dir)))
	fmt.Printf("Serving on port %s, browse to http://localhost:%s/\n", *Port, *Port)
	listen := fmt.Sprintf("%s:%s", host_ip, *Port)
	log.Fatal(http.ListenAndServe(listen, nil))
}
