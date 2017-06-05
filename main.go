package main

import (
	"github.com/elians/websocket/server"
)

func main() {
	s := server.NewWebsocket(":5000", "/test")
	s.Listen()
}
